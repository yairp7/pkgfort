package rule_test

import (
	"context"
	"testing"
	"time"

	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/ecosystem"
	"github.com/yairp7/pkgfort/internal/rule"
)

func TestMinAgeDays_ImplementsConfigurable(t *testing.T) {
	// compile-time check via interface variable; runtime guard for clarity
	var r rule.Rule = rule.NewMinAgeDays(7)
	if _, ok := r.(rule.Configurable); !ok {
		t.Fatal("MinAgeDays does not implement Configurable")
	}
}

func TestMinAgeDays_SetDays(t *testing.T) {
	// Call Commands() directly — NewMinAgeDays returns *MinAgeDays which has the method.
	cmds := rule.NewMinAgeDays(7).Commands()
	cmd, ok := cmds["set-days"]
	if !ok {
		t.Fatal("set-days command not registered") // fatal: nothing below is meaningful without cmd
	}

	tests := []struct {
		name    string
		args    []string
		want    int
		wantErr bool
	}{
		{"valid value", []string{"14"}, 14, false},
		{"zero", []string{"0"}, 0, false},
		{"large value", []string{"365"}, 365, false},
		{"no args", []string{}, 0, true},
		{"non-numeric", []string{"abc"}, 0, true},
		{"negative", []string{"-1"}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			err := cmd.Run(&cfg, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil") // error: no further state depends on this
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err) // fatal: Days check below would be misleading
			}
			if cfg.Rules.MinAgeDays.Days != tt.want {
				t.Errorf("Days = %d, want %d", cfg.Rules.MinAgeDays.Days, tt.want)
			}
		})
	}
}

func TestMinAgeDays(t *testing.T) {
	tests := []struct {
		name        string
		days        int
		publishedAt time.Time
		wantPassed  bool
	}{
		{"clearly old enough", 7, time.Now().Add(-30 * 24 * time.Hour), true},
		{"clearly too new", 7, time.Now().Add(-3 * 24 * time.Hour), false},
		{"just over threshold", 7, time.Now().Add(-(7*24 + 1) * time.Hour), true},
		{"just under threshold", 7, time.Now().Add(-6 * 24 * time.Hour), false},
		{"just published", 7, time.Now(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rule.NewMinAgeDays(tt.days)
			pkg := ecosystem.PackageInfo{
				Ecosystem:   "npm",
				Package:     "react",
				Version:     "18.2.0",
				PublishedAt: tt.publishedAt,
			}
			result, err := r.Evaluate(context.Background(), pkg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v (Reason: %q)", result.Passed, tt.wantPassed, result.Reason)
			}
			if result.Reason == "" {
				t.Error("Reason should not be empty")
			}
		})
	}
}
