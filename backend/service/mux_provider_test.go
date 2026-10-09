package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/go-acme/lego/v5/challenge"
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

func TestCredentialEnvironmentRestoresActualTargets(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		t.Run(fmt.Sprintf("inherited=%t", inherited), func(t *testing.T) {
			const key = "CERTVAULT_TEST_DNS_TOKEN"
			t.Setenv(key, "original")

			if !inherited {
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}

			t.Setenv(key+"_FILE", "inherited-file-path")

			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte(" file-secret\n"), 0600); err != nil {
				t.Fatal(err)
			}

			for _, environment := range []map[string]string{
				{key + "_FILE": path},
				{key: "literal-secret"},
				{key: "literal-secret", key + "_FILE": path},
			} {
				_, err := withCredentialEnvironment(environment, func() (challenge.Provider, error) {
					want := "literal-secret"
					if _, exists := environment[key+"_FILE"]; exists {
						want = "file-secret"
					}

					if got := os.Getenv(key); got != want {
						t.Fatalf("constructor token = %q", got)
					}

					return nil, nil
				})
				if err != nil {
					t.Fatal(err)
				}

				value, exists := os.LookupEnv(key)
				if exists != inherited || (inherited && value != "original") {
					t.Fatal("token environment was not restored")
				}

				if os.Getenv(key+"_FILE") != "inherited-file-path" {
					t.Fatal("file variable changed")
				}
			}

			_, err := withCredentialEnvironment(map[string]string{key: ""}, func() (challenge.Provider, error) {
				if os.Getenv(key) != "original" {
					t.Fatal("subsequent provider inherited another credential")
				}

				return nil, nil
			})
			if inherited && err != nil {
				t.Fatal(err)
			}

			if !inherited && err == nil {
				t.Fatal("subsequent provider inherited leaked token")
			}
		})
	}
}

func TestCredentialEnvironmentRestoresOnFailures(t *testing.T) {
	constructorError := errors.New("constructor failed")

	for _, failure := range []string{"file", "inherited", "setenv", "constructor"} {
		t.Run(failure, func(t *testing.T) {
			const (
				first   = "CERTVAULT_TEST_A_TOKEN"
				missing = "CERTVAULT_TEST_Z_MISSING"
			)

			t.Setenv(first, "original")
			t.Setenv(missing, "")

			if err := os.Unsetenv(missing); err != nil {
				t.Fatal(err)
			}

			environment := map[string]string{first: "temporary-secret"}

			switch failure {
			case "file":
				environment["CERTVAULT_TEST_Z_TOKEN_FILE"] = filepath.Join(t.TempDir(), "missing")
			case "inherited":
				environment[missing] = ""
			case "setenv":
				environment["CERTVAULT_TEST_Z=INVALID"] = "value"
			}

			called := false

			_, err := withCredentialEnvironment(environment, func() (challenge.Provider, error) {
				called = true
				return nil, constructorError
			})
			if err == nil {
				t.Fatal("expected credential setup failure")
			}

			if called != (failure == "constructor") {
				t.Fatal("unexpected constructor invocation")
			}

			if failure == "constructor" && !errors.Is(err, constructorError) {
				t.Fatalf("lost constructor error: %v", err)
			}

			if os.Getenv(first) != "original" {
				t.Fatal("partial failure leaked previously configured token")
			}
		})
	}
}

func TestMuxProviderFileCredentialDoesNotLeak(t *testing.T) {
	const key = "CF_DNS_API_TOKEN"
	for _, name := range []string{key, "CLOUDFLARE_DNS_API_TOKEN", "CLOUDFLARE_EMAIL", "CLOUDFLARE_API_KEY", "CF_API_EMAIL", "CF_API_KEY"} {
		t.Setenv(name, "")
	}

	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("file-token\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{DNSCredentials: map[string]config.DNSCredential{
		"file":      {Provider: "cloudflare", Environment: map[string]string{key + "_FILE": path}},
		"inherited": {Provider: "cloudflare", Environment: map[string]string{key: ""}},
	}}
	mux := &muxProvider{cfg: cfg, cert: config.Certificate{Credential: "file"}, providers: map[string]challenge.Provider{}}

	provider, err := mux.forDomain("example.com")
	if err != nil {
		t.Fatal(err)
	}

	cached, err := mux.forDomain("example.com")
	if err != nil || cached != provider {
		t.Fatal("provider cache changed")
	}

	if os.Getenv(key) != "" {
		t.Fatal("Cloudflare file token leaked")
	}

	mux.cert.Credential = "inherited"
	if _, err = mux.forDomain("example.com"); err == nil {
		t.Fatal("Cloudflare inherited previous file token")
	}

	if mux.providers["inherited"] != nil {
		t.Fatal("failed provider cached")
	}
}
