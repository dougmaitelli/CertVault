package api

import "net/http"

func (a *API) authenticationMethods(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]bool{
		"oidc":      a.cfg.Auth.OIDC != nil,
		"bootstrap": a.cfg.Auth.BootstrapToken != "" || a.cfg.Auth.BootstrapTokenFile != "",
	})
}
