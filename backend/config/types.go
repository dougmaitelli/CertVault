package config

type Config struct {
	AppVersion     string                   `yaml:"-" env:"APP_VERSION" default:"dev"`
	DataDir        string                   `yaml:"data_dir" env:"CERTVAULT_DATA_DIR" default:"/data"`
	Server         Server                   `yaml:"server"`
	ACME           ACME                     `yaml:"acme"`
	Auth           Auth                     `yaml:"auth"`
	Audit          Audit                    `yaml:"audit"`
	Notifications  Notifications            `yaml:"notifications"`
	DNSCredentials map[string]DNSCredential `yaml:"dns_credentials"`
	Zones          []Zone                   `yaml:"zones"`
	Certificates   []Certificate            `yaml:"certificates"`
	Hooks          []Hook                   `yaml:"hooks"`
	MasterKey      MasterKey                `yaml:"-" env:"CERTVAULT_MASTER_KEY" file:"true"`
}

type Notifications struct {
	AppriseURL  string   `yaml:"apprise_url" env:"CERTVAULT_APPRISE_URL"`
	AppriseURLs []string `yaml:"apprise_urls" env:"CERTVAULT_APPRISE_URLS"`
	AppriseTags []string `yaml:"apprise_tags" env:"CERTVAULT_APPRISE_TAGS"`
}

type Server struct {
	Listen         string   `yaml:"listen" env:"CERTVAULT_LISTEN" default:"0.0.0.0:8080"`
	PublicURL      string   `yaml:"public_url" env:"CERTVAULT_PUBLIC_URL"`
	UIEnabled      *bool    `yaml:"ui_enabled" env:"CERTVAULT_UI_ENABLED"`
	LogLevel       LogLevel `yaml:"log_level" env:"CERTVAULT_LOG_LEVEL"`
	TrustedProxies []string `yaml:"trusted_proxies"`
}

type ACME struct {
	Email             string   `yaml:"email" env:"CERTVAULT_ACME_EMAIL"`
	DirectoryURL      string   `yaml:"directory_url" env:"CERTVAULT_ACME_DIRECTORY_URL" default:"https://acme-v02.api.letsencrypt.org/directory"`
	DNSResolvers      []string `yaml:"dns_resolvers" env:"CERTVAULT_ACME_DNS_RESOLVERS" default:"1.1.1.1:53,1.0.0.1:53"`
	AcceptTerms       bool     `yaml:"accept_terms"`
	AutomaticIssuance bool     `yaml:"automatic_issuance"`
	Mock              bool     `yaml:"mock"`
}

type Auth struct {
	SessionDuration    Duration `yaml:"session_duration" env:"CERTVAULT_SESSION_DURATION"`
	BootstrapToken     string   `yaml:"-" env:"CERTVAULT_BOOTSTRAP_ADMIN_TOKEN" file:"true"`
	BootstrapTokenFile string   `yaml:"bootstrap_token_file"`
	OIDC               *OIDC    `yaml:"oidc"`
}

type Audit struct {
	Retention Duration `yaml:"retention"`
}

type OIDC struct {
	IssuerURL        string   `yaml:"issuer_url" env:"CERTVAULT_OIDC_ISSUER_URL"`
	ClientID         string   `yaml:"client_id" env:"CERTVAULT_OIDC_CLIENT_ID"`
	ClientSecret     string   `yaml:"-" env:"CERTVAULT_OIDC_CLIENT_SECRET" file:"true"`
	ClientSecretFile string   `yaml:"client_secret_file"`
	Scopes           []string `yaml:"scopes" env:"CERTVAULT_OIDC_SCOPES"`
	AllowedGroups    []string `yaml:"allowed_groups" env:"CERTVAULT_OIDC_ALLOWED_GROUPS"`
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
