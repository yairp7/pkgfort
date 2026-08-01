package config_test

import (
	"testing"

	"github.com/yairp7/pkgfort/internal/config"
)

func TestConfigSaveLoad_RoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := config.Default()
	cfg.Rules.MinAgeDays.Days = 42

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Rules.MinAgeDays.Days != 42 {
		t.Errorf("Days = %d, want 42", got.Rules.MinAgeDays.Days)
	}
	if got.Version != config.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, config.CurrentVersion)
	}
}

func TestConfigSave_Overwrites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := config.Default()
	cfg.Rules.MinAgeDays.Days = 10
	if err := cfg.Save(); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	cfg.Rules.MinAgeDays.Days = 20
	if err := cfg.Save(); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Rules.MinAgeDays.Days != 20 {
		t.Errorf("Days = %d, want 20", got.Rules.MinAgeDays.Days)
	}
}

func TestConfigLoad_MissingFileReturnsDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Default()
	if got.Rules.MinAgeDays.Days != want.Rules.MinAgeDays.Days {
		t.Errorf("Days = %d, want %d", got.Rules.MinAgeDays.Days, want.Rules.MinAgeDays.Days)
	}
}
