package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
)

const (
	certificateArtifact = "certificate.crt"
	chainArtifact       = "chain.crt"
	fullChainArtifact   = "fullchain.crt"
	privateKeyArtifact  = "private.key"
)

var certificateArtifacts = map[string]string{
	certificateArtifact: scopeCertificatesRead,
	chainArtifact:       scopeCertificatesRead,
	fullChainArtifact:   scopeCertificatesRead,
	privateKeyArtifact:  scopePrivateKeysRead,
}

func (a *API) listCertificates(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	certificates, err := a.repos.Certificates.List(r.Context())
	if !id.Admin {
		filtered := certificates[:0]
		for _, certificate := range certificates {
			if id.Principal.Allows(scopeCertificatesRead, certificate.Name) {
				filtered = append(filtered, certificate)
			}
		}

		certificates = filtered
	}

	respond(w, certificates, err)
}

func (a *API) getCertificate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	certificate, err := a.repos.Certificates.Get(r.Context(), name)
	respond(w, certificate, err)
}

func (a *API) listCertificateVersions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versions, err := a.repos.Certificates.Versions(r.Context(), name)
	respond(w, versions, err)
}

func (a *API) renewCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	name := r.PathValue("name")

	job, created, err := a.manager.Enqueue(r.Context(), name, service.IssueKindManual)
	if err != nil {
		if errors.Is(err, repository.ErrIssuanceQueueFull) {
			w.Header().Set("Retry-After", "60")
			problem(w, http.StatusTooManyRequests, "queue_full", "Issuance queue is full; retry later")
		} else {
			respond(w, nil, err)
		}

		return
	}

	if created {
		a.repos.Audits.Record(r.Context(), audit.Actor(id.Name), audit.ActionRenewalTrigger, name, fmt.Sprintf("job=%d", job.ID), a.remoteIP(r))
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/jobs/%d", job.ID))
	jsonResponse(w, http.StatusAccepted, map[string]any{"status": job.Status, "job_id": job.ID})
}

func (a *API) downloadCertificate(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestIdentity(w, r)
		if !ok {
			return
		}

		name := r.PathValue("name")

		version, err := a.repos.Certificates.CurrentVersion(r.Context(), name)
		if err != nil {
			respond(w, nil, err)
			return
		}

		contents, err := a.manager.ReadFile(version, file)
		if err != nil {
			problem(w, http.StatusInternalServerError, "storage_error", err.Error())
			return
		}

		sum := sha256.Sum256(contents)

		etag := fmt.Sprintf("\"%x\"", sum)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+"-"+file))
		_, _ = w.Write(contents)

		a.repos.Audits.Record(
			r.Context(), audit.Actor(id.Name), audit.ActionCertificateDownload,
			name, file, a.remoteIP(r),
		)
	}
}
