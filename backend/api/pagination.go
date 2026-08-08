package api

import (
	"errors"
	"net/http"
	"strconv"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

func paginationValue(r *http.Request, name string, fallback, max int) (int, error) {
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
