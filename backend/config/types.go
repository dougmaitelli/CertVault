package config

type Server struct {
	Listen         string   `yaml:"listen"`
	PublicURL      string   `yaml:"public_url"`
	UIEnabled      *bool    `yaml:"ui_enabled"`
	LogLevel       LogLevel `yaml:"log_level"`
	TrustedProxies []string `yaml:"trusted_proxies"`
}

type ACME struct {
	Email             string   `yaml:"email"`
	DirectoryURL      string   `yaml:"directory_url"`
	DNSResolvers      []string `yaml:"dns_resolvers"`
	AcceptTerms       bool     `yaml:"accept_terms"`
	AutomaticIssuance bool     `yaml:"automatic_issuance"`
	Mock              bool     `yaml:"mock"`
}

type Auth struct {
	SessionDuration    Duration `yaml:"session_duration"`
	BootstrapToken     string   `yaml:"-"`
	BootstrapTokenFile string   `yaml:"bootstrap_token_file"`
	OIDC               *OIDC    `yaml:"oidc"`
}

type Audit struct {
	Retention Duration `yaml:"retention"`
}

type OIDC struct {
	IssuerURL        string   `yaml:"issuer_url"`
	ClientID         string   `yaml:"client_id"`
	ClientSecret     string   `yaml:"-"`
	ClientSecretFile string   `yaml:"client_secret_file"`
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
	Name              string   `yaml:"name"`
	Domains           []string `yaml:"domains"`
	KeyType           KeyType  `yaml:"key_type"`
	RenewBefore       Duration `yaml:"renew_before"`
	Credential        string   `yaml:"credential"`
	Enabled           *bool    `yaml:"enabled"`
	AutomaticIssuance *bool    `yaml:"automatic_issuance"`
}

type Hook struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`
	Events       []string `yaml:"events"`
	Certificates []string `yaml:"certificates"`
	URL          string   `yaml:"url"`
	SecretFile   string   `yaml:"secret_file"`
	Command      string   `yaml:"command"`
	Args         []string `yaml:"args"`
	Timeout      Duration `yaml:"timeout"`
}
