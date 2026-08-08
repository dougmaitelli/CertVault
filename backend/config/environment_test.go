package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAppVersionDefaultsToDevelopment(t *testing.T) {
	t.Setenv("APP_VERSION", "")

	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.AppVersion != "dev" {
		t.Fatalf("app version = %q, want dev", cfg.AppVersion)
	}
}

func TestAppVersionEnvironmentOverride(t *testing.T) {
	t.Setenv("APP_VERSION", "v1.2.3")

	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.AppVersion != "v1.2.3" {
		t.Fatalf("app version = %q, want v1.2.3", cfg.AppVersion)
	}
}

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

func TestApplyEnvUsesPublicDNSResolversByDefault(t *testing.T) {
	t.Setenv("CERTVAULT_ACME_DNS_RESOLVERS", "")

	configuration := Config{}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	expected := []string{"1.1.1.1:53", "1.0.0.1:53"}
	assertDNSResolvers(t, configuration.ACME.DNSResolvers, expected)
}

func TestApplyEnvOverridesDNSResolvers(t *testing.T) {
	t.Setenv("CERTVAULT_ACME_DNS_RESOLVERS", "9.9.9.9:53, 8.8.8.8")

	configuration := Config{ACME: ACME{DNSResolvers: []string{"192.168.1.1:53"}}}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	expected := []string{"9.9.9.9:53", "8.8.8.8"}
	assertDNSResolvers(t, configuration.ACME.DNSResolvers, expected)
}

func assertDNSResolvers(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("DNS resolvers = %#v, want %#v", actual, expected)
	}

	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("DNS resolvers = %#v, want %#v", actual, expected)
		}
	}
}

func TestApplyEnvUsesOIDCClientSecretValue(t *testing.T) {
	t.Setenv("CERTVAULT_OIDC_CLIENT_SECRET", "direct-secret")
	t.Setenv("CERTVAULT_OIDC_CLIENT_SECRET_FILE", "")

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
	t.Setenv("CERTVAULT_OIDC_CLIENT_SECRET", "direct-secret")
	t.Setenv("CERTVAULT_OIDC_CLIENT_SECRET_FILE", "/run/secrets/oidc_client_secret")

	err := applyEnv(&Config{Auth: Auth{OIDC: &OIDC{}}})
	if err == nil {
		t.Fatal("applyEnv accepted both OIDC client secret variables")
	}

	if !strings.Contains(err.Error(), "CERTVAULT_OIDC_CLIENT_SECRET") ||
		!strings.Contains(err.Error(), "CERTVAULT_OIDC_CLIENT_SECRET_FILE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyEnvConfiguresOIDCFromEnvironment(t *testing.T) {
	t.Setenv("CERTVAULT_OIDC_ISSUER_URL", "https://auth.example.com")
	t.Setenv("CERTVAULT_OIDC_CLIENT_ID", "certvault")
	t.Setenv("CERTVAULT_OIDC_CLIENT_SECRET", "direct-secret")
	t.Setenv("CERTVAULT_OIDC_CLIENT_SECRET_FILE", "")
	t.Setenv("CERTVAULT_OIDC_SCOPES", "openid, profile, custom-scope")
	t.Setenv("CERTVAULT_OIDC_ALLOWED_GROUPS", "admins, certificate-operators")

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

	expectedScopes := []string{"openid", "profile", "custom-scope"}
	if !slices.Equal(configuration.Auth.OIDC.Scopes, expectedScopes) {
		t.Fatalf("OIDC scopes = %#v, want %#v", configuration.Auth.OIDC.Scopes, expectedScopes)
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

	if !slices.Equal(configuration.Auth.OIDC.Scopes, defaultOIDCScopes()) {
		t.Fatalf("OIDC scopes = %#v, want %#v", configuration.Auth.OIDC.Scopes, defaultOIDCScopes())
	}
}

func TestValidateRequiresOpenIDScope(t *testing.T) {
	configuration := Config{
		Server: Server{PublicURL: "https://certvault.example.com"},
		ACME: ACME{
			Email:        "admin@example.com",
			DirectoryURL: "https://acme.example.com/directory",
			AcceptTerms:  true,
		},
		Auth: Auth{OIDC: &OIDC{
			IssuerURL:    "https://id.example.com",
			ClientID:     "certvault",
			ClientSecret: "secret",
			Scopes:       []string{"profile", "email"},
		}},
	}

	err := configuration.Validate()
	if err == nil || !strings.Contains(err.Error(), "must include openid") {
		t.Fatalf("Validate() error = %v, want missing openid scope", err)
	}
}

func TestUIEnabledDefaultsToTrue(t *testing.T) {
	if !(&Config{}).UIEnabled() {
		t.Fatal("UI is disabled by default")
	}
}

func TestUIEnabledEnvironmentOverride(t *testing.T) {
	t.Setenv("CERTVAULT_UI_ENABLED", "false")

	cfg := Config{}
	if err := applyEnv(&cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.UIEnabled() {
		t.Fatal("UI environment override was ignored")
	}
}

func TestInvalidUIEnabledEnvironmentOverride(t *testing.T) {
	t.Setenv("CERTVAULT_UI_ENABLED", "sometimes")

	if err := applyEnv(&Config{}); err == nil {
		t.Fatal("invalid UI enabled value was accepted")
	}
}
