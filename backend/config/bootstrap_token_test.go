package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyEnvUsesBootstrapTokenValue(t *testing.T) {
	t.Setenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN", "direct-token")
	t.Setenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE", "")

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

func TestApplyEnvUsesBootstrapTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin-token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN", "")
	t.Setenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE", path)

	configuration := Config{Auth: Auth{BootstrapToken: "configured-token"}}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	if configuration.Auth.BootstrapToken != "file-token" {
		t.Fatalf("bootstrap token = %q", configuration.Auth.BootstrapToken)
	}

	if configuration.Auth.BootstrapTokenFile != "" {
		t.Fatalf("bootstrap token file = %q, want empty", configuration.Auth.BootstrapTokenFile)
	}
}

func TestApplyEnvRejectsBootstrapTokenValueAndFileTogether(t *testing.T) {
	t.Setenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN", "direct-token")
	t.Setenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE", "/run/secrets/admin_token")

	err := applyEnv(&Config{})
	if err == nil {
		t.Fatal("applyEnv accepted both bootstrap token variables")
	}

	if !strings.Contains(err.Error(), "CERTVAULT_BOOTSTRAP_ADMIN_TOKEN") ||
		!strings.Contains(err.Error(), "CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE") {
		t.Fatalf("unexpected error: %v", err)
	}
}
