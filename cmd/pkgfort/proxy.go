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
)

func runProxy(ctx context.Context, args []string) {
	if len(args) == 0 || args[0] != "--ecosystem" {
		proxyUsage()
		os.Exit(1)
	}

	runProxyServer(ctx, args)
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

func proxyUsage() {
	fmt.Fprintf(os.Stderr, "usage: %s proxy --ecosystem <npm|go>\n", binName)
}
