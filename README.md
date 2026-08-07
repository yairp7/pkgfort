# Package Fort

A local proxy service that validates npm and Go packages against configurable security rules before allowing them to be installed — without changing your existing workflow.

---

## The Problem

Supply chain attacks increasingly exploit the window between a package being published and the community noticing something is wrong. A malicious actor can publish a typosquatted or hijacked package and have it installed by thousands of developers within minutes. Most of these attacks happen with packages that are brand new — published hours or days before the attack is discovered.

The simplest possible defense: **don't install packages that were published less than a week ago.** By the time a week has passed, the community has usually had a chance to audit, flag, or pull the package.

---

## How It Works

When you run `npm install` or `go get`, a shell wrapper intercepts the command and:

1. Starts a local HTTP proxy server (bound to a random free port)
2. Sets the appropriate registry/proxy env vars pointing to `localhost`
3. Runs the real `npm`/`go` binary with those env vars
4. The proxy validates each package against configured rules before forwarding requests to the real registry
5. Blocked packages receive a `403` with a human-readable reason; allowed packages are proxied transparently
6. When the command exits, the proxy is shut down

For commands that don't fetch packages (`npm test`, `go build`, etc.), the wrapper passes through directly to the real binary — zero overhead.

```
npm install some-lib
      │
      ▼
 shell wrapper (pkgfort exec npm install some-lib)
      │
      ├─ starts pkgfort-proxy on :random port
      │
      ▼
 npm ──► http://localhost:PORT/some-lib
             │
             ▼
        pkgfort-proxy
             │
             ├─ check cache: ~/.pkgfort/cache.json
             ├─ fetch publish time from registry.npmjs.org
             ├─ apply rules (e.g. min_age_days: 7)
             │
             ├─ PASS ──► proxy to registry.npmjs.org ──► npm
             └─ FAIL ──► 403 "some-lib@2.1.0 published 2 days ago, minimum age is 7 days"
```

---

## Architecture

### Components

| Component | Description |
|-----------|-------------|
| `pkgfort` binary | Single Go binary: runs as proxy server (`pkgfort proxy`), command wrapper (`pkgfort exec`), and installer (`pkgfort install`) |
| `~/.pkgfort/init.sh` | Shell functions that intercept `npm` and `go` commands |
| `~/.pkgfort/config.json` | Global configuration (rules, allowlist, fail mode) |
| `~/.pkgfort/cache.json` | Immutable publish-timestamp cache keyed by `ecosystem:package@version` |
| `~/.pkgfort/audit.log` | JSONL audit trail of every allow and block decision |

### Binary Structure

```
pkgfort proxy --ecosystem <npm|go>    # start the proxy server (used internally by the wrapper)
pkgfort proxy <rule> <cmd> [args]     # configure a rule (e.g. pkgfort proxy min_age_days set-days 14)
pkgfort exec <tool>                   # intercept-or-passthrough wrapper (called by shell functions)
pkgfort install                       # install shell init file and create default config
pkgfort uninstall                     # remove ~/.pkgfort and the init.sh source line from shell rc files
pkgfort config                        # print current configuration as JSON
pkgfort version                       # print version
```

### Proxy Protocols Implemented

**npm** (proxies to `https://registry.npmjs.org` by default; respects `npm_config_registry` env var and `.npmrc`):
- `GET /:package` — full metadata; transparent proxy (version not yet known)
- `GET /:package/:version` — version metadata; validation happens here
- `GET /:package/-/:package-:version.tgz` — tarball; validation happens here

**Go module proxy** (proxies to `https://proxy.golang.org` by default; respects the `GOPROXY` env var including comma-separated fallback chains):
- `GET /:module/@v/:version.info` — version info with `Time` field; validation happens here
- `GET /:module/@v/:version.mod` — validation happens here
- `GET /:module/@v/:version.zip` — validation happens here
- `GET /:module/@v/list` — transparent proxy
- `GET /:module/@latest` — transparent proxy

---

## Design Decisions

These decisions were made deliberately — the rationale is documented here because the "why" matters as much as the "what."

### Ephemeral proxy per invocation (not a persistent daemon)

The proxy starts and stops with each `npm install` / `go get` invocation rather than running as a background daemon. This keeps the tool self-contained — no service management, no `launchd`/`systemd` setup, no state to worry about if the machine crashes. The downside is a small startup cost per invocation (~milliseconds for a Go HTTP server to bind a port), which is negligible compared to the network time of an actual package install.

### Port selection via `:0` binding

The proxy binds to port `0`, which lets the OS assign a free port atomically. It then prints `READY port=N` to stdout, and the wrapper reads that line before proceeding. This eliminates any TOCTOU race between "find a free port" and "bind to it", and allows multiple concurrent invocations to coexist without coordination (each gets its own proxy on its own port).

### Intercept only fetch subcommands

The wrapper checks the first argument and only starts the proxy for commands that actually download packages:

| Tool | Intercepted | Passed through |
|------|-------------|----------------|
| npm | `install`, `ci`, `update`, `i` | `run`, `test`, `build`, `publish`, … |
| go | `get`, `install`, `mod tidy`, `mod download` | `build`, `test`, `run`, `fmt`, … |

Everything else executes the real binary directly with no proxy overhead.

### Validate at metadata and content endpoints

For npm, validation runs at `GET /:package/:version` and `GET /:package/-/:pkg-:version.tgz`. Unversioned metadata requests (`GET /:package`) are proxied transparently because the version being installed is not yet known. Validation always fires before any bytes of source are downloaded.

For Go modules, validation runs at `.info`, `.mod`, and `.zip`. All three are gated because Go's download sequence can succeed partially: `.info` carries the publish timestamp used for validation, but `.mod` is needed for dependency resolution and `.zip` for the source itself. Blocking only `.info` allows Go to still fetch `.mod` and populate `go.sum` via the checksum database. The publish-time cache means the upstream is only queried once per `package@version` regardless of how many endpoints are hit.

### Fail closed by default

If the proxy cannot reach the upstream registry to check the publish timestamp (network issue, API down, timeout), it blocks the request by default. A security tool that silently bypasses its own checks on error is not a security tool. The behavior is configurable via `fail_open: true` in config for offline/air-gapped environments.

### File-based publish timestamp cache

Package version publish timestamps are immutable — `some-lib@2.0.0` will always have been published on the same date. The cache at `~/.pkgfort/cache.json` stores `"ecosystem:package@version" → "RFC3339 timestamp"` with no expiry. After the first install of a package version, all future installs are validated from a local file lookup with no network round-trip. Concurrent write collisions are accepted as a known trade-off (worst case: a cache miss causing a redundant API call, not data loss or a security bypass).

### Two bypass mechanisms

**Allowlist** (`config.json`): for packages you permanently trust, such as internal packages or ones you've manually audited. Survives across invocations.

**`PKGFORT_SKIP=1` env var**: for one-off bypasses when you've made a deliberate decision to install something that doesn't meet the rules. `PKGFORT_SKIP=1` is checked in the `exec` wrapper before the proxy starts, so the command passes through directly with no audit log entry. Allowlist bypasses, by contrast, are logged to `audit.log` with `"outcome":"bypassed"`.

### Audit log as JSONL

Every decision (allow, block, bypass) is appended to `~/.pkgfort/audit.log` as a single JSON line with: timestamp, ecosystem, package, version, rule that triggered, outcome, reason, and project CWD. Append-only writes below `PIPE_BUF` are atomic on POSIX, making concurrent writes from multiple proxy processes safe without locking.

### JSON config with stdlib only

No third-party dependencies. Go's `encoding/json` handles everything. This is a security tool — introducing dependencies to manage dependencies would be self-defeating.

---

## File Layout

```
~/.pkgfort/
├── config.json       # configuration
├── cache.json        # publish timestamp cache
├── audit.log         # JSONL audit trail
└── init.sh           # shell functions sourced from .zshrc
```

`cache.json` grows unbounded over time (publish timestamps are immutable, so there's no expiry). It's safe to delete at any point — the next install will repopulate it from upstream.

### `config.json` schema

```json
{
  "_v": 1,
  "enabled_rules": ["min_age_days", "vuln_check"],
  "rules": {
    "min_age_days": { "days": 7 },
    "vuln_check": {}
  },
  "fail_open": false,
  "allowlist": [
    { "ecosystem": "npm", "package": "@myorg/internal-lib" },
    { "ecosystem": "go", "package": "github.com/myorg/private-module" }
  ]
}
```

`_v` is the config schema version. `enabled_rules` controls which rules are active — remove a rule name to disable it without deleting its configuration.

### `audit.log` entry format

```jsonl
{"ts":"2026-05-21T10:00:00Z","ecosystem":"npm","package":"some-lib","version":"2.1.0","outcome":"blocked","rule":"min_age_days","reason":"published 2 day(s) ago, minimum age is 7","project":"/home/user/workspace/my-app"}
{"ts":"2026-05-21T10:01:00Z","ecosystem":"npm","package":"some-lib","version":"2.0.0","outcome":"allowed","rule":"","reason":"all rules passed","project":"/home/user/workspace/my-app"}
{"ts":"2026-05-21T10:02:00Z","ecosystem":"npm","package":"@myorg/internal","version":"1.0.0","outcome":"bypassed","rule":"","reason":"allowlist","project":"/home/user/workspace/my-app"}
```

---

## Code Architecture

### Core Interfaces

The project is built around two extension points. Adding a new ecosystem or a new rule means implementing one interface — nothing else changes.

**`Ecosystem`** — one implementation per package registry (`internal/ecosystem/ecosystem.go`):

```go
type Ecosystem interface {
    // Name returns the identifier used in config, cache keys, and audit log ("npm", "go").
    Name() string
    // UpstreamURLs returns the ordered list of upstream registry URLs to try.
    UpstreamURLs() []string
    // ValidationTarget inspects an incoming request and returns whether it
    // requires validation and which package/version it targets.
    ValidationTarget(r *http.Request) ValidationTargetResult
    // FetchPackageInfo fetches metadata for the given package version from the
    // upstream registry.
    FetchPackageInfo(ctx context.Context, pkg, version string) (PackageInfo, error)
}

type PackageInfo struct {
    Ecosystem   string
    Package     string
    Version     string
    PublishedAt time.Time
}
```

**`Rule`** — one implementation per validation rule (`internal/rule/rule.go`):

```go
type Rule interface {
    // Name returns the rule identifier used in audit log and error messages.
    Name() string
    // Evaluate runs the rule against a resolved package. Returns a Result
    // indicating pass/fail and a human-readable reason.
    Evaluate(ctx context.Context, pkg ecosystem.PackageInfo) (Result, error)
}

type Result struct {
    Passed bool
    Reason string
}
```

**`Configurable`** — optional interface rules can implement to expose CLI commands via `pkgfort proxy <rule> <cmd> [args]`:

```go
type Command struct {
    Usage string
    Run   func(cfg *config.Config, args []string) error
}

type Configurable interface {
    Commands() map[string]Command
}
```

`min_age_days` implements this with a `set-days <n>` command that writes to `~/.pkgfort/config.json`:

```bash
pkgfort proxy min_age_days set-days 14
# ok
```

### Dependency Injection

No global state. All dependencies are wired explicitly at the `cmd/pkgfort` layer and injected via constructors.

Each `pkgfort exec` invocation starts a **single-ecosystem proxy** (`pkgfort proxy --ecosystem npm` or `pkgfort proxy --ecosystem go`). The proxy is pre-configured for one ecosystem — it calls `ecosystem.ValidationTarget(r)` directly with no routing step. This means the proxy never needs to identify which ecosystem a request belongs to; that's guaranteed by construction.


```
cmd/pkgfort/main.go          # parses subcommand, calls into cmd/pkgfort/{proxy,exec,install,...}.go
cmd/pkgfort/proxy.go         # builds Proxy for a single ecosystem, starts HTTP server, or runs rule CLI
cmd/pkgfort/exec.go          # builds Executor, decides intercept vs passthrough
cmd/pkgfort/install.go       # builds Installer, writes init.sh + config

Wiring for `pkgfort proxy --ecosystem npm`:
    └── Config, Cache, AuditLog, HTTPClient
    └── ecosystem = npm.New(httpClient)
    └── []Rule{rule.NewMinAgeDays(config.Rules.MinAgeDays), rule.NewVulnCheck(osv.New(httpClient))}
    └── Proxy{ecosystem, rules, cache, auditLog, httpClient, cfg}
```

### Directory Structure

```
build/                   # compiled binaries (git-ignored)
cmd/
  pkgfort/
    main.go              # entry point, registers subcommands, wires dependencies
    proxy.go             # `pkgfort proxy` — proxy server + rule CLI dispatch
    exec.go              # `pkgfort exec` — intercept-or-passthrough wrapper
    install.go           # `pkgfort install` — write init.sh, config defaults, copy binary
    uninstall.go         # `pkgfort uninstall` — remove ~/.pkgfort and strip init.sh source line from shell rc files
    config.go            # `pkgfort config` — print current config as JSON
internal/
  proxy/
    proxy.go             # HTTP server, validation orchestration
  ecosystem/
    ecosystem.go         # Ecosystem interface, ValidationTargetResult, PackageInfo
    npm/
      npm.go             # npm registry handler
    gomod/
      gomod.go           # Go module proxy handler
  rule/
    rule.go              # Rule interface + Result type
    minagedays.go        # min_age_days rule implementation
    vuln.go              # vuln_check rule + VulnChecker interface
    osv/
      osv.go             # OSV.dev vulnerability checker
  cache/
    cache.go             # ~/.pkgfort/cache.json read/write
  audit/
    audit.go             # ~/.pkgfort/audit.log JSONL writer
  config/
    config.go            # ~/.pkgfort/config.json loader
  env/
    env.go               # PKGFORT_* env var helpers (PKGFORT_SKIP, PKGFORT_VERBOSE)
  exec/
    exec.go              # ecosystem/tool mapping, NeedsProxy intercept logic
```

### Testing Approach

Unit tests cover rule evaluation logic, ecosystem URL parsing, and cache read/write. They are written selectively where the behavior is well-defined and the test adds durable value.

End-to-end tests live in `test/` and run inside Docker via `make test-docker`. Each test copies a config fixture into `~/.pkgfort/config.json`, runs the real tool through the shell wrapper, and asserts on exit code and stderr output.

**npm** — four version specifier forms are tested, both block and pass:

| Specifier | Project dir | Block assertion |
|-----------|------------|-----------------|
| `4.17.21` (exact) | `npm-project/` | `block lodash@4.17.21` |
| `^4.0.0` (caret) | `npm-project-caret/` | `block lodash@` |
| `~4.17.0` (tilde) | `npm-project-tilde/` | `block lodash@` |
| `*` (latest) | `npm-project-latest/` | `block lodash@` |

Caret, tilde, and latest assertions omit the specific version because the resolved version depends on the registry state at test time. The exact-version case can be specific because the lock file pins the resolved version.

**Go** — one scenario using `go mod download` with an exact version pin (`github.com/google/uuid v1.6.0`), both block and pass.

`config-block.json` sets `min_age_days: 99999` (blocks everything); `config-pass.json` sets `min_age_days: 0` (allows everything).

---

## Building

**Prerequisites:** Go 1.26+

```bash
git clone https://github.com/yairp7/pkgfort.git
cd pkgfort
go build -o pkgfort ./cmd/pkgfort
```

Or with make:

```bash
make build       # builds ./build/pkgfort
make install     # builds and runs pkgfort install
make test-docker # runs integration tests in Docker
```

---

## Installation

**Option 1 — install script (recommended):**

```bash
curl -fsSL https://raw.githubusercontent.com/yairp7/pkgfort/main/install.sh | sh
```

Downloads a prebuilt binary for your OS/arch from the [latest release](https://github.com/yairp7/pkgfort/releases), falling back to `go install` if no matching release binary exists (e.g. on an unsupported platform) — Go must be installed for the fallback. Then runs `pkgfort install` and wires up your shell rc file automatically. Pin a version with `PKGFORT_VERSION=v1.0.0`.

**Option 2 — `go install` (no clone needed):**

```bash
go install github.com/yairp7/pkgfort/cmd/pkgfort@latest
pkgfort install
```

**Option 3 — build from source:**

```bash
./pkgfort install
```

This:
1. Copies the binary to `~/.pkgfort/pkgfort`
2. Writes `~/.pkgfort/init.sh` with the shell wrapper functions (using the full binary path) and adds `~/.pkgfort` to `PATH` so the `pkgfort` command itself is directly accessible
3. Creates `~/.pkgfort/config.json` with defaults if it doesn't exist
4. Prints the line to add to your shell config

Then add to `~/.zshrc` (or `~/.bashrc`):

```bash
source ~/.pkgfort/init.sh
```

Reload your shell:

```bash
source ~/.zshrc
```

---

## Usage

After installation, `npm` and `go` commands work exactly as before. When pkgfort intercepts a fetch command, it prints a brief animated banner to stderr (`🛡️ pkgfort activated (v1.0.0) 🛡️`). Blocks are always printed to stderr; per-request progress lines require `PKGFORT_VERBOSE=1`:

```
$ PKGFORT_VERBOSE=1 go get github.com/aws/aws-sdk-go-v2/aws
[pkgfort] starting go proxy
[pkgfort] proxy ready on :56199
[pkgfort] checking github.com/aws/aws-sdk-go-v2@v1.41.7
[pkgfort] rule min_age_days: pass — published 21 day(s) ago, minimum age is 7
[pkgfort] allow github.com/aws/aws-sdk-go-v2@v1.41.7
go: added github.com/aws/aws-sdk-go-v2 v1.41.7
```

```bash
# Blocked — package too new
npm install some-new-package@1.0.0
# [pkgfort] rule min_age_days: fail — published 1 day(s) ago, minimum age is 7
# error: BLOCKED: some-new-package@1.0.0 — published 1 day(s) ago, minimum age is 7

# Allowed — package is old enough
npm install some-lib@2.0.0
# (installs normally)

# One-off bypass
PKGFORT_SKIP=1 npm install some-new-package@1.0.0

# Verbose mode — print full per-request detail to stderr
PKGFORT_VERBOSE=1 npm install some-lib@2.0.0

# Go modules
go get github.com/some/module@v1.0.0
```

**Note on Go module cache:** Go caches proxy responses in `$GOPATH/pkg/mod/cache`. If a module version was downloaded before pkgfort was installed, Go won't re-fetch the `.info` file and validation won't run for that cached version. Validation always runs for module versions being downloaded for the first time.

**Known limitation — `go.sum` and the checksum database:** Even when a Go module is blocked, Go may still contact `sum.golang.org` directly — the checksum database is not routed through `GOPROXY` and is outside pkgfort's control. This can result in `go.sum` being updated with the expected hash of a blocked module. The practical impact is limited: the module source (`.zip`) is blocked so the module cannot be built or used. The `go.sum` entry is inert without the accompanying source download. Setting `GONOSUMDB=*` would prevent this but also disables checksum verification for allowed modules, which is a net security regression.

**Known limitation — runtime registry overrides:** pkgfort resolves the upstream registry once at proxy startup, from `npm_config_registry` / `GOPROXY` env vars or `.npmrc` / `go env GOPROXY`. Per-invocation flags like `npm install --registry=...` are *not* honored — npm passes the flag to the proxied npm, but the proxy itself has already bound to its own upstream. If you need a different registry, set it via env var or `.npmrc` before the command.

---

## Rules

### `min_age_days` (implemented)

Blocks any package version published less than N days ago. Configurable in `config.json`.

**Data source:**
- npm: `https://registry.npmjs.org/<package>` → `time["<version>"]`
- Go: `https://proxy.golang.org/<module>/@v/<version>.info` → `Time`

### `vuln_check` (implemented)

Blocks any package version with known vulnerabilities reported by a vulnerability database. Queries [OSV.dev](https://osv.dev) by default. Unknown ecosystems are silently skipped.

The rule accepts multiple `VulnChecker` implementations — adding a future source (e.g. GitHub Advisory) requires only implementing the interface and passing it to `NewVulnCheck`.

**Data source:**
- `https://api.osv.dev/v1/query` — POST `{package, version, ecosystem}`, returns matching vuln IDs

**Audit log example:**
```jsonl
{"ts":"…","ecosystem":"npm","package":"lodash","version":"4.17.4","outcome":"blocked","rule":"vuln_check","reason":"osv.dev: 4 known vuln(s): GHSA-jf85-cpcp-j695, …"}
```

### Adding a new rule

**1. Implement the `Rule` interface** — create `internal/rule/myrule.go`:

```go
package rule

import (
    "context"
    "github.com/yairp7/pkgfort/internal/ecosystem"
)

type MyRule struct{ /* config fields */ }

func NewMyRule( /* config */ ) *MyRule { return &MyRule{} }

func (r *MyRule) Name() string { return "my_rule" }

func (r *MyRule) Evaluate(_ context.Context, pkg ecosystem.PackageInfo) (Result, error) {
    if /* condition */ {
        return Result{Passed: false, Reason: "..."}, nil
    }
    return Result{Passed: true, Reason: "..."}, nil
}
```

**2. Add config** — in `internal/config/config.go`, add a config struct and wire it into `Rules` and `Default()`:

```go
type MyRule struct {
    SomeField string `json:"some_field"`
}

type Rules struct {
    MinAgeDays MinAgeDaysRule `json:"min_age_days"`
    VulnCheck  VulnCheckRule  `json:"vuln_check"`
    MyRule     MyRule         `json:"my_rule"`    // add this
}
```

**3. Register in `AllRules`** — in `internal/proxy/proxy.go`:

```go
func AllRules(cfg config.Config, httpClient *http.Client) []rule.Rule {
    return []rule.Rule{
        rule.NewMinAgeDays(cfg.Rules.MinAgeDays.Days),
        rule.NewVulnCheck(osv.New(httpClient)),
        rule.NewMyRule(cfg.Rules.MyRule.SomeField),    // add this
    }
}
```

**4. Enable in config** — add `"my_rule"` to `enabled_rules` in `~/.pkgfort/config.json`.

That's it — `BuildRules` filters the `all` slice by `enabled_rules`, so the rule is active only when listed there.

---

## Uninstalling

```bash
pkgfort uninstall
```

Removes `~/.pkgfort` entirely (binary, `init.sh`, `config.json`, `audit.log`) and strips the `source ~/.pkgfort/init.sh` line (and its `# pkgfort` marker) from `~/.zshrc` and `~/.bashrc`. Restart your shell, or open a new terminal, to pick up the change.
