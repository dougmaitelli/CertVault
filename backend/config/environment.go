package config

// Application environment variable names.
const (
	EnvConfigFile              = "CERTVAULT_CONFIG"
	EnvDataDir                 = "CERTVAULT_DATA_DIR"
	EnvListen                  = "CERTVAULT_LISTEN"
	EnvPublicURL               = "CERTVAULT_PUBLIC_URL"
	EnvLogLevel                = "CERTVAULT_LOG_LEVEL"
	EnvACMEEmail               = "CERTVAULT_ACME_EMAIL"
	EnvACMEDirectoryURL        = "CERTVAULT_ACME_DIRECTORY_URL"
	EnvMasterKeyFile           = "CERTVAULT_MASTER_KEY_FILE"
	EnvBootstrapAdminTokenFile = "CERTVAULT_BOOTSTRAP_ADMIN_TOKEN_FILE"
	EnvUIDir                   = "CERTVAULT_UI_DIR"
	EnvEventJSON               = "CERTVAULT_EVENT_JSON"
	EnvPath                    = "PATH"
)

// EnvFileSuffix marks a provider environment variable whose value is read
// from a mounted file.
const EnvFileSuffix = "_FILE"
