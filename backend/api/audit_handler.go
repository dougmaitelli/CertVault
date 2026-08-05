package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/certvault/certvault/database/repository"
)

const (
	defaultAuditPageSize = 25
	maxAuditPageSize     = 100
)

type auditPageResponse struct {
	Items      []repository.Audit `json:"items"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PerPage    int                `json:"per_page"`
	TotalPages int                `json:"total_pages"`
	Actors     []string           `json:"actors"`
	Actions    []string           `json:"actions"`
	Resources  []string           `json:"resources"`
}

func (a *API) listAudits(w http.ResponseWriter, r *http.Request) {
	page, err := auditPaginationValue(r, "page", 1, 1_000_000)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return
	}
	perPage, err := auditPaginationValue(r, "per_page", defaultAuditPageSize, maxAuditPageSize)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return
	}

	result, err := a.repos.Audits.Search(r.Context(), repository.AuditFilter{
		Query:     r.URL.Query().Get("q"),
		Actors:    r.URL.Query()["actor"],
		Actions:   r.URL.Query()["action"],
		Resources: r.URL.Query()["resource"],
		Page:      page,
		PerPage:   perPage,
	})
	if err != nil {
		respond(w, nil, err)
		return
	}
	actors, actions, resources, err := a.repos.Audits.FilterOptions(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}

	respond(w, auditPageResponse{
		Items:      result.Items,
		Total:      result.Total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: int((result.Total + int64(perPage) - 1) / int64(perPage)),
		Actors:     actors,
		Actions:    actions,
		Resources:  resources,
	}, nil)
}

func auditPaginationValue(r *http.Request, name string, fallback, max int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > max {
		return 0, errors.New(name + " must be between 1 and " + strconv.Itoa(max))
	}
	return value, nil
}
