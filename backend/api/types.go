package api

import (
	"time"

	"github.com/certvault/certvault/api/auth"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
)

type API struct {
	cfg           *config.Config
	repos         *repository.Repositories
	manager       *service.Manager
	authenticator *auth.Authenticator
}

type createAPIKeyRequest struct {
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	Certificates []string   `json:"certificates"`
	ExpiresAt    *time.Time `json:"expires_at"`
}
