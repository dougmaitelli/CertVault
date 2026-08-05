package api

import (
	"net/http"

	"github.com/certvault/certvault/database/repository"
)

type jobHistoryResponse struct {
	Items        []repository.Job `json:"items"`
	Total        int64            `json:"total"`
	Page         int              `json:"page"`
	PerPage      int              `json:"per_page"`
	TotalPages   int              `json:"total_pages"`
	Certificates []string         `json:"certificates"`
	Operations   []string         `json:"operations"`
	Statuses     []string         `json:"statuses"`
}

func (a *API) jobHistory(w http.ResponseWriter, r *http.Request) {
	page, err := paginationValue(r, "page", 1, 1_000_000)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return
	}
	perPage, err := paginationValue(r, "per_page", defaultPageSize, maxPageSize)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return
	}
	result, err := a.repos.Jobs.Search(r.Context(), repository.JobFilter{
		Certificates: r.URL.Query()["certificate"], Kinds: r.URL.Query()["operation"],
		Statuses: r.URL.Query()["status"], Page: page, PerPage: perPage,
	})
	if err != nil {
		respond(w, nil, err)
		return
	}
	certificates, operations, statuses, err := a.repos.Jobs.FilterOptions(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, jobHistoryResponse{Items: result.Items, Total: result.Total, Page: page, PerPage: perPage,
		TotalPages: int((result.Total + int64(perPage) - 1) / int64(perPage)), Certificates: certificates,
		Operations: operations, Statuses: statuses}, nil)
}
