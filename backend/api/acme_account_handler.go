package api

import (
	"errors"
	"net/http"

	"github.com/certvault/certvault/service"
)

func (a *API) listACMEAccounts(w http.ResponseWriter, _ *http.Request) {
	accounts, err := a.manager.ListAccounts()
	respond(w, accounts, err)
}

func (a *API) deleteACMEAccount(w http.ResponseWriter, r *http.Request) {
	account, err := a.manager.DeleteAccount(r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidACMEAccountID):
			problem(w, http.StatusBadRequest, "invalid_acme_account", err.Error())
		case errors.Is(err, service.ErrCurrentACMEAccount):
			problem(w, http.StatusConflict, "current_acme_account", err.Error())
		case errors.Is(err, service.ErrACMEAccountNotFound):
			problem(w, http.StatusNotFound, "acme_account_not_found", err.Error())
		default:
			problem(w, http.StatusInternalServerError, "acme_account_delete_failed", err.Error())
		}
		return
	}
	a.repos.Audits.Record(
		r.Context(),
		"admin",
		"acme_account.delete",
		account.DirectoryURL,
		"",
		a.remoteIP(r),
	)
	w.WriteHeader(http.StatusNoContent)
}
