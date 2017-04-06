package apiserver

import (
	"fmt"
	"net/http"
	"strconv"

	"istio.io/manager/model"

	restful "github.com/emicklei/go-restful"
	"github.com/golang/glog"
)

const (
	kind      = "kind"
	name      = "name"
	namespace = "namespace"
)

type APIServiceOptions struct {
	Version  string
	Port     int
	Registry *model.IstioRegistry
}

type API struct {
	server   *http.Server
	version  string
	registry *model.IstioRegistry
}

func NewAPI(o APIServiceOptions) *API {
	out := &API{
		version:  o.Version,
		registry: o.Registry,
	}
	container := restful.NewContainer()
	out.Register(container)
	out.server = &http.Server{Addr: ":" + strconv.Itoa(o.Port), Handler: container}
	return out
}

func (api *API) Register(container *restful.Container) {
	ws := &restful.WebService{}
	ws.Consumes(restful.MIME_JSON)
	ws.Produces(restful.MIME_JSON)
	ws.Path(fmt.Sprintf("/%s", api.version))

	ws.Route(ws.
		GET(fmt.Sprintf("/config/{%s}/{%s}/{%s}", kind, namespace, name)).
		To(api.GetConfig).
		Doc("Get a config").
		Writes(Config{}))

	ws.Route(ws.
		POST(fmt.Sprintf("/config/{%s}/{%s}/{%s}", kind, namespace, name)).
		To(api.AddConfig).
		Doc("Add a config").
		Reads(Config{}))

	ws.Route(ws.
		PUT(fmt.Sprintf("/config/{%s}/{%s}/{%s}", kind, namespace, name)).
		To(api.UpdateConfig).
		Doc("Update a config").
		Reads(Config{}))

	ws.Route(ws.
		DELETE(fmt.Sprintf("/config/{%s}/{%s}/{%s}", kind, namespace, name)).
		To(api.DeleteConfig).
		Doc("Delete a config"))

	ws.Route(ws.
		GET(fmt.Sprintf("/config/{%s}/{%s}", kind, namespace)).
		To(api.ListConfigs).
		Doc("List all configs for kind in a given namespace").
		Writes([]Config{}))

	ws.Route(ws.
		GET(fmt.Sprintf("/config/{%s}", kind)).
		To(api.ListConfigs).
		Doc("List all configs for kind in across all namespaces").
		Writes([]Config{}))

	container.Add(ws)
}

func (api *API) Run() {
	glog.Infof("Starting api at %v", api.server.Addr)
	if err := api.server.ListenAndServe(); err != nil {
		glog.Warning(err)
	}
}
