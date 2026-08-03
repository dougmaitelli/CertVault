package api

import (
	"log/slog"
	"sync"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type API struct {
	cfg       *config.Config
	repos     *store.Repositories
	manager   *service.Manager
	log       *slog.Logger
	bootstrap string
	oidc      *oidc.Provider
	oauth     *oauth2.Config
	states    sync.Map
}

type oidcState struct {
	Nonce    string
	Verifier string
	At       time.Time
}

type contextKey string

type identity struct {
	admin     bool
	name      string
	principal store.Principal
}

type createAPIKeyRequest struct {
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	Certificates []string   `json:"certificates"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

type bootstrapLoginRequest struct {
	Token string `json:"token"`
}

type oidcClaims struct {
	Email  string   `json:"email"`
	Sub    string   `json:"sub"`
	Groups []string `json:"groups"`
}
