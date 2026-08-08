package service

import (
	"os"
	"strings"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestMuxProviderRequiresInheritedCredentialEnvironmentVariable(t *testing.T) {
	const tokenVariable = "CERTVAULT_TEST_DNS_TOKEN"

	previous, existed := os.LookupEnv(tokenVariable)
	if err := os.Unsetenv(tokenVariable); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(tokenVariable, previous)
		} else {
			_ = os.Unsetenv(tokenVariable)
		}
	})

	configuration := &config.Config{
		DNSCredentials: map[string]config.DNSCredential{
			"test": {
				Provider:    "cloudflare",
				Environment: map[string]string{tokenVariable: ""},
			},
		},
	}
	provider := &muxProvider{
		cfg:  configuration,
		cert: config.Certificate{Credential: "test"},
	}

	_, err := provider.forDomain("example.com")
	if err == nil || !strings.Contains(err.Error(), tokenVariable) {
		t.Fatalf("unexpected error: %v", err)
	}
}
