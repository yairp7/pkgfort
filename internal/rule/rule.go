package rule

import (
	"context"

	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/ecosystem"
)

// Result is the outcome of a single rule evaluation.
type Result struct {
	Passed bool
	Reason string
}

// Rule abstracts a validation rule. Implement this interface to add a new rule.
type Rule interface {
	// Name returns the rule identifier used in audit log and error messages.
	Name() string
	// Evaluate runs the rule against a resolved package.
	Evaluate(ctx context.Context, pkg ecosystem.PackageInfo) (Result, error)
}

// Command is a CLI subcommand exposed by a rule via pkgfort proxy <rule> <cmd>.
type Command struct {
	Usage string
	Run   func(cfg *config.Config, args []string) error
}

// Configurable is an optional interface rules can implement to expose CLI
// commands accessible via `pkgfort proxy <rule> <cmd> [args]`.
type Configurable interface {
	Commands() map[string]Command
}
