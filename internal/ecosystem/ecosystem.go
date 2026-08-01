package ecosystem

import (
	"context"
	"net/http"
	"time"
)


// ValidationTargetResult describes whether an incoming request needs validation,
// and if so, which package and version it targets.
type ValidationTargetResult struct {
	Package string
	Version string
	// NeedsValidation is false for requests that should be proxied transparently
	// (tarballs, zip files, etc.).
	NeedsValidation bool
}

// Ecosystem abstracts a package registry. Implement this interface to support
// a new ecosystem (npm, Go modules, PyPI, Cargo, etc.).
type Ecosystem interface {
	// Name returns the identifier used in config, cache keys, and audit log.
	Name() string
	// UpstreamURLs returns the ordered list of upstream registry URLs to try.
	// For most ecosystems this is a single entry; Go modules may have several.
	UpstreamURLs() []string
	// NeedsProxy reports whether this CLI invocation downloads packages and
	// should be intercepted by the validation proxy.
	NeedsProxy(args []string) bool
	// ValidationTarget inspects an incoming request and returns whether it
	// requires validation and which package/version it targets.
	ValidationTarget(r *http.Request) ValidationTargetResult
	// FetchPackageInfo fetches metadata for the given package version from the
	// upstream registry.
	FetchPackageInfo(ctx context.Context, pkg, version string) (PackageInfo, error)
}

// PackageInfo holds the resolved metadata for a package version, passed to rules.
type PackageInfo struct {
	Ecosystem   string
	Package     string
	Version     string
	PublishedAt time.Time
}
