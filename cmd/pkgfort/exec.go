package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/yairp7/pkgfort/internal/config"
	pkgenv "github.com/yairp7/pkgfort/internal/env"
	scmexec "github.com/yairp7/pkgfort/internal/exec"
)

func runExec(ctx context.Context, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: "+binName+" exec <npm|go> [args...]")
		os.Exit(1)
	}

	tool := args[0]
	toolArgs := args[1:]

	ecosystemName, known := scmexec.EcosystemForTool(tool)

	if pkgenv.SkipRequested() {
		log("PKGFORT_SKIP=1: bypassing proxy")
		passThrough(tool, toolArgs)
		return
	}
	if !known || !scmexec.NeedsProxy(tool, toolArgs) {
		passThrough(tool, toolArgs)
		return
	}

	animDone := make(chan struct{})
	go func() {
		printActivated()
		close(animDone)
	}()

	port, proxyCmd, err := startProxy(ctx, ecosystemName)
	if err != nil {
		<-animDone
		cfg, _ := config.Load()
		if cfg.FailOpen {
			log("proxy failed to start (%v): fail_open=true, passing through", err)
			passThrough(tool, toolArgs)
			return
		}
		fatalf("proxy failed to start: %v", err)
	}
	defer proxyCmd.Process.Kill()

	realBin, err := osexec.LookPath(tool)
	if err != nil {
		<-animDone
		fatalf("%s not found: %v", tool, err)
	}

	cmd := osexec.Command(realBin, toolArgs...)
	cmd.Env = scmexec.EnvWithProxy(os.Environ(), tool, port)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	<-animDone
	log("starting %s proxy", ecosystemName)
	log("proxy ready on :%d", port)

	exitCode := 0
	if runErr := cmd.Run(); runErr != nil {
		if exitErr, ok := runErr.(*osexec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	proxyCmd.Process.Kill()
	os.Exit(exitCode)
}

func printActivated() {
	colors := []string{"\033[31m", "\033[33m", "\033[32m", "\033[36m", "\033[34m", "\033[35m"}
	reset := "\033[0m"
	text := []rune("🛡️ pkgfort activated (v" + version + ") 🛡️")
	for i := 0; i < 12; i++ {
		var sb strings.Builder
		sb.WriteString("\r")
		for j, r := range text {
			sb.WriteString(colors[(i+j)%len(colors)])
			sb.WriteRune(r)
		}
		sb.WriteString(reset)
		fmt.Fprint(os.Stderr, sb.String())
		time.Sleep(60 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr)
}

func passThrough(tool string, args []string) {
	realBin, err := osexec.LookPath(tool)
	if err != nil {
		fatalf("%s not found: %v", tool, err)
	}
	cmd := osexec.Command(realBin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*osexec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func startProxy(ctx context.Context, ecosystemName string) (int, *osexec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, nil, fmt.Errorf("could not determine executable path: %w", err)
	}

	cmd := osexec.Command(self, "proxy", "--ecosystem", ecosystemName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, nil, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}

	type readResult struct {
		port int
		err  error
	}
	ch := make(chan readResult, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "READY port=") {
				portStr := strings.TrimPrefix(line, "READY port=")
				port, err := strconv.Atoi(portStr)
				ch <- readResult{port: port, err: err}
				return
			}
		}
		ch <- readResult{err: fmt.Errorf("proxy exited without READY signal")}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			cmd.Process.Kill()
			return 0, nil, r.err
		}
		return r.port, cmd, nil
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		return 0, nil, fmt.Errorf("proxy startup timed out after 5s")
	case <-ctx.Done():
		cmd.Process.Kill()
		return 0, nil, ctx.Err()
	}
}
