package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yairp7/pkgfort/internal/audit"
	"github.com/yairp7/pkgfort/internal/cache"
	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/ecosystem"
	"github.com/yairp7/pkgfort/internal/proxy"
	"github.com/yairp7/pkgfort/internal/rule"
)

// --- mocks ---

type mockEcosystem struct {
	upstream        *httptest.Server
	validationResult ecosystem.ValidationTargetResult
	publishTime     time.Time
	fetchErr        error
}

func (m *mockEcosystem) Name() string             { return "test" }
func (m *mockEcosystem) UpstreamURLs() []string   { return []string{m.upstream.URL} }
func (m *mockEcosystem) NeedsProxy(_ []string) bool { return true }
func (m *mockEcosystem) ValidationTarget(_ *http.Request) ecosystem.ValidationTargetResult {
	return m.validationResult
}
func (m *mockEcosystem) FetchPackageInfo(_ context.Context, pkg, version string) (ecosystem.PackageInfo, error) {
	return ecosystem.PackageInfo{
		Ecosystem:   m.Name(),
		Package:     pkg,
		Version:     version,
		PublishedAt: m.publishTime,
	}, m.fetchErr
}

type mockRule struct {
	result rule.Result
}

func (m *mockRule) Name() string { return "mock_rule" }
func (m *mockRule) Evaluate(_ context.Context, _ ecosystem.PackageInfo) (rule.Result, error) {
	return m.result, nil
}

// --- helpers ---

func newProxy(t *testing.T, eco ecosystem.Ecosystem, rules []rule.Rule, cfg config.Config) *proxy.Proxy {
	t.Helper()
	dir := t.TempDir()
	c, err := cache.New(filepath.Join(dir, "cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	return proxy.New(eco, rules, c, audit.New(filepath.Join(dir, "audit.log")), &http.Client{}, cfg)
}

// --- BuildRules tests ---

func TestAllRules_ReturnsAllRegardlessOfEnabledRules(t *testing.T) {
	cfg := config.Default()
	cfg.EnabledRules = nil
	rules := proxy.AllRules(cfg, &http.Client{})
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
}

func TestAllRules_ContainsKnownRules(t *testing.T) {
	rules := proxy.AllRules(config.Default(), &http.Client{})
	names := make(map[string]bool, len(rules))
	for _, r := range rules {
		names[r.Name()] = true
	}
	for _, want := range []string{"min_age_days", "vuln_check"} {
		if !names[want] {
			t.Errorf("rule %q missing from AllRules", want)
		}
	}
}

func TestBuildRules_Default(t *testing.T) {
	rules := proxy.BuildRules(config.Default(), &http.Client{})
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
}

func TestBuildRules_SingleRule(t *testing.T) {
	for _, name := range []string{"min_age_days", "vuln_check"} {
		cfg := config.Default()
		cfg.EnabledRules = []string{name}
		rules := proxy.BuildRules(cfg, &http.Client{})
		if len(rules) != 1 {
			t.Errorf("%s: got %d rules, want 1", name, len(rules))
		} else if rules[0].Name() != name {
			t.Errorf("got rule %q, want %q", rules[0].Name(), name)
		}
	}
}

func TestBuildRules_NoneEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.EnabledRules = nil
	rules := proxy.BuildRules(cfg, &http.Client{})
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0", len(rules))
	}
}

func TestBuildRules_UnknownRuleIgnored(t *testing.T) {
	cfg := config.Default()
	cfg.EnabledRules = []string{"unknown_rule"}
	rules := proxy.BuildRules(cfg, &http.Client{})
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0", len(rules))
	}
}

// --- tests ---

func TestServeHTTP_PassThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	eco := &mockEcosystem{
		upstream:        upstream,
		validationResult: ecosystem.ValidationTargetResult{NeedsValidation: false},
	}
	p := newProxy(t, eco, nil, config.Default())

	r := httptest.NewRequest(http.MethodGet, "/react/-/react-18.2.0.tgz", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServeHTTP_RulePasses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	eco := &mockEcosystem{
		upstream:        upstream,
		validationResult: ecosystem.ValidationTargetResult{Package: "react", Version: "18.2.0", NeedsValidation: true},
		publishTime:     time.Now().AddDate(0, 0, -30),
	}
	rules := []rule.Rule{&mockRule{result: rule.Result{Passed: true, Reason: "old enough"}}}
	p := newProxy(t, eco, rules, config.Default())

	r := httptest.NewRequest(http.MethodGet, "/react/18.2.0", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServeHTTP_RuleBlocks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	eco := &mockEcosystem{
		upstream:        upstream,
		validationResult: ecosystem.ValidationTargetResult{Package: "react", Version: "19.0.0", NeedsValidation: true},
		publishTime:     time.Now().AddDate(0, 0, -1),
	}
	rules := []rule.Rule{&mockRule{result: rule.Result{Passed: false, Reason: "published 1 day ago, minimum 7"}}}
	p := newProxy(t, eco, rules, config.Default())

	r := httptest.NewRequest(http.MethodGet, "/react/19.0.0", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", w.Code, http.StatusForbidden)
	}
	if !strings.Contains(w.Body.String(), "BLOCKED") {
		t.Errorf("expected BLOCKED in body, got: %s", w.Body.String())
	}
}

func TestServeHTTP_FetchError_FailClosed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	eco := &mockEcosystem{
		upstream:        upstream,
		validationResult: ecosystem.ValidationTargetResult{Package: "react", Version: "18.2.0", NeedsValidation: true},
		fetchErr:        context.DeadlineExceeded,
	}
	cfg := config.Default()
	cfg.FailOpen = false
	p := newProxy(t, eco, nil, cfg)

	r := httptest.NewRequest(http.MethodGet, "/react/18.2.0", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("got status %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestServeHTTP_FetchError_FailOpen(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	eco := &mockEcosystem{
		upstream:        upstream,
		validationResult: ecosystem.ValidationTargetResult{Package: "react", Version: "18.2.0", NeedsValidation: true},
		fetchErr:        context.DeadlineExceeded,
	}
	cfg := config.Default()
	cfg.FailOpen = true
	p := newProxy(t, eco, nil, cfg)

	r := httptest.NewRequest(http.MethodGet, "/react/18.2.0", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestServeHTTP_AllowlistBypass(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	eco := &mockEcosystem{
		upstream:        upstream,
		validationResult: ecosystem.ValidationTargetResult{Package: "react", Version: "19.0.0", NeedsValidation: true},
	}
	rules := []rule.Rule{&mockRule{result: rule.Result{Passed: false, Reason: "too new"}}}
	cfg := config.Default()
	cfg.Allowlist = []config.AllowlistEntry{{Ecosystem: "test", Package: "react"}}
	p := newProxy(t, eco, rules, cfg)

	r := httptest.NewRequest(http.MethodGet, "/react/19.0.0", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
	}
}
