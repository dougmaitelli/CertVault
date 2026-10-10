package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/database/repository"
)

func (a *API) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.repos.APIKeys.List(r.Context())
	respond(w, keys, err)
}

func (a *API) createAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, ok := requestIdentity(w, r)
	if !ok {
		return
	}

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
		repository.AuditMetadata{Actor: audit.Actor(identity.Name), IP: a.remoteIP(r)},
	)
	if err != nil {
		problem(w, http.StatusInternalServerError, "database_error", err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]any{"api_key": key, "token": token})
}

func (a *API) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")

	keyID, err := strconv.ParseInt(id, 10, 64)
	if err == nil {
		_, err = a.repos.APIKeys.Revoke(r.Context(), keyID, repository.AuditMetadata{Actor: audit.Actor(identity.Name), IP: a.remoteIP(r)})
	}

	if err != nil {
		respond(w, nil, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	identity, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")

	keyID, err := strconv.ParseInt(id, 10, 64)
	if err == nil {
		_, err = a.repos.APIKeys.Delete(r.Context(), keyID, repository.AuditMetadata{Actor: audit.Actor(identity.Name), IP: a.remoteIP(r)})
	}

	if err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotRevoked) {
			problem(w, http.StatusConflict, "api_key_active", "API key must be revoked before it can be deleted")
			return
		}

		respond(w, nil, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
