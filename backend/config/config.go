package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	certnetwork "github.com/certvault/certvault/network"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	DataDir        string                   `yaml:"data_dir"`
	Server         Server                   `yaml:"server"`
	ACME           ACME                     `yaml:"acme"`
	Auth           Auth                     `yaml:"auth"`
	DNSCredentials map[string]DNSCredential `yaml:"dns_credentials"`
	Zones          []Zone                   `yaml:"zones"`
	Certificates   []Certificate            `yaml:"certificates"`
	Hooks          []Hook                   `yaml:"hooks"`
	MasterKey      []byte                   `yaml:"-"`
}

const OIDCCallbackPath = "/auth/callback"

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if err := applyEnv(&c); err != nil {
		return nil, err
	}
	c.MasterKey, err = loadMasterKey(c.DataDir)
	if err != nil {
		return nil, fmt.Errorf("master key: %w", err)
	}
	return &c, c.Validate()
}

func applyEnv(c *Config) error {
	if v := os.Getenv(EnvDataDir); v != "" {
		c.DataDir = v
	}
	if c.DataDir == "" {
		c.DataDir = "/data"
	}
	if v := os.Getenv(EnvListen); v != "" {
		c.Server.Listen = v
	}
	if c.Server.Listen == "" {
		c.Server.Listen = "0.0.0.0:8080"
	}
	if v := os.Getenv(EnvPublicURL); v != "" {
		c.Server.PublicURL = v
	}
	if v := os.Getenv(EnvLogLevel); v != "" {
		if err := c.Server.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return fmt.Errorf("%s: %w", EnvLogLevel, err)
		}
	}
	if v := os.Getenv(EnvACMEEmail); v != "" {
		c.ACME.Email = v
	}
	if v := os.Getenv(EnvACMEDirectoryURL); v != "" {
		c.ACME.DirectoryURL = v
	}
	if c.ACME.DirectoryURL == "" {
		c.ACME.DirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	bootstrapToken := os.Getenv(EnvBootstrapAdminToken)
	bootstrapTokenFile := os.Getenv(EnvBootstrapAdminTokenFile)
	if bootstrapToken != "" && bootstrapTokenFile != "" {
		return fmt.Errorf(
			"%s and %s cannot both be set",
			EnvBootstrapAdminToken,
			EnvBootstrapAdminTokenFile,
		)
	}
	if bootstrapToken != "" {
		c.Auth.BootstrapToken = bootstrapToken
		c.Auth.BootstrapTokenFile = ""
	}
	if bootstrapTokenFile != "" {
		c.Auth.BootstrapToken = ""
		c.Auth.BootstrapTokenFile = bootstrapTokenFile
	}
	oidcIssuerURL := os.Getenv(EnvOIDCIssuerURL)
	oidcClientID := os.Getenv(EnvOIDCClientID)
	oidcClientSecret := os.Getenv(EnvOIDCClientSecret)
	oidcClientSecretFile := os.Getenv(EnvOIDCClientSecretFile)
	oidcAllowedGroups := os.Getenv(EnvOIDCAllowedGroups)
	if oidcClientSecret != "" && oidcClientSecretFile != "" {
		return fmt.Errorf(
			"%s and %s cannot both be set",
			EnvOIDCClientSecret,
			EnvOIDCClientSecretFile,
		)
	}
	if c.Auth.OIDC == nil && (oidcIssuerURL != "" || oidcClientID != "" ||
		oidcClientSecret != "" || oidcClientSecretFile != "" ||
		oidcAllowedGroups != "") {
		c.Auth.OIDC = &OIDC{}
	}
	if c.Auth.OIDC == nil {
		return nil
	}
	if oidcIssuerURL != "" {
		c.Auth.OIDC.IssuerURL = oidcIssuerURL
	}
	if oidcClientID != "" {
		c.Auth.OIDC.ClientID = oidcClientID
	}
	if oidcClientSecret != "" {
		c.Auth.OIDC.ClientSecret = oidcClientSecret
		c.Auth.OIDC.ClientSecretFile = ""
	}
	if oidcClientSecretFile != "" {
		c.Auth.OIDC.ClientSecret = ""
		c.Auth.OIDC.ClientSecretFile = oidcClientSecretFile
	}
	if oidcAllowedGroups != "" {
		c.Auth.OIDC.AllowedGroups = splitCommaSeparated(oidcAllowedGroups)
	}
	return nil
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func (c *Config) Validate() error {
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
		if c.Auth.OIDC.IssuerURL == "" || c.Auth.OIDC.ClientID == "" {
			return errors.New("auth.oidc requires issuer_url and client_id")
		}
		if c.Auth.OIDC.ClientSecret == "" && c.Auth.OIDC.ClientSecretFile == "" {
			return errors.New("auth.oidc requires a client secret")
		}
		if c.Auth.OIDC.ClientSecret != "" && c.Auth.OIDC.ClientSecretFile != "" {
			return errors.New("auth.oidc client secret and client secret file cannot both be set")
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
	return nil
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

func loadMasterKeyFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		return decodeMasterKey(string(b))
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return nil, fmt.Errorf("%s does not exist; generate it with: openssl rand -base64 32", path)
}

func loadMasterKey(dataDir string) ([]byte, error) {
	value := os.Getenv(EnvMasterKey)
	path := os.Getenv(EnvMasterKeyFile)
	if value != "" && path != "" {
		return nil, fmt.Errorf("%s and %s cannot both be set", EnvMasterKey, EnvMasterKeyFile)
	}
	if value != "" {
		return decodeMasterKey(value)
	}
	if path == "" {
		path = filepath.Join(dataDir, "master.key")
	}
	return loadMasterKeyFile(path)
}

func decodeMasterKey(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != 32 {
		return nil, errors.New("master key must be base64-encoded 32 bytes")
	}
	return raw, nil
}
