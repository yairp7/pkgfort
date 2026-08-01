package rule

import (
	"context"
	"fmt"
	"strings"

	"github.com/yairp7/pkgfort/internal/ecosystem"
)

// VulnChecker queries a vulnerability database for known issues affecting a package version.
type VulnChecker interface {
	// Name identifies the data source used in audit/reason strings.
	Name() string
	// Check returns the IDs of known vulnerabilities, or an empty slice if none found.
	Check(ctx context.Context, pkg ecosystem.PackageInfo) ([]string, error)
}

// VulnCheck blocks packages with known vulnerabilities reported by any of its checkers.
type VulnCheck struct {
	checkers []VulnChecker
}

func NewVulnCheck(checkers ...VulnChecker) *VulnCheck {
	return &VulnCheck{checkers: checkers}
}

func (v *VulnCheck) Name() string { return "vuln_check" }

func (v *VulnCheck) Evaluate(ctx context.Context, pkg ecosystem.PackageInfo) (Result, error) {
	checkerNames := make([]string, len(v.checkers))
	for i, c := range v.checkers {
		checkerNames[i] = c.Name()

		ids, err := c.Check(ctx, pkg)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", c.Name(), err)
		}

		if len(ids) == 0 {
			continue
		}

		return Result{
			Passed: false,
			Reason: fmt.Sprintf("%s: %d known vuln(s): %s", c.Name(), len(ids), strings.Join(ids, ", ")),
		}, nil
	}

	return Result{
		Passed: true,
		Reason: fmt.Sprintf("no known vulnerabilities found in %s", strings.Join(checkerNames, ", ")),
	}, nil
}
