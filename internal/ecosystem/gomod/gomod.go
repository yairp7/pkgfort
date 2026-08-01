package gomod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/yairp7/pkgfort/internal/ecosystem"
)

const defaultUpstreamURL = "https://proxy.golang.org"

// NewExec returns a lightweight GoMod value suitable for exec-side subcommand
// detection. Unlike New, it performs no upstream proxy resolution.
func NewExec() *GoMod {
	return &GoMod{}
}

// NeedsProxy reports whether this go invocation downloads packages and should
// be routed through the validation proxy.
func (g *GoMod) NeedsProxy(args []string) bool {
	sub := firstNonFlag(args)
	if sub == "get" || sub == "install" {
		return true
	}
	if sub == "mod" {
		modSub := firstNonFlag(args[1:])
		return modSub == "tidy" || modSub == "download"
	}
	return false
}

func firstNonFlag(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

type Option func(*GoMod)

func WithUpstreamURLs(urls []string) Option {
	return func(g *GoMod) { g.upstreams = urls }
}

type GoMod struct {
	client    *http.Client
	upstreams []string
}

func resolveUpstreamList() []string {
	proxy := os.Getenv("GOPROXY")
	if proxy == "" {
		out, err := osexec.Command("go", "env", "GOPROXY").Output()
		if err != nil {
			return []string{defaultUpstreamURL}
		}
		proxy = strings.TrimSpace(string(out))
	}
	var urls []string
	for _, entry := range strings.Split(proxy, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" && entry != "direct" && entry != "off" {
			urls = append(urls, entry)
		}
	}
	if len(urls) == 0 {
		return []string{defaultUpstreamURL}
	}
	return urls
}

func New(client *http.Client, opts ...Option) *GoMod {
	g := &GoMod{client: client, upstreams: resolveUpstreamList()}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func (g *GoMod) Name() string           { return "go" }
func (g *GoMod) UpstreamURLs() []string { return g.upstreams }

func (g *GoMod) ValidationTarget(r *http.Request) ecosystem.ValidationTargetResult {
	path := strings.TrimPrefix(r.URL.Path, "/")

	idx := strings.Index(path, "/@v/")
	if idx < 0 {
		return ecosystem.ValidationTargetResult{}
	}

	module := path[:idx]
	rest := path[idx+4:]

	var version string
	switch {
	case strings.HasSuffix(rest, ".info"):
		version = strings.TrimSuffix(rest, ".info")
	case strings.HasSuffix(rest, ".mod"):
		version = strings.TrimSuffix(rest, ".mod")
	case strings.HasSuffix(rest, ".zip"):
		version = strings.TrimSuffix(rest, ".zip")
	default:
		return ecosystem.ValidationTargetResult{}
	}

	return ecosystem.ValidationTargetResult{Package: module, Version: version, NeedsValidation: true}
}

func (g *GoMod) FetchPackageInfo(ctx context.Context, module, version string) (ecosystem.PackageInfo, error) {
	var lastErr error
	for _, upstream := range g.upstreams {
		info, err := g.fetchFromUpstream(ctx, upstream, module, version)
		if err != nil {
			lastErr = err
			continue
		}
		return info, nil
	}
	return ecosystem.PackageInfo{}, lastErr
}

func (g *GoMod) fetchFromUpstream(ctx context.Context, upstream, module, version string) (ecosystem.PackageInfo, error) {
	url := upstream + "/" + module + "/@v/" + version + ".info"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ecosystem.PackageInfo{}, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return ecosystem.PackageInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return ecosystem.PackageInfo{}, fmt.Errorf("go proxy returned %d for %s@%s", resp.StatusCode, module, version)
	}
	if resp.StatusCode != http.StatusOK {
		// Non-404/410 error — don't fall through to next proxy
		return ecosystem.PackageInfo{}, fmt.Errorf("go proxy returned %d for %s@%s (permanent)", resp.StatusCode, module, version)
	}

	var info struct {
		Time time.Time `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ecosystem.PackageInfo{}, err
	}
	return ecosystem.PackageInfo{
		Ecosystem:   g.Name(),
		Package:     module,
		Version:     version,
		PublishedAt: info.Time,
	}, nil
}
