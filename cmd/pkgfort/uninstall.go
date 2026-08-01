package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func runUninstall(_ context.Context, _ []string) {
	appDir := filepath.Join(os.Getenv("HOME"), "."+binName)

	for _, name := range []string{binName, "init.sh"} {
		path := filepath.Join(appDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fatalf("uninstall: remove %s: %v", path, err)
		} else if err == nil {
			fmt.Println("removed", path)
		}
	}

	fmt.Println()
	fmt.Printf("Remove the 'source ~/.%s/init.sh' line from your shell rc to complete uninstall.\n", binName)
	fmt.Println("config.json and audit.log were left in place.")
}
