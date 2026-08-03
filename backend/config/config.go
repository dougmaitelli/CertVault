package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const DefaultRenewBefore = 30 * 24 * time.Hour

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
	Listen    string   `yaml:"listen"`
	PublicURL string   `yaml:"public_url"`
	LogLevel  LogLevel `yaml:"log_level"`
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
	KeyType     KeyType  `yaml:"key_type"`
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

// LogLevel is a validated slog logging threshold.
type LogLevel slog.Level

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	d.Duration = v
	return err
}

func (l *LogLevel) UnmarshalYAML(n *yaml.Node) error {
	return l.UnmarshalText([]byte(n.Value))
}

// UnmarshalText parses a standard slog level name or offset.
func (l *LogLevel) UnmarshalText(text []byte) error {
	level, err := ParseLogLevel(string(text))
	if err != nil {
		return err
	}
	*l = LogLevel(level)
	return nil
}

// Level returns the configured threshold as a slog level.
func (l LogLevel) Level() slog.Level {
	return slog.Level(l)
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
	if err := applyEnv(&c); err != nil {
		return nil, err
	}
	keyFile := os.Getenv(EnvMasterKeyFile)
	if keyFile == "" {
		keyFile = filepath.Join(c.DataDir, "master.key")
	}
	c.MasterKey, err = loadOrCreateKey(keyFile)
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
	if v := os.Getenv(EnvBootstrapAdminTokenFile); v != "" {
		c.Auth.BootstrapTokenFile = v
	}
	return nil
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

// ParseLogLevel converts a configured slog level name or offset into a level.
func ParseLogLevel(value string) (slog.Level, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "WARNING" {
		normalized = "WARN"
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(normalized)); err != nil {
		return 0, fmt.Errorf("invalid server.log_level %q: %w", value, err)
	}
	return level, nil
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
