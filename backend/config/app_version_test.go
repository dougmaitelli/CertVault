package config

import "testing"

func TestAppVersionDefaultsToDevelopment(t *testing.T) {
	t.Setenv(EnvAppVersion, "")
	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AppVersion != "dev" {
		t.Fatalf("app version = %q, want dev", cfg.AppVersion)
	}
}

func TestAppVersionEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvAppVersion, "v1.2.3")
	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AppVersion != "v1.2.3" {
		t.Fatalf("app version = %q, want v1.2.3", cfg.AppVersion)
	}
}
