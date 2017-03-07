// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envoy

import (
	"fmt"
	"net/http"
	"strconv"

	restful "github.com/emicklei/go-restful"
	"github.com/golang/glog"

	"istio.io/manager/model"
)

// DiscoveryService publishes services, clusters, and routes for all proxies
type DiscoveryService struct {
	services model.ServiceDiscovery
	config   *model.IstioRegistry
	mesh     *MeshConfig
	server   *http.Server
}

type hosts struct {
	Hosts []*host `json:"hosts,omitempty"`
}

type host struct {
	Address string `json:"ip_address"`
	Port    int    `json:"port"`

	// Weight is an integer in the range [1, 100] or empty
	Weight int `json:"load_balancing_weight,omitempty"`
}

type clusters struct {
	Clusters []*Cluster `json:"clusters,omitempty"`
}

// Request parameters for discovery services
const (
	ServiceKey      = "service-key"
	ServiceCluster  = "service-cluster"
	ServiceNode     = "service-node"
	RouteConfigName = "route-config-name"
)

// NewDiscoveryService creates an Envoy discovery service on a given port
func NewDiscoveryService(services model.ServiceDiscovery, config *model.IstioRegistry, mesh *MeshConfig, port int) *DiscoveryService {
	out := &DiscoveryService{
		services: services,
		config:   config,
		mesh:     mesh,
	}
	container := restful.NewContainer()
	out.Register(container)
	out.server = &http.Server{Addr: ":" + strconv.Itoa(port), Handler: container}
	return out
}

// Register adds routes a web service container
func (ds *DiscoveryService) Register(container *restful.Container) {
	ws := &restful.WebService{}
	ws.Produces(restful.MIME_JSON)

	ws.Route(ws.
		GET(fmt.Sprintf("/v1/registration/{%s}", ServiceKey)).
		To(ds.ListEndpoints).
		Doc("SDS registration").
		Param(ws.PathParameter(ServiceKey, "tuple of service name and tag name").DataType("string")).
		Writes(hosts{}))

	ws.Route(ws.
		GET(fmt.Sprintf("/v1/clusters/{%s}/{%s}", ServiceCluster, ServiceNode)).
		To(ds.ListClusters).
		Doc("CDS registration").
		Param(ws.PathParameter(ServiceCluster, "client proxy service cluster").DataType("string")).
		Param(ws.PathParameter(ServiceNode, "client proxy service node").DataType("string")).
		Writes(clusters{}))

	ws.Route(ws.
		GET(fmt.Sprintf("/v1/routes/{%s}/{%s}/{%s}", RouteConfigName, ServiceCluster, ServiceNode)).
		To(ds.ListRoutes).
		Doc("RDS registration").
		Param(ws.PathParameter(RouteConfigName, "route configuration name").DataType("string")).
		Param(ws.PathParameter(ServiceCluster, "client proxy service cluster").DataType("string")).
		Param(ws.PathParameter(ServiceNode, "client proxy service node").DataType("string")).
		Writes(HTTPRouteConfig{}))

	container.Add(ws)
}

// Run starts the server and blocks
func (ds *DiscoveryService) Run() {
	glog.Infof("Starting discovery service at %v", ds.server.Addr)
	if err := ds.server.ListenAndServe(); err != nil {
		glog.Warning(err)
	}
}

// ListEndpoints responds to SDS requests
func (ds *DiscoveryService) ListEndpoints(request *restful.Request, response *restful.Response) {
	hostname, ports, tags := model.ParseServiceKey(request.PathParameter(ServiceKey))
	out := &hosts{}
	for _, ep := range ds.services.Instances(hostname, ports.GetNames(), tags) {
		out.Hosts = append(out.Hosts, &host{
			Address: ep.Endpoint.Address,
			Port:    ep.Endpoint.Port,
		})
	}
	if err := response.WriteEntity(out); err != nil {
		glog.Warning(err)
	}
}

// ListClusters responds to CDS requests
func (ds *DiscoveryService) ListClusters(request *restful.Request, response *restful.Response) {
	_ = ds.services.Services()
	// TODO: fix this
	/*
		if err := response.WriteEntity(clusters{buildClusters(svc)}); err != nil {
			glog.Warning(err)
		}
	*/
}

// ListRoutes responds to RDS requests
func (ds *DiscoveryService) ListRoutes(request *restful.Request, response *restful.Response) {
	if serviceCluster := request.PathParameter(ServiceCluster); serviceCluster != IstioServiceCluster {
		errorResponse(fmt.Sprintf("Unexpected %s %q", ServiceCluster, serviceCluster), response)
		return
	}

	// service-node holds the IP address
	ip := request.PathParameter(ServiceNode)

	// route-config-name holds the listener port
	routeConfigName := request.PathParameter(RouteConfigName)
	port, err := strconv.Atoi(routeConfigName)
	if err != nil {
		errorResponse(fmt.Sprintf("Unexpected %s %q", RouteConfigName, routeConfigName), response)
		return
	}

	httpRouteConfigs, _ := buildRoutes(&ProxyContext{
		Discovery:  ds.services,
		Config:     ds.config,
		MeshConfig: ds.mesh,
		Addrs:      map[string]bool{ip: true},
	})

	routeConfig, ok := httpRouteConfigs[port]
	if !ok {
		errorResponse(fmt.Sprintf("Missing route config for port %d", port), response)
		return
	}

	if err = response.WriteEntity(routeConfig); err != nil {
		glog.Warning(err)
	}
}

func errorResponse(msg string, response *restful.Response) {
	glog.Warning(msg)
	if err := response.WriteErrorString(404, msg); err != nil {
		glog.Warning(err)
	}
}
