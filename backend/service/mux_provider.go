package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/certvault/certvault/config"
	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/providers/dns"
)

type muxProvider struct {
	cfg       *config.Config
	cert      config.Certificate
	providers map[string]challenge.Provider
}

func (m *muxProvider) forDomain(domain string) (challenge.Provider, error) {
	name, credential, ok := m.cfg.CredentialForDomain(m.cert, domain)
	if !ok {
		return nil, fmt.Errorf("no DNS credential for %s", domain)
	}

	if provider := m.providers[name]; provider != nil {
		return provider, nil
	}

	provider, err := withCredentialEnvironment(credential.Environment, func() (challenge.Provider, error) {
		return dns.NewDNSChallengeProviderByName(credential.Provider)
	})
	if err != nil {
		return nil, err
	}

	m.providers[name] = provider

	return provider, nil
}

// Provider constructors read process-wide environment, including inherited values.
// Serialize setup and restoration across all managers.
var credentialEnvironmentMu sync.Mutex

func withCredentialEnvironment(environment map[string]string, create func() (challenge.Provider, error)) (provider challenge.Provider, result error) {
	credentialEnvironmentMu.Lock()
	defer credentialEnvironmentMu.Unlock()

	restore := map[string]*string{}
	defer func() {
		for key, value := range restore {
			var err error
			if value == nil {
				err = os.Unsetenv(key)
			} else {
				err = os.Setenv(key, *value)
			}

			if err != nil {
				result = errors.Join(result, fmt.Errorf("restore DNS credential environment variable %s: %w", key, err))
			}
		}
	}()

	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value := environment[key]
		if value == "" {
			if _, exists := os.LookupEnv(key); !exists {
				return nil, fmt.Errorf("DNS credential environment variable %s is not set", key)
			}

			continue
		}

		if strings.HasSuffix(key, config.EnvFileSuffix) {
			contents, err := os.ReadFile(value)
			if err != nil {
				return nil, fmt.Errorf("read DNS credential environment variable %s: %w", key, err)
			}

			key = strings.TrimSuffix(key, config.EnvFileSuffix)
			value = strings.TrimSpace(string(contents))
		}

		// Snapshot each actual target once, even if both KEY and KEY_FILE are set.
		if _, saved := restore[key]; !saved {
			old, exists := os.LookupEnv(key)
			if exists {
				restore[key] = &old
			} else {
				restore[key] = nil
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return nil, fmt.Errorf("set DNS credential environment variable %s: %w", key, err)
		}
	}

	return create()
}

func (m *muxProvider) Present(ctx context.Context, domain, token, keyAuth string) error {
	provider, err := m.forDomain(domain)
	if err != nil {
		return err
	}

	return provider.Present(ctx, domain, token, keyAuth)
}

func (m *muxProvider) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	provider, err := m.forDomain(domain)
	if err != nil {
		return err
	}

	return provider.CleanUp(ctx, domain, token, keyAuth)
}
