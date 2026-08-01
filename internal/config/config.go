package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type MinAgeDaysRule struct {
	Days int `json:"days"`
}

type VulnCheckRule struct{}

type Rules struct {
	MinAgeDays MinAgeDaysRule `json:"min_age_days"`
	VulnCheck  VulnCheckRule  `json:"vuln_check"`
}

type AllowlistEntry struct {
	Ecosystem string `json:"ecosystem"`
	Package   string `json:"package"`
}

const CurrentVersion = 1

type Config struct {
	Version      int              `json:"_v"`
	EnabledRules []string         `json:"enabled_rules"`
	Rules        Rules            `json:"rules"`
	FailOpen     bool             `json:"fail_open"`
	Allowlist    []AllowlistEntry `json:"allowlist"`
}

func Default() Config {
	return Config{
		Version:      CurrentVersion,
		EnabledRules: []string{"min_age_days", "vuln_check"},
		Rules: Rules{
			MinAgeDays: MinAgeDaysRule{Days: 7},
		},
		FailOpen:   false,
		Allowlist:  []AllowlistEntry{},
	}
}

func appDir() string {
	return filepath.Join(os.Getenv("HOME"), ".pkgfort")
}

func Load() (Config, error) {
	path := filepath.Join(appDir(), "config.json")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) Save() error {
	dir := appDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}
