package api

import "net/http"

func (a *API) session(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	name := id.DisplayName
	if name == "" {
		name = id.Name
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"name":                  name,
		"email":                 id.Email,
		"picture":               id.Picture,
		"authentication_method": id.AuthenticationMethod,
		"admin":                 id.Admin,
	})
}
