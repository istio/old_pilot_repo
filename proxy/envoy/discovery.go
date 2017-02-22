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
	"net/http"
	"strconv"

	restful "github.com/emicklei/go-restful"
	"github.com/golang/glog"

	"istio.io/manager/model"
	"sort"
)

// DiscoveryService publishes services, clusters, and routes for proxies
type DiscoveryService struct {
	mesh *MeshConfig
	addrs map[string]bool
	services model.ServiceDiscovery
	server   *http.Server
	registry *model.IstioRegistry
}

type hosts struct {
	Hosts []host `json:"hosts"`
}

type host struct {
	Address string `json:"ip_address"`
	Port    int    `json:"port"`

	// Weight is an integer in the range [1, 100] or empty
	Weight int `json:"load_balancing_weight,omitempty"`
}

type clusters struct {
	Clusters []*Cluster `json:"clusters"`
}

type virtualHosts struct {
	VirtualHosts []*VirtualHost `json:"virtual_hosts"`
}

// NewDiscoveryService creates an Envoy discovery service on a given port
func NewDiscoveryService(services model.ServiceDiscovery, registry *model.IstioRegistry, identity *ProxyNode, port int) *DiscoveryService {
	addrs := make(map[string]bool)
	if identity.IP != "" {
		addrs[identity.IP] = true
	}
	glog.V(2).Infof("Local instance address: %#v", addrs)
	out := &DiscoveryService{
		services: services,
		registry: registry,
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
		GET("/v1/registration/{service-key}").
		To(ds.ListEndpoints).
		Doc("SDS registration").
		Param(ws.PathParameter("service-key", "tuple of service name and tag name").DataType("string")).
		Writes(hosts{}))
	ws.Route(ws.
		GET("/v1/clusters/{service-cluster}/{service-node}").
		To(ds.ListClusters).
		Doc("CDS registration").
		Param(ws.PathParameter("service-cluster", "service cluster").DataType("string")).
		Param(ws.PathParameter("service-node", "service node").DataType("string")).
		Writes(clusters{}))
	ws.Route(ws.
		GET("/v1/routes/{route-config-name}/{service-cluster}/{service-node}").
		To(ds.ListHosts).
		Doc("RDS registration").
		Param(ws.PathParameter("route-config-name", "route configuration name").DataType("string")).
		Param(ws.PathParameter("service-cluster", "service cluster").DataType("string")).
		Param(ws.PathParameter("service-node", "service node").DataType("string")).
		Writes(virtualHosts{}))
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
	key := request.PathParameter("service-key")
	hostname, ports, tags := model.ParseServiceKey(key)
	out := make([]host, 0)
	for _, ep := range ds.services.Instances(hostname, ports.GetNames(), tags) {
		out = append(out, host{
			Address: ep.Endpoint.Address,
			Port:    ep.Endpoint.Port,
		})
	}
	if err := response.WriteEntity(hosts{out}); err != nil {
		glog.Warning(err)
	}
}

// ListClusters responds to CDS requests
func (ds *DiscoveryService) ListClusters(request *restful.Request, response *restful.Response) {
	routeConfigs := ds.buildRouteConfigs()

	// collect clusters
	out := make(Clusters, 0)
	for _, routeConfig := range routeConfigs {
		out = append(out, routeConfig.clusters()...)
	}

	if err := response.WriteEntity(clusters{out}); err != nil {
		glog.Warning(err)
	}
}

// ListHosts responds to RDS requests
func (ds *DiscoveryService) ListHosts(request *restful.Request, response *restful.Response) {
	key := request.PathParameter("route-config-name")

	port, err := strconv.ParseInt(key, 10, 32)
	if err != nil {
		glog.Warning(err)
		response.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: this is very inefficient, since we only care about specific port
	routeConfigs := ds.buildRouteConfigs()

	var out []*VirtualHost
	if routeConfig, exists := routeConfigs[int(port)]; exists {
		out = routeConfig.VirtualHosts
		sort.Sort(HostsByName(out))
	}

	if err := response.WriteEntity(virtualHosts{out}); err != nil {
		glog.Warning(err)
	}
}

// TODO: de-duplicate this code.
// TODO: move logic out of the API
func (ds *DiscoveryService) buildRouteConfigs() HTTPRouteConfigs {
	services := ds.services.Services()
	instances := ds.services.HostInstances(ds.addrs)

	outbound := buildOutboundFilters(instances, services, ds.registry, ds.mesh)
	inbound := buildInboundFilters(instances)

	// merge the two sets of route configs
	routeConfigs := make(HTTPRouteConfigs)
	for port, routeConfig := range inbound {
		routeConfigs[port] = routeConfig
	}

	for port, outgoing := range outbound {
		if incoming, ok := routeConfigs[port]; ok {
			// If the traffic is sent to a service that has instances co-located with the proxy,
			// we choose the local service instance since we cannot distinguish between inbound and outbound packets.
			// Note that this may not be a problem if the service port and its endpoint port are distinct.
			routeConfigs[port] = incoming.merge(outgoing)
		} else {
			routeConfigs[port] = outgoing
		}
	}

	return routeConfigs
}