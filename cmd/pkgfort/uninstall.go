package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runUninstall(_ context.Context, _ []string) {
	home := os.Getenv("HOME")
	appDir := filepath.Join(home, "."+binName)
	initPath := filepath.Join(appDir, "init.sh")

	removeInitLineFromRCFiles(home, initPath)

	for _, name := range []string{binName, "init.sh", "config.json", "audit.log"} {
		path := filepath.Join(appDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fatalf("uninstall: remove %s: %v", path, err)
		} else if err == nil {
			fmt.Println("removed", path)
		}
	}
}

// removeInitLineFromRCFiles strips the "source <initPath>" line (and its
// preceding "# pkgfort" marker and blank line, as written by install.sh)
// from any shell rc file that has it.
func removeInitLineFromRCFiles(home, initPath string) {
	rcFiles := []string{filepath.Join(home, ".bashrc")}
	zdotdir := os.Getenv("ZDOTDIR")
	if zdotdir == "" {
		zdotdir = home
	}
	rcFiles = append(rcFiles, filepath.Join(zdotdir, ".zshrc"))

	initLine := "source " + initPath
	for _, rc := range rcFiles {
		if removeLineFromFile(rc, initLine) {
			fmt.Println("removed pkgfort init line from", rc)
		}
	}
}

func removeLineFromFile(path, targetLine string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == targetLine {
			found = true
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "# "+binName {
				out = out[:len(out)-1]
				if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
					out = out[:len(out)-1]
				}
			}
			continue
		}
		out = append(out, line)
	}

	if !found {
		return false
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644); err != nil {
		fatalf("uninstall: update %s: %v", path, err)
	}
	return true
}
