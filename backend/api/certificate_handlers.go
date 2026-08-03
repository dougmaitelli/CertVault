package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
)

const (
	certificateArtifact = "certificate.pem"
	chainArtifact       = "chain.pem"
	fullChainArtifact   = "fullchain.pem"
	privateKeyArtifact  = "private-key.pem"
)

var certificateArtifacts = []string{
	certificateArtifact,
	chainArtifact,
	fullChainArtifact,
	privateKeyArtifact,
}

func (a *API) listCertificates(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	if !a.allow(id, "certificates:read", "") {
		problem(w, http.StatusForbidden, "forbidden", "Missing scope")
		return
	}
	certificates, err := a.repos.Certificates.List(r.Context())
	if !id.admin {
		filtered := certificates[:0]
		for _, certificate := range certificates {
			if id.principal.Allows("certificates:read", certificate.Name) {
				filtered = append(filtered, certificate)
			}
		}
		certificates = filtered
	}
	respond(w, certificates, err)
}

func (a *API) getCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !a.allow(id, "certificates:read", name) {
		problem(w, http.StatusForbidden, "forbidden", "Missing scope")
		return
	}
	certificate, err := a.repos.Certificates.Get(r.Context(), name)
	respond(w, certificate, err)
}

func (a *API) listCertificateVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !a.allow(id, "certificates:read", name) {
		problem(w, http.StatusForbidden, "forbidden", "Missing scope")
		return
	}
	versions, err := a.repos.Certificates.Versions(r.Context(), name)
	respond(w, versions, err)
}

func (a *API) renewCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !a.allow(id, "renewals:trigger", name) {
		problem(w, http.StatusForbidden, "forbidden", "Missing scope")
		return
	}
	go func() { _ = a.manager.Issue(context.Background(), name, "manual") }()
	a.repos.Audits.Record(r.Context(), id.name, "renewal.trigger", name, "", remoteIP(r))
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (a *API) downloadCertificate(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requestIdentity(w, r)
		if !ok {
			return
		}
		name := r.PathValue("name")
		scope := "certificates:read"
		if file == privateKeyArtifact {
			scope = "private_keys:read"
		}
		if !a.allow(id, scope, name) {
			problem(w, http.StatusForbidden, "forbidden", "Missing scope")
			return
		}
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
		a.repos.Audits.Record(r.Context(), id.name, "certificate.download", name, file, remoteIP(r))
	}
}
