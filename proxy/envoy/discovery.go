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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strconv"
	"sync"

	restful "github.com/emicklei/go-restful"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/manager/model"
)

// DiscoveryService publishes services, clusters, and routes for all proxies
type DiscoveryService struct {
	services   model.ServiceDiscovery
	controller model.Controller
	config     *model.IstioRegistry
	mesh       *proxyconfig.ProxyMeshConfig
	server     *http.Server

	disableCache bool
	cacheMu      sync.Mutex
	cache        map[string][]byte
	cacheStats   discoveryCacheStats
}

type discoveryCacheStatEntry struct {
	Hit  uint64 `json:"hit"`
	Miss uint64 `json:"miss"`
}

type discoveryCacheStats struct {
	Stats map[string]*discoveryCacheStatEntry `json:"cache_stats"`
}

type hosts struct {
	Hosts []*host `json:"hosts"`
}

type host struct {
	Address string `json:"ip_address"`
	Port    int    `json:"port"`

	// Weight is an integer in the range [1, 100] or empty
	Weight int `json:"load_balancing_weight,omitempty"`
}

// Request parameters for discovery services
const (
	ServiceKey      = "service-key"
	ServiceCluster  = "service-cluster"
	ServiceNode     = "service-node"
	RouteConfigName = "route-config-name"
)

// DiscoveryServiceOptions contains options for create a new discovery
// service instance.
type DiscoveryServiceOptions struct {
	Services        model.ServiceDiscovery
	Controller      model.Controller
	Config          *model.IstioRegistry
	Mesh            *proxyconfig.ProxyMeshConfig
	Port            int
	EnableProfiling bool
	EnableCaching   bool
}

// NewDiscoveryService creates an Envoy discovery service on a given port
func NewDiscoveryService(o DiscoveryServiceOptions) (*DiscoveryService, error) {
	out := &DiscoveryService{
		services:     o.Services,
		controller:   o.Controller,
		config:       o.Config,
		mesh:         o.Mesh,
		cache:        make(map[string][]byte),
		cacheStats:   discoveryCacheStats{make(map[string]*discoveryCacheStatEntry)},
		disableCache: !o.EnableCaching,
	}
	container := restful.NewContainer()
	if o.EnableProfiling {
		container.ServeMux.HandleFunc("/debug/pprof/", pprof.Index)
		container.ServeMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		container.ServeMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		container.ServeMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		container.ServeMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	out.Register(container)
	out.server = &http.Server{Addr: ":" + strconv.Itoa(o.Port), Handler: container}

	// Flush cached discovery responses whenever services, service
	// instances, or routing configuration changes.
	serviceHandler := func(s *model.Service, e model.Event) { out.clearCache() }
	if err := o.Controller.AppendServiceHandler(serviceHandler); err != nil {
		return nil, err
	}
	instanceHandler := func(s *model.ServiceInstance, e model.Event) { out.clearCache() }
	if err := o.Controller.AppendInstanceHandler(instanceHandler); err != nil {
		return nil, err
	}
	configHandler := func(k model.Key, m proto.Message, e model.Event) { out.clearCache() }
	if err := o.Controller.AppendConfigHandler(model.RouteRule, configHandler); err != nil {
		return nil, err
	}
	if err := o.Controller.AppendConfigHandler(model.DestinationPolicy, configHandler); err != nil {
		return nil, err
	}

	return out, nil
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
		Writes(ClusterManager{}))

	ws.Route(ws.
		GET(fmt.Sprintf("/v1/routes/{%s}/{%s}/{%s}", RouteConfigName, ServiceCluster, ServiceNode)).
		To(ds.ListRoutes).
		Doc("RDS registration").
		Param(ws.PathParameter(RouteConfigName, "route configuration name").DataType("string")).
		Param(ws.PathParameter(ServiceCluster, "client proxy service cluster").DataType("string")).
		Param(ws.PathParameter(ServiceNode, "client proxy service node").DataType("string")).
		Writes(HTTPRouteConfig{}))

	ws.Route(ws.
		GET(fmt.Sprintf("/cache_stats")).
		To(ds.GetCacheStats).
		Writes(discoveryCacheStats{}))

	container.Add(ws)
}

// Run starts the server and blocks
func (ds *DiscoveryService) Run() {
	glog.Infof("Starting discovery service at %v", ds.server.Addr)
	if err := ds.server.ListenAndServe(); err != nil {
		glog.Warning(err)
	}
}

// GetCacheStats returns the statistics for cached discovery responses.
func (ds *DiscoveryService) GetCacheStats(_ *restful.Request, response *restful.Response) {
	ds.cacheMu.Lock()
	defer ds.cacheMu.Unlock()
	if err := response.WriteEntity(ds.cacheStats); err != nil {
		glog.Warning(err)
	}
}

func (ds *DiscoveryService) clearCache() {
	ds.cacheMu.Lock()
	defer ds.cacheMu.Unlock()
	glog.Infof("Cleared discovery service cache")
	ds.cache = make(map[string][]byte)
}

// caller must hold cache mutex lock
func (ds *DiscoveryService) cacheHitMiss(key string, hit bool) {
	if ds.disableCache {
		return
	}
	s, ok := ds.cacheStats.Stats[key]
	if !ok {
		s = &discoveryCacheStatEntry{}
		ds.cacheStats.Stats[key] = s
	}
	if hit {
		s.Hit++
	} else {
		s.Miss++
	}
}

// ListEndpoints responds to SDS requests
func (ds *DiscoveryService) ListEndpoints(request *restful.Request, response *restful.Response) {
	ds.cacheMu.Lock()
	defer ds.cacheMu.Unlock()

	key := request.Request.URL.String()
	out, cached := ds.cache[key]
	if ds.disableCache || !cached {
		hostname, ports, tags := model.ParseServiceKey(request.PathParameter(ServiceKey))
		// envoy expects an empty array if no hosts are available
		hostArray := make([]*host, 0)
		for _, ep := range ds.services.Instances(hostname, ports.GetNames(), tags) {
			hostArray = append(hostArray, &host{
				Address: ep.Endpoint.Address,
				Port:    ep.Endpoint.Port,
			})
		}
		var err error
		if out, err = json.MarshalIndent(hosts{Hosts: hostArray}, " ", " "); err != nil {
			errorResponse(response, http.StatusInternalServerError, err.Error())
			return
		}
		ds.cache[key] = out
	}
	ds.cacheHitMiss(key, cached)
	writeResponse(response, out)
}

// ListClusters responds to CDS requests for all outbound clusters
func (ds *DiscoveryService) ListClusters(request *restful.Request, response *restful.Response) {
	ds.cacheMu.Lock()
	defer ds.cacheMu.Unlock()

	key := request.Request.URL.String()
	out, cached := ds.cache[key]
	if ds.disableCache || !cached {
		if sc := request.PathParameter(ServiceCluster); sc != ds.mesh.IstioServiceCluster {
			errorResponse(response, http.StatusNotFound,
				fmt.Sprintf("Unexpected %s %q", ServiceCluster, sc))
			return
		}

		// service-node holds the IP address
		ip := request.PathParameter(ServiceNode)
		// CDS computes clusters that are referenced by RDS routes for a particular proxy node
		// TODO: this implementation is inefficient as it is recomputing all the routes for all proxies
		// There is a lot of potential to cache and reuse cluster definitions across proxies and also
		// skip computing the actual HTTP routes
		instances := ds.services.HostInstances(map[string]bool{ip: true})
		services := ds.services.Services()
		httpRouteConfigs := buildOutboundHTTPRoutes(instances, services, &ProxyContext{
			Discovery:  ds.services,
			Config:     ds.config,
			MeshConfig: ds.mesh,
			IPAddress:  ip,
		})

		// de-duplicate and canonicalize clusters
		clusters := httpRouteConfigs.clusters().normalize()

		// apply custom policies for HTTP clusters
		for _, cluster := range clusters {
			insertDestinationPolicy(ds.config, cluster)
		}

		var err error
		if out, err = json.MarshalIndent(ClusterManager{Clusters: clusters}, " ", " "); err != nil {
			errorResponse(response, http.StatusInternalServerError, err.Error())
			return
		}
		ds.cache[key] = out
	}
	ds.cacheHitMiss(key, cached)
	writeResponse(response, out)
}

// ListRoutes responds to RDS requests, used by HTTP routes
// Routes correspond to HTTP routes and use the listener port as the route name
// to identify HTTP filters in the config. Service node value holds the local proxy identity.
func (ds *DiscoveryService) ListRoutes(request *restful.Request, response *restful.Response) {
	ds.cacheMu.Lock()
	defer ds.cacheMu.Unlock()

	key := request.Request.URL.String()
	out, cached := ds.cache[key]
	if ds.disableCache || !cached {
		if sc := request.PathParameter(ServiceCluster); sc != ds.mesh.IstioServiceCluster {
			errorResponse(response, http.StatusNotFound,
				fmt.Sprintf("Unexpected %s %q", ServiceCluster, sc))
			return
		}
		// service-node holds the IP address
		ip := request.PathParameter(ServiceNode)

		// route-config-name holds the listener port
		routeConfigName := request.PathParameter(RouteConfigName)
		port, err := strconv.Atoi(routeConfigName)
		if err != nil {
			errorResponse(response, http.StatusNotFound,
				fmt.Sprintf("Unexpected %s %q", RouteConfigName, routeConfigName))
			return
		}

		instances := ds.services.HostInstances(map[string]bool{ip: true})
		services := ds.services.Services()
		httpRouteConfigs := buildOutboundHTTPRoutes(instances, services, &ProxyContext{
			Discovery:  ds.services,
			Config:     ds.config,
			MeshConfig: ds.mesh,
			IPAddress:  ip,
		})

		routeConfig, ok := httpRouteConfigs[port]
		if !ok {
			errorResponse(response, http.StatusNotFound,
				fmt.Sprintf("Missing route config for port %d", port))
			return
		}
		if out, err = json.MarshalIndent(routeConfig, " ", " "); err != nil {
			errorResponse(response, http.StatusInternalServerError, err.Error())
			return
		}
		ds.cache[key] = out
	}
	ds.cacheHitMiss(key, cached)
	writeResponse(response, out)
}

func errorResponse(r *restful.Response, status int, msg string) {
	glog.Warning(msg)
	if err := r.WriteErrorString(status, msg); err != nil {
		glog.Warning(err)
	}
}

func writeResponse(r *restful.Response, data []byte) {
	r.WriteHeader(http.StatusOK)
	r.Header().Set("Content-Type", "application/json")
	if _, err := r.Write(data); err != nil {
		glog.Warning(err)
	}
}
