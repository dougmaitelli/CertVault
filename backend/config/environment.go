package config

// Application environment variable names.
const (
	EnvAppVersion              = "APP_VERSION"
	EnvConfigFile              = "CERTVAULT_CONFIG"
	EnvDataDir                 = "CERTVAULT_DATA_DIR"
	EnvListen                  = "CERTVAULT_LISTEN"
	EnvPublicURL               = "CERTVAULT_PUBLIC_URL"
	EnvLogLevel                = "CERTVAULT_LOG_LEVEL"
	EnvUIEnabled               = "CERTVAULT_UI_ENABLED"
	EnvACMEEmail               = "CERTVAULT_ACME_EMAIL"
	EnvACMEDirectoryURL        = "CERTVAULT_ACME_DIRECTORY_URL"
	EnvACMEDNSResolvers        = "CERTVAULT_ACME_DNS_RESOLVERS"
	EnvMasterKey               = "CERTVAULT_MASTER_KEY"
	EnvMasterKeyFile           = "CERTVAULT_MASTER_KEY_FILE"
	EnvSessionDuration         = "CERTVAULT_SESSION_DURATION"
	EnvBootstrapAdminToken     = "CERTVAULT_BOOTSTRAP_ADMIN_TOKEN"
	EnvBootstrapAdminTokenFile = "CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE"
	EnvOIDCIssuerURL           = "CERTVAULT_OIDC_ISSUER_URL"
	EnvOIDCClientID            = "CERTVAULT_OIDC_CLIENT_ID"
	EnvOIDCClientSecret        = "CERTVAULT_OIDC_CLIENT_SECRET"
	EnvOIDCClientSecretFile    = "CERTVAULT_OIDC_CLIENT_SECRET_FILE"
	EnvOIDCAllowedGroups       = "CERTVAULT_OIDC_ALLOWED_GROUPS"
	EnvUIDir                   = "CERTVAULT_UI_DIR"
	EnvEventJSON               = "CERTVAULT_EVENT_JSON"
	EnvPath                    = "PATH"
)

const (
	DefaultDNSResolverPrimary   = "1.1.1.1:53"
	DefaultDNSResolverSecondary = "1.0.0.1:53"
)

// EnvFileSuffix marks a provider environment variable whose value is read
// from a mounted file.
const EnvFileSuffix = "_FILE"
