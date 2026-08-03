package api

import (
	"net/http"

	"github.com/certvault/certvault/auth"
)

const (
	scopeCertificatesRead = "certificates:read"
	scopePrivateKeysRead  = "private_keys:read"
	scopeRenewalsTrigger  = "renewals:trigger"
)

func (a *API) requireScope(scope, resourcePathValue string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestIdentity(w, r)
		if !ok {
			return
		}
		resource := ""
		if resourcePathValue != "" {
			resource = r.PathValue(resourcePathValue)
		}
		if !id.Admin && !id.Principal.Allows(scope, resource) {
			problem(w, http.StatusForbidden, "forbidden", "Missing scope")
			return
		}
		next(w, r)
	}
}

func requireAdministrator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestIdentity(w, r)
		if !ok {
			return
		}
		if !id.Admin {
			problem(w, http.StatusForbidden, "forbidden", "Administrator access required")
			return
		}
		next(w, r)
	}
}

func requestIdentity(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	id, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		problem(
			w,
			http.StatusInternalServerError,
			"missing_identity",
			"Authenticated identity is unavailable",
		)
	}
	return id, ok
}
