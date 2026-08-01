package exec_test

import (
	"strings"
	"testing"

	scmexec "github.com/yairp7/pkgfort/internal/exec"
)

func TestEcosystemForTool(t *testing.T) {
	tests := []struct {
		tool      string
		wantEco   string
		wantFound bool
	}{
		{"npm", "npm", true},
		{"go", "go", true},
		{"pip", "", false},
		{"cargo", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			eco, found := scmexec.EcosystemForTool(tt.tool)
			if eco != tt.wantEco || found != tt.wantFound {
				t.Errorf("EcosystemForTool(%q) = (%q, %v), want (%q, %v)", tt.tool, eco, found, tt.wantEco, tt.wantFound)
			}
		})
	}
}

func TestNeedsProxy(t *testing.T) {
	tests := []struct {
		tool string
		args []string
		want bool
	}{
		// npm — intercept
		{"npm", []string{"install"}, true},
		{"npm", []string{"install", "react"}, true},
		{"npm", []string{"i", "react"}, true},
		{"npm", []string{"ci"}, true},
		{"npm", []string{"update"}, true},
		// npm — pass through
		{"npm", []string{"run", "build"}, false},
		{"npm", []string{"test"}, false},
		{"npm", []string{"publish"}, false},
		{"npm", []string{}, false},
		// go — intercept
		{"go", []string{"get", "github.com/foo/bar"}, true},
		{"go", []string{"install", "github.com/foo/bar@latest"}, true},
		{"go", []string{"mod", "tidy"}, true},
		{"go", []string{"mod", "download"}, true},
		// go — pass through
		{"go", []string{"build", "./..."}, false},
		{"go", []string{"test", "./..."}, false},
		{"go", []string{"fmt", "./..."}, false},
		{"go", []string{"mod", "verify"}, false},
		{"go", []string{}, false},
	}
	for _, tt := range tests {
		name := tt.tool + " " + strings.Join(tt.args, " ")
		t.Run(name, func(t *testing.T) {
			got := scmexec.NeedsProxy(tt.tool, tt.args)
			if got != tt.want {
				t.Errorf("NeedsProxy(%q, %v) = %v, want %v", tt.tool, tt.args, got, tt.want)
			}
		})
	}
}

func TestEnvWithProxy(t *testing.T) {
	base := []string{"HOME=/home/user", "PATH=/usr/bin"}

	t.Run("npm sets registry", func(t *testing.T) {
		env := scmexec.EnvWithProxy(base, "npm", 12345)
		if !contains(env, "npm_config_registry=http://127.0.0.1:12345") {
			t.Errorf("expected npm_config_registry in env, got %v", env)
		}
	})

	t.Run("go sets GOPROXY", func(t *testing.T) {
		env := scmexec.EnvWithProxy(base, "go", 12345)
		if !contains(env, "GOPROXY=http://127.0.0.1:12345") {
			t.Errorf("expected GOPROXY in env, got %v", env)
		}
	})

	t.Run("replaces existing npm registry", func(t *testing.T) {
		existing := append(base, "npm_config_registry=https://old-registry.com")
		env := scmexec.EnvWithProxy(existing, "npm", 12345)
		for _, e := range env {
			if e == "npm_config_registry=https://old-registry.com" {
				t.Error("old registry should have been replaced")
			}
		}
		if !contains(env, "npm_config_registry=http://127.0.0.1:12345") {
			t.Error("new registry should be present")
		}
	})

	t.Run("replaces existing GOPROXY", func(t *testing.T) {
		existing := append(base, "GOPROXY=https://old-proxy.com")
		env := scmexec.EnvWithProxy(existing, "go", 12345)
		for _, e := range env {
			if e == "GOPROXY=https://old-proxy.com" {
				t.Error("old GOPROXY should have been replaced")
			}
		}
	})

	t.Run("unknown tool returns env unchanged", func(t *testing.T) {
		env := scmexec.EnvWithProxy(base, "pip", 12345)
		if len(env) != len(base) {
			t.Errorf("expected env unchanged for unknown tool, got %v", env)
		}
	})
}

func contains(env []string, s string) bool {
	for _, e := range env {
		if e == s {
			return true
		}
	}
	return false
}
