package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

const binName = "pkgfort"

// version is overridden at release-build time via -ldflags "-X main.version=...".
var version = "1.0.1"

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println(binName + " " + version)
	case "config":
		runConfig()
	case "proxy":
		runProxy(ctx, os.Args[2:])
	case "exec":
		runExec(ctx, os.Args[2:])
	case "install":
		runInstall(ctx, os.Args[2:])
	case "uninstall":
		runUninstall(ctx, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, binName+" "+version)
	fmt.Fprintln(os.Stderr, "usage: "+binName+" <command> [args]")
	fmt.Fprintln(os.Stderr, "  proxy      start the validation proxy (--ecosystem <npm|go>)")
	fmt.Fprintln(os.Stderr, "  exec       run a tool command through the proxy")
	fmt.Fprintln(os.Stderr, "  install    install shell wrappers and default config")
	fmt.Fprintln(os.Stderr, "  uninstall  remove shell wrappers and binary")
	fmt.Fprintln(os.Stderr, "  config     print current configuration")
	fmt.Fprintln(os.Stderr, "  version    print version")
}
