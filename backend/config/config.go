package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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
type Server struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"public_url"`
	LogLevel  string `yaml:"log_level"`
}
type ACME struct {
	Email        string `yaml:"email"`
	DirectoryURL string `yaml:"directory_url"`
	AcceptTerms  bool   `yaml:"accept_terms"`
}
type Auth struct {
	BootstrapTokenFile string `yaml:"bootstrap_token_file"`
	OIDC               *OIDC  `yaml:"oidc"`
}
type OIDC struct {
	IssuerURL        string   `yaml:"issuer_url"`
	ClientID         string   `yaml:"client_id"`
	ClientSecretFile string   `yaml:"client_secret_file"`
	RedirectURL      string   `yaml:"redirect_url"`
	AllowedGroups    []string `yaml:"allowed_groups"`
}
type DNSCredential struct {
	Provider    string            `yaml:"provider"`
	Environment map[string]string `yaml:"environment"`
}
type Zone struct {
	Name       string `yaml:"name"`
	Credential string `yaml:"credential"`
}
type Certificate struct {
	Name        string   `yaml:"name"`
	Domains     []string `yaml:"domains"`
	KeyType     string   `yaml:"key_type"`
	RenewBefore Duration `yaml:"renew_before"`
	Credential  string   `yaml:"credential"`
	Enabled     *bool    `yaml:"enabled"`
}
type Hook struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`
	Events     []string `yaml:"events"`
	URL        string   `yaml:"url"`
	SecretFile string   `yaml:"secret_file"`
	Command    string   `yaml:"command"`
	Args       []string `yaml:"args"`
	Timeout    Duration `yaml:"timeout"`
}
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	d.Duration = v
	return err
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	applyEnv(&c)
	keyFile := os.Getenv("CERTVAULT_MASTER_KEY_FILE")
	if keyFile == "" {
		keyFile = filepath.Join(c.DataDir, "master.key")
	}
	c.MasterKey, err = loadOrCreateKey(keyFile)
	if err != nil {
		return nil, fmt.Errorf("master key: %w", err)
	}
	return &c, c.Validate()
}

func applyEnv(c *Config) {
	if v := os.Getenv("CERTVAULT_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if c.DataDir == "" {
		c.DataDir = "/data"
	}
	if v := os.Getenv("CERTVAULT_LISTEN"); v != "" {
		c.Server.Listen = v
	}
	if c.Server.Listen == "" {
		c.Server.Listen = "0.0.0.0:8080"
	}
	if v := os.Getenv("CERTVAULT_PUBLIC_URL"); v != "" {
		c.Server.PublicURL = v
	}
	if v := os.Getenv("CERTVAULT_LOG_LEVEL"); v != "" {
		c.Server.LogLevel = v
	}
	if v := os.Getenv("CERTVAULT_ACME_EMAIL"); v != "" {
		c.ACME.Email = v
	}
	if v := os.Getenv("CERTVAULT_ACME_DIRECTORY_URL"); v != "" {
		c.ACME.DirectoryURL = v
	}
	if c.ACME.DirectoryURL == "" {
		c.ACME.DirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	if v := os.Getenv("CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE"); v != "" {
		c.Auth.BootstrapTokenFile = v
	}
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
		if cert.RenewBefore.Duration == 0 {
			cert.RenewBefore.Duration = 30 * 24 * time.Hour
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

func (c *Config) Certificate(name string) (Certificate, bool) {
	for _, v := range c.Certificates {
		if v.Name == name {
			if v.KeyType == "" {
				v.KeyType = "ec256"
			}
			if v.RenewBefore.Duration == 0 {
				v.RenewBefore.Duration = 30 * 24 * time.Hour
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

func loadOrCreateKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if e != nil || len(raw) != 32 {
			return nil, errors.New("master key must be base64-encoded 32 bytes")
		}
		return raw, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return nil, fmt.Errorf("%s does not exist; generate it with: openssl rand -base64 32", path)
}
