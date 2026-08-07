package config

import "testing"

func TestUIEnabledDefaultsToTrue(t *testing.T) {
	if !(&Config{}).UIEnabled() {
		t.Fatal("UI is disabled by default")
	}
}

func TestUIEnabledEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvUIEnabled, "false")
	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.UIEnabled() {
		t.Fatal("UI environment override was ignored")
	}
}

func TestInvalidUIEnabledEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvUIEnabled, "sometimes")
	if err := applyEnv(&Config{}); err == nil {
		t.Fatal("invalid UI enabled value was accepted")
	}
}
