package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
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
	go func() { _ = a.manager.Issue(context.Background(), name, "manual") }()
	a.repos.Audits.Record(r.Context(), id.Name, "renewal.trigger", name, "", a.remoteIP(r))
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "queued"})
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
		a.repos.Audits.Record(r.Context(), id.Name, "certificate.download", name, file, a.remoteIP(r))
	}
}
