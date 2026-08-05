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
