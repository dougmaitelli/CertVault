package api

import (
	"net/http"
	"strconv"
)

func (a *API) getIssuanceJob(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	jobID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || jobID <= 0 {
		problem(w, http.StatusBadRequest, "invalid_job", "Invalid job ID")
		return
	}

	job, err := a.repos.Jobs.Get(r.Context(), jobID)
	if err != nil {
		respond(w, nil, err)
		return
	}

	if !id.Admin && !id.Principal.Allows(scopeRenewalsTrigger, job.CertificateName) && !id.Principal.Allows(scopeCertificatesRead, job.CertificateName) {
		problem(w, http.StatusForbidden, "forbidden", "Missing certificate permission")
		return
	}

	respond(w, job, nil)
}
