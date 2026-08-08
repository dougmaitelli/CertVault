package api

import (
	"time"

	"github.com/certvault/certvault/api/auth"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	certnetwork "github.com/certvault/certvault/network"
	"github.com/certvault/certvault/service"
)

type API struct {
	cfg                  *config.Config
	database             *database.Database
	repos                *repository.Repositories
	manager              *service.Manager
	browserAuthenticator *auth.BrowserAuthenticator
	clientIPs            *certnetwork.ClientIPResolver
}

type createAPIKeyRequest struct {
	Name         string     `json:"name"`
	Scopes       []string   `json:"scopes"`
	Certificates []string   `json:"certificates"`
	ExpiresAt    *time.Time `json:"expires_at"`
}
