package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yairp7/pkgfort/internal/config"
	pkgenv "github.com/yairp7/pkgfort/internal/env"
)

func initScript(binPath string) string {
	return `# ` + binName + ` shell wrappers — source this file in your shell rc
npm() { "` + binPath + `" exec npm "$@"; }
go()  { "` + binPath + `" exec go  "$@"; }
`
}

func runInstall(_ context.Context, _ []string) {
	appDir := filepath.Join(os.Getenv("HOME"), "."+binName)

	if err := os.MkdirAll(appDir, 0700); err != nil {
		fatalf("install: create dir: %v", err)
	}

	binPath := filepath.Join(appDir, binName)
	if err := installBinary(appDir); err != nil {
		fatalf("install: copy binary: %v", err)
	}

	initPath := filepath.Join(appDir, "init.sh")
	if err := os.WriteFile(initPath, []byte(initScript(binPath)), 0644); err != nil {
		fatalf("install: write init.sh: %v", err)
	}

	cfgPath := filepath.Join(appDir, "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		def := config.Default()
		if err := def.Save(); err != nil {
			fatalf("install: write config: %v", err)
		}
		fmt.Println("wrote default config to", cfgPath)
	} else {
		fmt.Println("config already exists, skipping:", cfgPath)
	}

	fmt.Println("installed to", appDir)
	fmt.Println()
	fmt.Println("Add the following line to your ~/.zshrc or ~/.bashrc:")
	fmt.Printf("  source %s\n", initPath)
}

func installBinary(appDir string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}

	dst := filepath.Join(appDir, binName)

	src, err := os.Open(self)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return err
	}
	fmt.Println("installed binary to", dst)
	return nil
}

func log(format string, args ...any) {
	if !pkgenv.Verbose() {
		return
	}
	fmt.Fprintf(os.Stderr, "["+binName+"] "+format+"\n", args...)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "["+binName+"] "+format+"\n", args...)
	os.Exit(1)
}
