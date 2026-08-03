package auth

import (
	"sync"
	"time"

	"github.com/certvault/certvault/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Authenticator struct {
	config    Config
	repos     *store.Repositories
	bootstrap string
	oidc      *oidc.Provider
	oauth     *oauth2.Config
	states    sync.Map
}

type Config struct {
	PublicURL          string
	MasterKey          []byte
	BootstrapTokenFile string
	OIDC               *OIDCConfig
}

type OIDCConfig struct {
	IssuerURL        string
	ClientID         string
	ClientSecretFile string
	RedirectURL      string
	AllowedGroups    []string
}

type Identity struct {
	Admin     bool
	Name      string
	Principal store.Principal
}

type oidcState struct {
	nonce    string
	verifier string
	at       time.Time
}

type bootstrapLoginRequest struct {
	Token string `json:"token"`
}

type oidcClaims struct {
	Email  string   `json:"email"`
	Sub    string   `json:"sub"`
	Groups []string `json:"groups"`
}

type contextKey string
