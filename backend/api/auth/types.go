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
	Admin     bool
	Name      string
	Principal repository.Principal
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
