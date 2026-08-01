package exec

import (
	"fmt"
	"strings"

	"github.com/yairp7/pkgfort/internal/ecosystem"
	"github.com/yairp7/pkgfort/internal/ecosystem/gomod"
	"github.com/yairp7/pkgfort/internal/ecosystem/npm"
)

// execEcosystems maps each supported tool name to a lightweight Ecosystem
// instance used only for exec-side subcommand detection (no HTTP client needed).
var execEcosystems = map[string]ecosystem.Ecosystem{
	"npm": npm.NewExec(),
	"go":  gomod.NewExec(),
}

// EcosystemForTool maps a tool name to its pkgfort ecosystem identifier.
// Returns ("", false) for unrecognised tools.
func EcosystemForTool(tool string) (string, bool) {
	if eco, ok := execEcosystems[tool]; ok {
		return eco.Name(), true
	}
	return "", false
}

// NeedsProxy reports whether a tool invocation downloads packages and should
// be routed through the validation proxy.
func NeedsProxy(tool string, args []string) bool {
	if eco, ok := execEcosystems[tool]; ok {
		return eco.NeedsProxy(args)
	}
	return false
}

// EnvWithProxy returns env with the appropriate proxy variable set for tool at
// port. Any pre-existing value for that variable is replaced.
func EnvWithProxy(env []string, tool string, port int) []string {
	var key, value string
	switch tool {
	case "npm":
		key = "npm_config_registry"
		value = fmt.Sprintf("npm_config_registry=http://127.0.0.1:%d", port)
	case "go":
		key = "GOPROXY"
		value = fmt.Sprintf("GOPROXY=http://127.0.0.1:%d", port)
	default:
		return env
	}

	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return append(result, value)
}

