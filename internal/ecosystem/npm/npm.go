package npm

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

const defaultUpstreamURL = "https://registry.npmjs.org"

// valueFlags lists npm CLI flags that consume the following argument as their
// value (--flag value form, not --flag=value). Knowing these lets us skip
// past them to find the subcommand.
var valueFlags = map[string]bool{
	"--prefix":       true,
	"--registry":     true,
	"--cache":        true,
	"--userconfig":   true,
	"--globalconfig": true,
	"--tag":          true,
	"--workspace":    true,
	"-w":             true,
	"--scope":        true,
	"--proxy":        true,
}

// NewExec returns a lightweight NPM value suitable for exec-side subcommand
// detection. Unlike New, it performs no upstream registry resolution.
func NewExec() *NPM {
	return &NPM{}
}

// NeedsProxy reports whether this npm invocation downloads packages and should
// be routed through the validation proxy.
func (n *NPM) NeedsProxy(args []string) bool {
	sub := firstSubcommand(args)
	return sub == "install" || sub == "i" || sub == "ci" || sub == "update"
}

func firstSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if valueFlags[arg] {
				i++ // skip this flag's value argument
			}
			continue
		}
		return arg
	}
	return ""
}

type Option func(*NPM)

func WithUpstreamURL(url string) Option {
	return func(n *NPM) { n.upstream = url }
}

type NPM struct {
	client   *http.Client
	upstream string
}

func resolveUpstream() string {
	if v := os.Getenv("npm_config_registry"); v != "" {
		return strings.TrimRight(v, "/")
	}
	out, err := osexec.Command("npm", "config", "get", "registry").Output()
	if err != nil {
		return defaultUpstreamURL
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/")
}

func New(client *http.Client, opts ...Option) *NPM {
	n := &NPM{client: client, upstream: resolveUpstream()}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

func (n *NPM) Name() string           { return "npm" }
func (n *NPM) UpstreamURLs() []string { return []string{n.upstream} }

func (n *NPM) ValidationTarget(r *http.Request) ecosystem.ValidationTargetResult {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if path == "" || strings.HasPrefix(path, "-/") {
		return ecosystem.ValidationTargetResult{}
	}

	// Tarball: /:pkg/-/:filename.tgz — version is always known here
	if idx := strings.Index(path, "/-/"); idx >= 0 {
		pkg := path[:idx]
		tarball := strings.TrimSuffix(path[idx+3:], ".tgz")

		// Strip scope from package name to get the tarball prefix (e.g. "@babel/core" → "core")
		base := pkg
		if i := strings.LastIndex(pkg, "/"); i >= 0 {
			base = pkg[i+1:]
		}
		version := strings.TrimPrefix(tarball, base+"-")
		return ecosystem.ValidationTargetResult{Package: pkg, Version: version, NeedsValidation: true}
	}

	// Metadata: /:pkg or /:pkg/:version
	var pkg, version string
	if strings.HasPrefix(path, "@") {
		// Scoped: @scope/name or @scope/name/version
		parts := strings.SplitN(path, "/", 4)
		if len(parts) < 2 {
			return ecosystem.ValidationTargetResult{}
		}
		pkg = parts[0] + "/" + parts[1]
		if len(parts) >= 3 && parts[2] != "" {
			version = parts[2]
		}
	} else {
		parts := strings.SplitN(path, "/", 3)
		pkg = parts[0]
		if len(parts) >= 2 && parts[1] != "" {
			version = parts[1]
		}
	}

	if version == "" {
		return ecosystem.ValidationTargetResult{}
	}
	return ecosystem.ValidationTargetResult{Package: pkg, Version: version, NeedsValidation: true}
}

func (n *NPM) FetchPackageInfo(ctx context.Context, pkg, version string) (ecosystem.PackageInfo, error) {
	url := n.upstream + "/" + pkg
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ecosystem.PackageInfo{}, err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return ecosystem.PackageInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ecosystem.PackageInfo{}, fmt.Errorf("npm registry returned %d for %s", resp.StatusCode, pkg)
	}

	var meta struct {
		Time map[string]string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return ecosystem.PackageInfo{}, err
	}

	ts, ok := meta.Time[version]
	if !ok {
		return ecosystem.PackageInfo{}, fmt.Errorf("version %s not found for package %s", version, pkg)
	}

	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ecosystem.PackageInfo{}, fmt.Errorf("invalid timestamp for %s@%s: %w", pkg, version, err)
	}
	return ecosystem.PackageInfo{
		Ecosystem:   n.Name(),
		Package:     pkg,
		Version:     version,
		PublishedAt: t,
	}, nil
}
