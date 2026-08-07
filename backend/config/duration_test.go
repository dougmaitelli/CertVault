package config

import (
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestDurationAcceptsWholeDays(t *testing.T) {
	var value struct {
		Retention Duration `yaml:"retention"`
	}
	if err := yaml.Unmarshal([]byte("retention: 90d\n"), &value); err != nil {
		t.Fatal(err)
	}
	if value.Retention.Duration != 90*24*time.Hour {
		t.Fatalf("retention = %v", value.Retention.Duration)
	}
}

func TestSessionDurationDefaultsToEightHours(t *testing.T) {
	if got := (&Config{}).SessionDuration(); got != 8*time.Hour {
		t.Fatalf("session duration = %v, want 8h", got)
	}
}

func TestSessionDurationEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvSessionDuration, "12h")
	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.SessionDuration(); got != 12*time.Hour {
		t.Fatalf("session duration = %v, want 12h", got)
	}
}

func TestInvalidSessionDurationEnvironmentOverride(t *testing.T) {
	t.Setenv(EnvSessionDuration, "tomorrow")
	if err := applyEnv(&Config{}); err == nil {
		t.Fatal("invalid session duration was accepted")
	}
}
