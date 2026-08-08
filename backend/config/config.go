package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	certnetwork "github.com/certvault/certvault/network"
	"go.yaml.in/yaml/v3"
)

const OIDCCallbackPath = "/auth/callback"

const DefaultPath = "/config/config.yaml"

const oidcScopeOpenID = "openid"

func defaultOIDCScopes() []string {
	return []string{oidcScopeOpenID, "profile", "email", "groups"}
}

// Path returns the configured YAML path or the container default.
func Path() string {
	if path := os.Getenv(EnvConfigFile); path != "" {
		return path
	}

	return DefaultPath
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c := Config{ACME: ACME{AutomaticIssuance: true}}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	if err := applyEnv(&c); err != nil {
		return nil, err
	}

	if len(c.MasterKey) == 0 {
		path := filepath.Join(c.DataDir, "master.key")

		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return nil, fmt.Errorf("master key: %s does not exist; generate it with: openssl rand -base64 32", path)
			}

			return nil, fmt.Errorf("master key: %w", readErr)
		}

		if err := c.MasterKey.UnmarshalText(contents); err != nil {
			return nil, fmt.Errorf("master key: %w", err)
		}
	}

	return &c, c.Validate()
}

func (c *Config) Validate() error {
	if c.Auth.SessionDuration.Duration < 0 {
		return errors.New("auth.session_duration cannot be negative")
	}

	if c.Audit.Retention.Duration < 0 {
		return errors.New("audit.retention cannot be negative")
	}

	if c.ACME.Email == "" {
		return errors.New("acme.email is required")
	}

	if !c.ACME.AcceptTerms {
		return errors.New("acme.accept_terms must be true")
	}

	if _, err := url.ParseRequestURI(c.ACME.DirectoryURL); err != nil {
		return fmt.Errorf("invalid ACME directory URL: %w", err)
	}

	if _, err := certnetwork.NewClientIPResolver(c.Server.TrustedProxies); err != nil {
		return err
	}

	if c.Auth.OIDC != nil {
		c.Auth.OIDC.IssuerURL = strings.TrimRight(c.Auth.OIDC.IssuerURL, "/")
		if len(c.Auth.OIDC.Scopes) == 0 {
			c.Auth.OIDC.Scopes = defaultOIDCScopes()
		}

		if c.Auth.OIDC.IssuerURL == "" || c.Auth.OIDC.ClientID == "" {
			return errors.New("auth.oidc requires issuer_url and client_id")
		}

		if c.Auth.OIDC.ClientSecret == "" && c.Auth.OIDC.ClientSecretFile == "" {
			return errors.New("auth.oidc requires a client secret")
		}

		if c.Auth.OIDC.ClientSecret != "" && c.Auth.OIDC.ClientSecretFile != "" {
			return errors.New("auth.oidc client secret and client secret file cannot both be set")
		}

		if !slices.Contains(c.Auth.OIDC.Scopes, oidcScopeOpenID) {
			return errors.New("auth.oidc.scopes must include openid")
		}

		publicURL, err := url.Parse(c.Server.PublicURL)
		if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
			return errors.New("server.public_url must be an absolute URL when OIDC is enabled")
		}
	}

	credentials := map[string]bool{}

	for name, cred := range c.DNSCredentials {
		if name == "" || cred.Provider == "" {
			return errors.New("DNS credentials require a name and provider")
		}

		credentials[name] = true
	}

	zones := map[string]string{}

	for _, z := range c.Zones {
		z.Name = strings.TrimSuffix(strings.ToLower(z.Name), ".")
		if z.Name == "" || !credentials[z.Credential] {
			return fmt.Errorf("zone %q references unknown credential %q", z.Name, z.Credential)
		}

		zones[z.Name] = z.Credential
	}

	names := map[string]bool{}
	for _, cert := range c.Certificates {
		if cert.Name == "" || names[cert.Name] {
			return fmt.Errorf("certificate name %q is empty or duplicated", cert.Name)
		}

		names[cert.Name] = true
		if len(cert.Domains) == 0 {
			return fmt.Errorf("certificate %q has no domains", cert.Name)
		}

		if cert.KeyType != "" && !cert.KeyType.Valid() {
			return fmt.Errorf("certificate %q has unsupported key type %q", cert.Name, cert.KeyType)
		}

		if cert.RenewBefore.Duration == 0 {
			cert.RenewBefore.Duration = DefaultRenewBefore
		}

		if cert.Credential != "" && !credentials[cert.Credential] {
			return fmt.Errorf("certificate %q references unknown credential", cert.Name)
		}

		for _, domain := range cert.Domains {
			if findZone(strings.TrimPrefix(domain, "*."), zones) == "" && cert.Credential == "" {
				return fmt.Errorf("no configured zone covers %q", domain)
			}
		}
	}

	for _, hook := range c.Hooks {
		for _, certificate := range hook.Certificates {
			if !names[certificate] {
				return fmt.Errorf("hook %q references unknown certificate %q", hook.Name, certificate)
			}
		}
	}

	return nil
}

func (c *Config) SessionDuration() time.Duration {
	if c.Auth.SessionDuration.Duration == 0 {
		return DefaultSessionDuration
	}

	return c.Auth.SessionDuration.Duration
}

func (c *Config) UIEnabled() bool {
	return c.Server.UIEnabled == nil || *c.Server.UIEnabled
}

func (c *Config) OIDCRedirectURL() string {
	return strings.TrimRight(c.Server.PublicURL, "/") + OIDCCallbackPath
}

func (c *Config) Certificate(name string) (Certificate, bool) {
	for _, v := range c.Certificates {
		if v.Name == name {
			if v.KeyType == "" {
				v.KeyType = DefaultKeyType
			}

			if v.RenewBefore.Duration == 0 {
				v.RenewBefore.Duration = DefaultRenewBefore
			}

			return v, true
		}
	}

	return Certificate{}, false
}

func (c *Config) ShouldAutomaticallyIssue(certificate Certificate) bool {
	if certificate.AutomaticIssuance != nil {
		return *certificate.AutomaticIssuance
	}

	return c.ACME.AutomaticIssuance
}

func (c *Config) HasAutomaticIssuance() bool {
	for _, certificate := range c.Certificates {
		enabled := certificate.Enabled == nil || *certificate.Enabled
		if enabled && c.ShouldAutomaticallyIssue(certificate) {
			return true
		}
	}

	return false
}

func (c *Config) CredentialForDomain(cert Certificate, domain string) (string, DNSCredential, bool) {
	name := cert.Credential
	if name == "" {
		zones := map[string]string{}
		for _, z := range c.Zones {
			zones[strings.TrimSuffix(strings.ToLower(z.Name), ".")] = z.Credential
		}

		name = findZone(strings.TrimPrefix(strings.ToLower(domain), "*."), zones)
	}

	v, ok := c.DNSCredentials[name]

	return name, v, ok
}

func findZone(domain string, zones map[string]string) string {
	best, value := "", ""
	for zone, credential := range zones {
		if (domain == zone || strings.HasSuffix(domain, "."+zone)) && len(zone) > len(best) {
			best, value = zone, credential
		}
	}

	return value
}
