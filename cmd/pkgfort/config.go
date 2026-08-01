package main

import (
	"encoding/json"
	"fmt"

	"github.com/yairp7/pkgfort/internal/config"
)

func runConfig() {
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
