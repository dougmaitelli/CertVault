package config

import (
	"strings"
	"testing"
)

func TestApplyEnvUsesOIDCClientSecretValue(t *testing.T) {
	t.Setenv(EnvOIDCClientSecret, "direct-secret")
	t.Setenv(EnvOIDCClientSecretFile, "")
	configuration := Config{Auth: Auth{OIDC: &OIDC{ClientSecretFile: "/configured/secret"}}}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}
	if configuration.Auth.OIDC.ClientSecret != "direct-secret" {
		t.Fatalf("OIDC client secret = %q", configuration.Auth.OIDC.ClientSecret)
	}
	if configuration.Auth.OIDC.ClientSecretFile != "" {
		t.Fatalf("OIDC client secret file = %q, want empty", configuration.Auth.OIDC.ClientSecretFile)
	}
}

func TestApplyEnvRejectsOIDCClientSecretValueAndFileTogether(t *testing.T) {
	t.Setenv(EnvOIDCClientSecret, "direct-secret")
	t.Setenv(EnvOIDCClientSecretFile, "/run/secrets/oidc_client_secret")

	err := applyEnv(&Config{Auth: Auth{OIDC: &OIDC{}}})
	if err == nil {
		t.Fatal("applyEnv accepted both OIDC client secret variables")
	}
	if !strings.Contains(err.Error(), EnvOIDCClientSecret) ||
		!strings.Contains(err.Error(), EnvOIDCClientSecretFile) {
		t.Fatalf("unexpected error: %v", err)
	}
}
