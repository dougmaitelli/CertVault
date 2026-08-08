package api

import (
	"net/http"

	"github.com/certvault/certvault/config"
)

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", a.health)
	mux.HandleFunc("GET /api/v1/ready", a.ready)

	if a.cfg.UIEnabled() {
		mux.HandleFunc("GET /auth/methods", a.authenticationMethods)
		mux.HandleFunc("GET /auth/login", a.browserAuthenticator.Login)
		mux.HandleFunc("GET "+config.OIDCCallbackPath, a.browserAuthenticator.Callback)
		mux.HandleFunc("POST /auth/bootstrap", a.browserAuthenticator.BootstrapLogin)
		mux.HandleFunc("POST /auth/logout", a.browserAuthenticator.Logout)
	}

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/v1/session", a.session)
	apiMux.HandleFunc(
		"GET /api/v1/certificates",
		a.requireScope(scopeCertificatesRead, "", a.listCertificates),
	)
	apiMux.HandleFunc(
		"GET /api/v1/certificates/{name}",
		a.requireScope(scopeCertificatesRead, "name", a.getCertificate),
	)
	apiMux.HandleFunc(
		"GET /api/v1/certificates/{name}/versions",
		a.requireScope(scopeCertificatesRead, "name", a.listCertificateVersions),
	)
	apiMux.HandleFunc(
		"POST /api/v1/certificates/{name}/renew",
		a.requireScope(scopeRenewalsTrigger, "name", a.renewCertificate),
	)

	for artifact, scope := range certificateArtifacts {
		apiMux.Handle(
			"GET /api/v1/certificates/{name}/"+artifact,
			a.requireScope(scope, "name", a.downloadCertificate(artifact)),
		)
	}

	apiMux.HandleFunc("GET /api/v1/jobs/history", requireAdministrator(a.jobHistory))
	apiMux.HandleFunc("GET /api/v1/acme-accounts", requireAdministrator(a.listACMEAccounts))
	apiMux.HandleFunc("DELETE /api/v1/acme-accounts/{id}", requireAdministrator(a.deleteACMEAccount))
	apiMux.HandleFunc("GET /api/v1/api-keys", requireAdministrator(a.listAPIKeys))
	apiMux.HandleFunc("POST /api/v1/api-keys", requireAdministrator(a.createAPIKey))
	apiMux.HandleFunc("POST /api/v1/api-keys/{id}/revoke", requireAdministrator(a.revokeAPIKey))
	apiMux.HandleFunc("DELETE /api/v1/api-keys/{id}", requireAdministrator(a.deleteAPIKey))
	apiMux.HandleFunc("GET /api/v1/audit", requireAdministrator(a.listAudits))
	mux.Handle("/api/", a.authenticate(apiMux))

	if a.cfg.UIEnabled() {
		mux.HandleFunc("/", a.frontend)
	}

	return a.security(mux)
}

func (a *API) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}
