package api

import "net/http"

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
		if !id.admin && !id.principal.Allows(scope, resource) {
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
		if !id.admin {
			problem(w, http.StatusForbidden, "forbidden", "Administrator access required")
			return
		}
		next(w, r)
	}
}

func requestIdentity(w http.ResponseWriter, r *http.Request) (identity, bool) {
	id, ok := r.Context().Value(principalKey).(identity)
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
