package rule

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/ecosystem"
)

type MinAgeDays struct {
	days int
}

func NewMinAgeDays(days int) *MinAgeDays {
	return &MinAgeDays{days: days}
}

func (m *MinAgeDays) Name() string { return "min_age_days" }

func (m *MinAgeDays) Commands() map[string]Command {
	return map[string]Command{
		"set-days": {
			Usage: "set-days <n>    set the minimum package age in days",
			Run: func(cfg *config.Config, args []string) error {
				if len(args) < 1 {
					return fmt.Errorf("usage: pkgfort proxy min_age_days set-days <n>")
				}
				n, err := strconv.Atoi(args[0])
				if err != nil || n < 0 {
					return fmt.Errorf("invalid days value: %q", args[0])
				}
				cfg.Rules.MinAgeDays.Days = n
				return nil
			},
		},
	}
}

func (m *MinAgeDays) Evaluate(_ context.Context, pkg ecosystem.PackageInfo) (Result, error) {
	age := int(time.Since(pkg.PublishedAt).Hours() / 24)
	reason := fmt.Sprintf("published %d day(s) ago, minimum age is %d", age, m.days)
	if age < m.days {
		return Result{
			Passed: false,
			Reason: reason,
		}, nil
	}
	return Result{
		Passed: true,
		Reason: reason,
	}, nil
}
