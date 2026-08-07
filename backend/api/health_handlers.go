package api

import (
	"net/http"
	"os"
)

const readinessFilePattern = ".certvault-readiness-*"

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": a.cfg.AppVersion,
	})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{
		"database": "ok",
		"storage":  "ok",
	}
	ready := true

	if err := a.database.Ping(r.Context()); err != nil {
		checks["database"] = "unavailable"
		ready = false
	}
	if err := checkWritableDirectory(a.cfg.DataDir); err != nil {
		checks["storage"] = "unavailable"
		ready = false
	}

	if !ready {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": checks,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"status": "ready",
		"checks": checks,
	})
}

func checkWritableDirectory(path string) error {
	file, err := os.CreateTemp(path, readinessFilePattern)
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
