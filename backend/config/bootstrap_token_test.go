package config

import (
	"strings"
	"testing"
)

func TestApplyEnvUsesBootstrapTokenValue(t *testing.T) {
	t.Setenv(EnvBootstrapAdminToken, "direct-token")
	t.Setenv(EnvBootstrapAdminTokenFile, "")

	configuration := Config{Auth: Auth{BootstrapTokenFile: "/configured/token"}}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	if configuration.Auth.BootstrapToken != "direct-token" {
		t.Fatalf("bootstrap token = %q", configuration.Auth.BootstrapToken)
	}

	if configuration.Auth.BootstrapTokenFile != "" {
		t.Fatalf("bootstrap token file = %q, want empty", configuration.Auth.BootstrapTokenFile)
	}
}

func TestApplyEnvRejectsBootstrapTokenValueAndFileTogether(t *testing.T) {
	t.Setenv(EnvBootstrapAdminToken, "direct-token")
	t.Setenv(EnvBootstrapAdminTokenFile, "/run/secrets/admin_token")

	err := applyEnv(&Config{})
	if err == nil {
		t.Fatal("applyEnv accepted both bootstrap token variables")
	}

	if !strings.Contains(err.Error(), EnvBootstrapAdminToken) ||
		!strings.Contains(err.Error(), EnvBootstrapAdminTokenFile) {
		t.Fatalf("unexpected error: %v", err)
	}
}
