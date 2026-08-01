package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/yairp7/pkgfort/internal/audit"
	"github.com/yairp7/pkgfort/internal/cache"
	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/ecosystem/gomod"
	"github.com/yairp7/pkgfort/internal/ecosystem/npm"
	"github.com/yairp7/pkgfort/internal/proxy"
	"github.com/yairp7/pkgfort/internal/rule"
)

func runProxy(ctx context.Context, args []string) {
	if len(args) == 0 {
		proxyUsage()
		os.Exit(1)
	}

	if args[0] == "--ecosystem" {
		runProxyServer(ctx, args)
		return
	}

	runRuleCommand(args)
}

func runProxyServer(ctx context.Context, args []string) {
	if len(args) < 2 {
		proxyUsage()
		os.Exit(1)
	}
	ecosystemName := args[1]

	cfg, err := config.Load()
	if err != nil {
		fatalf("failed to load config: %v", err)
	}

	appDir := filepath.Join(os.Getenv("HOME"), "."+binName)
	c, err := cache.New(filepath.Join(appDir, "cache.json"))
	if err != nil {
		fatalf("failed to load cache: %v", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	rules := proxy.BuildRules(cfg, httpClient)
	auditLog := audit.New(filepath.Join(appDir, "audit.log"))

	switch ecosystemName {
	case "npm":
		p := proxy.New(npm.New(httpClient), rules, c, auditLog, httpClient, cfg)
		if err := p.Start(ctx); err != nil {
			fatalf("proxy error: %v", err)
		}
	case "go":
		p := proxy.New(gomod.New(httpClient), rules, c, auditLog, httpClient, cfg)
		if err := p.Start(ctx); err != nil {
			fatalf("proxy error: %v", err)
		}
	default:
		fatalf("unknown ecosystem: %s", ecosystemName)
	}
}

func runRuleCommand(args []string) {
	ruleName := args[0]

	cfg, err := config.Load()
	if err != nil {
		fatalf("failed to load config: %v", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	allRules := proxy.AllRules(cfg, httpClient)

	// Validate rule name.
	var matched rule.Rule
	for _, r := range allRules {
		if r.Name() == ruleName {
			matched = r
			break
		}
	}
	if matched == nil {
		fmt.Fprintf(os.Stderr, "unknown rule %q\n\n", ruleName)
		printRuleCommands(allRules)
		os.Exit(1)
	}

	configurable, ok := matched.(rule.Configurable)
	if !ok {
		fmt.Fprintf(os.Stderr, "rule %q has no configurable commands\n\n", ruleName)
		printRuleCommands(allRules)
		os.Exit(1)
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s proxy %s <cmd> [args]\navailable commands:\n", binName, ruleName)
		for _, c := range configurable.Commands() {
			fmt.Fprintf(os.Stderr, "  %s\n", c.Usage)
		}
		os.Exit(1)
	}
	cmdName, cmdArgs := args[1], args[2:]

	cmds := configurable.Commands()
	cmd, ok := cmds[cmdName]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q for rule %q\navailable commands:\n", cmdName, ruleName)
		for _, c := range cmds {
			fmt.Fprintf(os.Stderr, "  %s\n", c.Usage)
		}
		os.Exit(1)
	}

	if err := cmd.Run(&cfg, cmdArgs); err != nil {
		fatalf("%v", err)
	}
	if err := cfg.Save(); err != nil {
		fatalf("failed to save config: %v", err)
	}
	fmt.Println("ok")
}

func proxyUsage() {
	fmt.Fprintf(os.Stderr, "usage:\n")
	fmt.Fprintf(os.Stderr, "  %s proxy --ecosystem <npm|go>\n", binName)
	fmt.Fprintf(os.Stderr, "  %s proxy <rule> <cmd> [args]\n\n", binName)

	cfg, _ := config.Load()
	printRuleCommands(proxy.AllRules(cfg, &http.Client{Timeout: 30 * time.Second}))
}

func printRuleCommands(rules []rule.Rule) {
	fmt.Fprintln(os.Stderr, "rules:")
	for _, r := range rules {
		configurable, ok := r.(rule.Configurable)
		if !ok {
			continue
		}
		for _, cmd := range configurable.Commands() {
			fmt.Fprintf(os.Stderr, "  %s %s\n", r.Name(), cmd.Usage)
		}
	}
}
