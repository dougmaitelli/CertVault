package auth

import (
	"sync"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database/repository"
	certnetwork "github.com/certvault/certvault/network"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Authenticator struct {
	config    *config.Config
	repos     *repository.Repositories
	bootstrap string
	oidc      *oidc.Provider
	oauth     *oauth2.Config
	clientIPs *certnetwork.ClientIPResolver
	states    sync.Map
}

type Identity struct {
	Admin                bool
	Name                 string
	DisplayName          string
	Email                string
	Picture              string
	AuthenticationMethod string
	Principal            repository.Principal
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
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Picture string   `json:"picture"`
	Sub     string   `json:"sub"`
	Groups  []string `json:"groups"`
}

type sessionPayload struct {
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name,omitempty"`
	Email                string `json:"email,omitempty"`
	Picture              string `json:"picture,omitempty"`
	AuthenticationMethod string `json:"authentication_method"`
	ExpiresAt            int64  `json:"expires_at"`
}

type contextKey string
