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

func TestApplyEnvConfiguresOIDCFromEnvironment(t *testing.T) {
	t.Setenv(EnvOIDCIssuerURL, "https://auth.example.com")
	t.Setenv(EnvOIDCClientID, "certvault")
	t.Setenv(EnvOIDCClientSecret, "direct-secret")
	t.Setenv(EnvOIDCClientSecretFile, "")
	t.Setenv(EnvOIDCAllowedGroups, "admins, certificate-operators")

	configuration := Config{}
	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}
	if configuration.Auth.OIDC == nil {
		t.Fatal("OIDC was not configured")
	}
	if configuration.Auth.OIDC.IssuerURL != "https://auth.example.com" ||
		configuration.Auth.OIDC.ClientID != "certvault" ||
		configuration.Auth.OIDC.ClientSecret != "direct-secret" {
		t.Fatalf("unexpected OIDC configuration: %#v", configuration.Auth.OIDC)
	}
	expectedGroups := []string{"admins", "certificate-operators"}
	if len(configuration.Auth.OIDC.AllowedGroups) != len(expectedGroups) {
		t.Fatalf("allowed groups = %#v", configuration.Auth.OIDC.AllowedGroups)
	}
	for index, expected := range expectedGroups {
		if configuration.Auth.OIDC.AllowedGroups[index] != expected {
			t.Fatalf("allowed groups = %#v", configuration.Auth.OIDC.AllowedGroups)
		}
	}
}

func TestOIDCRedirectURLUsesPublicURL(t *testing.T) {
	configuration := Config{Server: Server{PublicURL: "https://certvault.example.com/"}}
	if actual := configuration.OIDCRedirectURL(); actual != "https://certvault.example.com/auth/callback" {
		t.Fatalf("OIDCRedirectURL() = %q", actual)
	}
}

func TestValidateRemovesTrailingSlashesFromOIDCIssuer(t *testing.T) {
	configuration := Config{
		Server: Server{PublicURL: "https://certvault.example.com"},
		ACME: ACME{
			Email:        "admin@example.com",
			DirectoryURL: "https://acme.example.com/directory",
			AcceptTerms:  true,
		},
		Auth: Auth{OIDC: &OIDC{
			IssuerURL:    "https://id.example.com/realms/homelab///",
			ClientID:     "certvault",
			ClientSecret: "secret",
		}},
	}

	if err := configuration.Validate(); err != nil {
		t.Fatal(err)
	}
	if configuration.Auth.OIDC.IssuerURL != "https://id.example.com/realms/homelab" {
		t.Fatalf("OIDC issuer URL = %q", configuration.Auth.OIDC.IssuerURL)
	}
}
