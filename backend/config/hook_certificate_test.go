package config

import (
	"strings"
	"testing"
)

func TestHookRejectsUnknownCertificate(t *testing.T) {
	cfg := Config{
		ACME:           ACME{Email: "admin@example.com", DirectoryURL: "https://acme.example/directory", AcceptTerms: true},
		DNSCredentials: map[string]DNSCredential{"dns": {Provider: "mock"}},
		Certificates:   []Certificate{{Name: "home", Domains: []string{"home.example"}, Credential: "dns"}},
		Hooks:          []Hook{{Name: "deploy", Certificates: []string{"unknown"}}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `hook "deploy" references unknown certificate "unknown"`) {
		t.Fatalf("unknown hook certificate error = %v", err)
	}
}
