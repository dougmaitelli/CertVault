package api

import (
	"time"

	"github.com/certvault/certvault/auth"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/store"
)

type API struct {
	cfg           *config.Config
	repos         *store.Repositories
	manager       *service.Manager
	authenticator *auth.Authenticator
}

type createAPIKeyRequest struct {
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	Certificates []string   `json:"certificates"`
	ExpiresAt    *time.Time `json:"expires_at"`
}
