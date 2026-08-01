package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/yairp7/pkgfort/internal/audit"
	"github.com/yairp7/pkgfort/internal/cache"
	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/ecosystem"
	pkgenv "github.com/yairp7/pkgfort/internal/env"
	"github.com/yairp7/pkgfort/internal/rule"
	"github.com/yairp7/pkgfort/internal/rule/osv"
)

type Proxy struct {
	ecosystem  ecosystem.Ecosystem
	rules      []rule.Rule
	cache      *cache.Cache
	auditLog   *audit.Log
	httpClient *http.Client
	cfg        config.Config
}

func New(
	eco ecosystem.Ecosystem,
	rules []rule.Rule,
	c *cache.Cache,
	auditLog *audit.Log,
	httpClient *http.Client,
	cfg config.Config,
) *Proxy {
	return &Proxy{
		ecosystem:  eco,
		rules:      rules,
		cache:      c,
		auditLog:   auditLog,
		httpClient: httpClient,
		cfg:        cfg,
	}
}

// Start binds to a random port and serves until ctx is cancelled.
// It prints "READY port=N" to stdout once the listener is up.
func (p *Proxy) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("READY port=%d\n", port)

	srv := &http.Server{Handler: p}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxyLog("→ %s %s", r.Method, r.URL.Path)
	target := p.ecosystem.ValidationTarget(r)

	if !target.NeedsValidation {
		p.forwardRequest(w, r)
		return
	}

	projectDir, _ := os.Getwd()

	if pkgenv.SkipRequested() {
		proxyLog("bypass %s@%s (PKGFORT_SKIP=1)", target.Package, target.Version)
		p.auditLog.Write(r.Context(), audit.Entry{
			Ecosystem:  p.ecosystem.Name(),
			Package:    target.Package,
			Version:    target.Version,
			Outcome:    audit.OutcomeBypassed,
			Reason:     "PKGFORT_SKIP=1",
			ProjectDir: projectDir,
		})
		p.forwardRequest(w, r)
		return
	}

	if p.isAllowlisted(target.Package) {
		proxyLog("bypass %s@%s (allowlist)", target.Package, target.Version)
		p.auditLog.Write(r.Context(), audit.Entry{
			Ecosystem:  p.ecosystem.Name(),
			Package:    target.Package,
			Version:    target.Version,
			Outcome:    audit.OutcomeBypassed,
			Reason:     "allowlist",
			ProjectDir: projectDir,
		})
		p.forwardRequest(w, r)
		return
	}

	proxyLog("checking %s@%s", target.Package, target.Version)
	cacheKey := fmt.Sprintf("%s:%s@%s", p.ecosystem.Name(), target.Package, target.Version)

	var pkg ecosystem.PackageInfo
	if entry, ok := p.cache.Get(cacheKey); ok {
		var publishedAt time.Time
		if err := json.Unmarshal(entry["published_at"], &publishedAt); err != nil {
			proxyLog("cache decode error for %s: %v — refetching", cacheKey, err)
		} else {
			pkg = ecosystem.PackageInfo{
				Ecosystem:   p.ecosystem.Name(),
				Package:     target.Package,
				Version:     target.Version,
				PublishedAt: publishedAt,
			}
		}
	}
	if pkg.PublishedAt.IsZero() {
		var err error
		pkg, err = p.ecosystem.FetchPackageInfo(r.Context(), target.Package, target.Version)
		if err != nil {
			if p.cfg.FailOpen {
				proxyLog("registry unreachable for %s@%s: %v — fail_open=true, allowing", target.Package, target.Version, err)
				p.auditLog.Write(r.Context(), audit.Entry{
					Ecosystem:  p.ecosystem.Name(),
					Package:    target.Package,
					Version:    target.Version,
					Outcome:    audit.OutcomeAllowed,
					Reason:     "registry_unreachable fail_open=true: " + err.Error(),
					ProjectDir: projectDir,
				})
				p.forwardRequest(w, r)
			} else {
				proxyLog("registry unreachable for %s@%s: %v — blocking", target.Package, target.Version, err)
				p.auditLog.Write(r.Context(), audit.Entry{
					Ecosystem:  p.ecosystem.Name(),
					Package:    target.Package,
					Version:    target.Version,
					Outcome:    audit.OutcomeBlocked,
					Reason:     "registry_unreachable fail_open=false: " + err.Error(),
					ProjectDir: projectDir,
				})
				http.Error(w, fmt.Sprintf("pkgfort: could not verify %s@%s: %v\n%s", target.Package, target.Version, err, bypassHint), http.StatusForbidden)
			}
			return
		}
		b, _ := json.Marshal(pkg.PublishedAt)
		p.cache.Set(r.Context(), cacheKey, cache.Entry{"published_at": b})
	}

	for _, rl := range p.rules {
		result, err := rl.Evaluate(r.Context(), pkg)
		if err != nil {
			if p.cfg.FailOpen {
				proxyLog("rule %s: error (%v) — fail_open=true, skipping", rl.Name(), err)
				continue
			}
			proxyLog("rule %s: error (%v) — blocking", rl.Name(), err)
			http.Error(w, fmt.Sprintf("pkgfort: rule %s error: %v", rl.Name(), err), http.StatusForbidden)
			return
		}
		if !result.Passed {
			proxyLog("rule %s: fail — %s", rl.Name(), result.Reason)
			proxyBlockLog("block %s@%s — %s", target.Package, target.Version, result.Reason)
			p.auditLog.Write(r.Context(), audit.Entry{
				Ecosystem:  p.ecosystem.Name(),
				Package:    target.Package,
				Version:    target.Version,
				Outcome:    audit.OutcomeBlocked,
				Rule:       rl.Name(),
				Reason:     result.Reason,
				ProjectDir: projectDir,
			})
			http.Error(w, fmt.Sprintf("BLOCKED: %s@%s — %s\n%s", target.Package, target.Version, result.Reason, bypassHint), http.StatusForbidden)
			return
		}
		proxyLog("rule %s: pass — %s", rl.Name(), result.Reason)
	}

	proxyLog("allow %s@%s", target.Package, target.Version)
	p.auditLog.Write(r.Context(), audit.Entry{
		Ecosystem:  p.ecosystem.Name(),
		Package:    target.Package,
		Version:    target.Version,
		Outcome:    audit.OutcomeAllowed,
		Reason:     "all rules passed",
		ProjectDir: projectDir,
	})
	p.forwardRequest(w, r)
}

const bypassHint = "To bypass pkgfort, set PKGFORT_SKIP=1 before your command."

func (p *Proxy) forwardRequest(w http.ResponseWriter, r *http.Request) {
	upstreams := p.ecosystem.UpstreamURLs()
	var lastStatus int
	for _, upstream := range upstreams {
		done, status := p.tryUpstream(w, r, upstream)
		if done {
			return
		}
		lastStatus = status
	}
	// All upstreams returned 404/410. For Go modules this usually means the
	// module is only available via direct VCS access, which pkgfort cannot
	// proxy. The user must bypass explicitly.
	msg := fmt.Sprintf(
		"pkgfort: %s not found in any configured proxy.\n%s",
		r.URL.Path, bypassHint,
	)
	if lastStatus == http.StatusGone {
		http.Error(w, msg, http.StatusGone)
	} else {
		http.Error(w, msg, http.StatusNotFound)
	}
}

// tryUpstream forwards the request to a single upstream.
// Returns (true, _) if the response was written to w (caller must stop).
// Returns (false, status) if the upstream returned 404/410 and the caller should try the next.
func (p *Proxy) tryUpstream(w http.ResponseWriter, r *http.Request, upstream string) (done bool, status int) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream+r.URL.RequestURI(), r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("pkgfort: %v\n%s", err, bypassHint), http.StatusBadGateway)
		return true, 0
	}
	for k, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("pkgfort: upstream unreachable: %v\n%s", err, bypassHint), http.StatusBadGateway)
		return true, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return false, resp.StatusCode
	}

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	return true, 0
}

func proxyLog(format string, args ...any) {
	if !pkgenv.Verbose() {
		return
	}
	fmt.Fprintf(os.Stderr, "[pkgfort] "+format+"\n", args...)
}

func proxyBlockLog(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[pkgfort] "+format+"\n", args...)
}

func (p *Proxy) isAllowlisted(pkg string) bool {
	for _, entry := range p.cfg.Allowlist {
		if entry.Ecosystem == p.ecosystem.Name() && entry.Package == pkg {
			return true
		}
	}
	return false
}

// AllRules returns every known rule regardless of enabled_rules, used for CLI dispatch.
func AllRules(cfg config.Config, httpClient *http.Client) []rule.Rule {
	return []rule.Rule{
		rule.NewMinAgeDays(cfg.Rules.MinAgeDays.Days),
		rule.NewVulnCheck(osv.New(httpClient)),
	}
}

func BuildRules(cfg config.Config, httpClient *http.Client) []rule.Rule {
	enabledSet := make(map[string]bool, len(cfg.EnabledRules))
	for _, name := range cfg.EnabledRules {
		enabledSet[name] = true
	}
	var enabled []rule.Rule
	for _, r := range AllRules(cfg, httpClient) {
		if enabledSet[r.Name()] {
			enabled = append(enabled, r)
		}
	}
	return enabled
}
