package api

import (
	"encoding/json"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/certvault/certvault/api/auth"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
)

const contentSecurityPolicy = "default-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'"

func New(c *config.Config, repos *repository.Repositories, manager *service.Manager) (http.Handler, error) {
	authenticator, err := auth.New(c, repos)
	if err != nil {
		return nil, err
	}
	a := &API{cfg: c, repos: repos, manager: manager, authenticator: authenticator}
	return a.routes(), nil
}

func (a *API) frontend(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	if path == "." || path == "" {
		path = "index.html"
	}
	root := os.Getenv(config.EnvUIDir)
	if root == "" {
		root = "/app/ui"
	}
	full := filepath.Join(root, path)
	if _, e := os.Stat(full); e != nil {
		full = filepath.Join(root, "index.html")
	}
	if t := mime.TypeByExtension(filepath.Ext(full)); t != "" {
		w.Header().Set("Content-Type", t)
	}
	http.ServeFile(w, r, full)
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	jsonResponse(w, status, map[string]any{"status": status, "code": code, "detail": detail})
}

func respond(w http.ResponseWriter, v any, e error) {
	if e == nil {
		jsonResponse(w, 200, v)
		return
	}
	if repository.NotFound(e) {
		problem(w, 404, "not_found", "Resource not found")
		return
	}
	problem(w, 500, "internal_error", e.Error())
}

func remoteIP(r *http.Request) string {
	h, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return h
	}
	return r.RemoteAddr
}
