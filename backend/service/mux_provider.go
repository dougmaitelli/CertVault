package service

import (
	"context"
	"fmt"
	"os"
	"strings"

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

	restore := map[string]*string{}
	for key, value := range credential.Environment {
		if value == "" {
			if _, exists := os.LookupEnv(key); !exists {
				return nil, fmt.Errorf("DNS credential environment variable %s is not set", key)
			}
			continue
		}
		old, exists := os.LookupEnv(key)
		if exists {
			copy := old
			restore[key] = &copy
		} else {
			restore[key] = nil
		}
		if strings.HasSuffix(key, config.EnvFileSuffix) {
			contents, err := os.ReadFile(value)
			if err != nil {
				return nil, err
			}
			if err := os.Setenv(
				strings.TrimSuffix(key, config.EnvFileSuffix),
				strings.TrimSpace(string(contents)),
			); err != nil {
				return nil, err
			}
		} else if err := os.Setenv(key, value); err != nil {
			return nil, err
		}
	}

	provider, providerErr := dns.NewDNSChallengeProviderByName(credential.Provider)
	for key, value := range restore {
		if value == nil {
			if err := os.Unsetenv(key); err != nil {
				return nil, err
			}
		} else if err := os.Setenv(key, *value); err != nil {
			return nil, err
		}
	}
	if providerErr != nil {
		return nil, providerErr
	}
	m.providers[name] = provider
	return provider, nil
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
