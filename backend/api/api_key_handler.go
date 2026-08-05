package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/certvault/certvault/database/repository"
)

func (a *API) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.repos.APIKeys.List(r.Context())
	respond(w, keys, err)
}

func (a *API) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var input createAPIKeyRequest
	if err := decode(r, &input); err != nil {
		problem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.Name == "" || len(input.Scopes) == 0 || len(input.Certificates) == 0 {
		problem(w, http.StatusBadRequest, "invalid_request", "name, scopes, and certificates are required")
		return
	}
	key, token, err := a.repos.APIKeys.Create(
		r.Context(), input.Name, input.Scopes, input.Certificates, input.ExpiresAt,
	)
	if err != nil {
		problem(w, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	a.repos.Audits.Record(r.Context(), "admin", "api_key.create", key.Name, "", a.remoteIP(r))
	jsonResponse(w, http.StatusCreated, map[string]any{"api_key": key, "token": token})
}

func (a *API) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := ""
	keyID, err := strconv.ParseInt(id, 10, 64)
	if err == nil {
		name, err = a.repos.APIKeys.Revoke(r.Context(), keyID)
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	a.repos.Audits.Record(r.Context(), "admin", "api_key.revoke", name, "", a.remoteIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := ""
	keyID, err := strconv.ParseInt(id, 10, 64)
	if err == nil {
		name, err = a.repos.APIKeys.Delete(r.Context(), keyID)
	}
	if err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotRevoked) {
			problem(w, http.StatusConflict, "api_key_active", "API key must be revoked before it can be deleted")
			return
		}
		respond(w, nil, err)
		return
	}
	a.repos.Audits.Record(r.Context(), "admin", "api_key.delete", name, "", a.remoteIP(r))
	w.WriteHeader(http.StatusNoContent)
}
