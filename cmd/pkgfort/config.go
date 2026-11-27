package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/yairp7/pkgfort/internal/config"
	"github.com/yairp7/pkgfort/internal/proxy"
	"github.com/yairp7/pkgfort/internal/rule"
)

func runConfig(args []string) {
	if len(args) == 0 {
		runConfigPrint()
		return
	}

	runRuleCommand(args)
}

func runConfigPrint() {
	cfg, err := config.Load()
	if err != nil {
		fatalf("failed to load config: %v", err)
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fatalf("failed to format config: %v", err)
	}
	fmt.Println(string(out))
}

func runRuleCommand(args []string) {
	ruleName := args[0]

	cfg, err := config.Load()
	if err != nil {
		fatalf("failed to load config: %v", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	allRules := proxy.AllRules(cfg, httpClient)

	var matched rule.Rule
	for _, r := range allRules {
		if r.Name() == ruleName {
			matched = r
			break
		}
	}
	if matched == nil {
		fmt.Fprintf(os.Stderr, "unknown rule %q\n\n", ruleName)
		configUsage(allRules)
		os.Exit(1)
	}

	configurable, ok := matched.(rule.Configurable)
	if !ok {
		fmt.Fprintf(os.Stderr, "rule %q has no configurable commands\n\n", ruleName)
		configUsage(allRules)
		os.Exit(1)
	}

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s config %s <cmd> [args]\navailable commands:\n", binName, ruleName)
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

func configUsage(rules []rule.Rule) {
	fmt.Fprintf(os.Stderr, "usage:\n")
	fmt.Fprintf(os.Stderr, "  %s config                     print current configuration\n", binName)
	fmt.Fprintf(os.Stderr, "  %s config <rule> <cmd> [args] configure a rule\n\n", binName)

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
