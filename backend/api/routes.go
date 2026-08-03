package api

import (
	"context"
	"net/http"
	"strings"
)

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/ready", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.HandleFunc("POST /auth/bootstrap", a.bootstrapLogin)
	mux.HandleFunc("POST /auth/logout", a.logout)

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
	apiMux.HandleFunc("GET /api/v1/jobs", a.requireScope(scopeCertificatesRead, "", a.listJobs))
	apiMux.HandleFunc("GET /api/v1/api-keys", requireAdministrator(a.listAPIKeys))
	apiMux.HandleFunc("POST /api/v1/api-keys", requireAdministrator(a.createAPIKey))
	apiMux.HandleFunc("DELETE /api/v1/api-keys/{id}", requireAdministrator(a.revokeAPIKey))
	apiMux.HandleFunc("GET /api/v1/audit", requireAdministrator(a.listAudits))
	mux.Handle("/api/", a.auth(apiMux))

	mux.HandleFunc("/", a.frontend)
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

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("cv_session"); err == nil {
			if name, ok := a.verifySession(cookie.Value); ok {
				next.ServeHTTP(
					w,
					r.WithContext(context.WithValue(r.Context(), principalKey, identity{admin: true, name: name})),
				)
				return
			}
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != "" {
			principal, err := a.repos.APIKeys.Authenticate(r.Context(), token, remoteIP(r))
			if err == nil {
				next.ServeHTTP(
					w,
					r.WithContext(context.WithValue(
						r.Context(),
						principalKey,
						identity{name: principal.Name, principal: principal},
					)),
				)
				return
			}
		}
		problem(w, http.StatusUnauthorized, "unauthorized", "Authentication is required")
	})
}
