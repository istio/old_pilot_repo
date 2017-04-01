package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

type Route struct {
	Name        string
	Pattern     string
	Method      string
	HandlerFunc http.HandlerFunc
}

func NewRouter(version string) *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	vSubRouter := router.PathPrefix(fmt.Sprintf("/%s/", version)).Subrouter()
	for _, route := range routes {
		vSubRouter.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}

	return router
}

var routes = []Route{
	Route{
		Name:        "GetConfig",
		Pattern:     "/config/{kind}/{namespace}/{name}",
		Method:      "GET",
		HandlerFunc: GetConfig,
	},
	Route{
		Name:        "AddConfig",
		Pattern:     "/config/{kind}/{namespace}/{name}",
		Method:      "POST",
		HandlerFunc: AddConfig,
	},
	Route{
		Name:        "UpdateConfig",
		Pattern:     "/config/{kind}/{namespace}/{name}",
		Method:      "PUT",
		HandlerFunc: UpdateConfig,
	},
	Route{
		Name:        "DeleteConfig",
		Pattern:     "/config/{kind}/{namespace}/{name}",
		Method:      "DELETE",
		HandlerFunc: DeleteConfig,
	},
	Route{
		Name:        "ListConfigs",
		Pattern:     "/config/{kind}/{namespace}",
		Method:      "GET",
		HandlerFunc: ListConfigs,
	},
}
