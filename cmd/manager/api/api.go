package api

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"
)

type API struct {
	router  *mux.Router
	version string
}

func NewAPI(version string) *API {
	return &API{
		version: version,
		router:  NewRouter(version),
	}
}

func (api *API) ListenAndServe() error {
	if api.router == nil {
		return errors.New("router is nil")
	}
	go http.ListenAndServe(":8080", api.router)
	return nil
}
