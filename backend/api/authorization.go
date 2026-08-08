package api

import (
	"net/http"
	"strings"

	"github.com/certvault/certvault/api/auth"
)

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.browserAuthenticator != nil {
			if identity, ok := a.browserAuthenticator.AuthenticateSession(r); ok {
				next.ServeHTTP(w, auth.WithIdentity(r, identity))
				return
			}
		}

		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != "" {
			principal, err := a.repos.APIKeys.Authenticate(r.Context(), token, a.clientIPs.ClientIP(r))
			if err == nil {
				identity := auth.Identity{Name: principal.Name, Principal: principal}
				next.ServeHTTP(w, auth.WithIdentity(r, identity))

				return
			}
		}

		problem(w, http.StatusUnauthorized, "unauthorized", "Authentication is required")
	})
}

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
