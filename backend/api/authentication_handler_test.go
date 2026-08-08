package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestAuthenticationMethods(t *testing.T) {
	handler := API{cfg: &config.Config{Auth: config.Auth{
		BootstrapTokenFile: "/run/secrets/admin",
		OIDC:               &config.OIDC{},
	}}}
	response := httptest.NewRecorder()
	handler.authenticationMethods(response, nil)

	var methods map[string]bool
	if err := json.Unmarshal(response.Body.Bytes(), &methods); err != nil {
		t.Fatal(err)
	}

	if !methods["oidc"] || !methods["bootstrap"] {
		t.Fatalf("authentication methods = %#v", methods)
	}
}
