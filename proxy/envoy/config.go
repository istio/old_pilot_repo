// Copyright 2017 Google Inc.
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
	"io"
	"os"
	"sort"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"

	"istio.io/manager/model"
)

// TODO: TCP routing for outbound based on dst IP
// TODO: HTTPS protocol for inbound and outbound configuration using TCP routing or SNI
// TODO: if two service ports have same port or same target port values but
// different names, we will get duplicate host routes.  Envoy prohibits
// duplicate entries with identical domains.

// WriteFile saves config to a file
func (conf *Config) WriteFile(fname string) error {
	file, err := os.Create(fname)
	if err != nil {
		return err
	}

	if err := conf.Write(file); err != nil {
		err = multierror.Append(err, file.Close())
		return err
	}

	return file.Close()
}

func (conf *Config) Write(w io.Writer) error {
	out, err := json.MarshalIndent(&conf, "", "  ")
	if err != nil {
		return err
	}

	_, err = w.Write(out)
	return err
}

// Generate Envoy configuration for service instances co-located with Envoy and all services in the mesh
func Generate(instances []*model.ServiceInstance, services []*model.Service,
	rules *model.IstioRegistry, mesh *MeshConfig) (*Config, error) {
	listeners, clusters := buildListeners(instances, services, rules, mesh)

	// set bind to port values
	for _, listener := range listeners {
		listener.BindToPort = false
	}

	listeners = append(listeners, Listener{
		Port:           mesh.ProxyPort,
		BindToPort:     true,
		UseOriginalDst: true,
		Filters:        make([]*NetworkFilter, 0),
	})

	return &Config{
		Listeners: listeners,
		Admin: Admin{
			AccessLogPath: DefaultAccessLog,
			Port:          mesh.AdminPort,
		},
		ClusterManager: ClusterManager{
			Clusters: clusters,
			SDS: SDS{
				Cluster: Cluster{
					Name:             "sds",
					Type:             "strict_dns",
					ConnectTimeoutMs: DefaultTimeoutMs,
					LbType:           DefaultLbType,
					Hosts: []Host{
						{
							URL: "tcp://" + mesh.DiscoveryAddress,
						},
					},
				},
				RefreshDelayMs: 1000,
			},
		},
	}, nil
}

func buildListeners(instances []*model.ServiceInstance, services []*model.Service,
	rules *model.IstioRegistry, mesh *MeshConfig) ([]Listener, Clusters) {
	outbound, outboundClusters := buildOutboundFilters(instances, services, rules, mesh)
	inbound, inboundClusters := buildInboundFilters(instances)
	clusters := append(inboundClusters, outboundClusters...)

	// merge the two sets of route configs
	configs := make(RouteConfigs)
	for port, config := range inbound {
		configs[port] = config
	}
	for port, outgoing := range outbound {
		if incoming, ok := configs[port]; ok {
			// If the traffic is sent to a service that has instances co-located with the proxy,
			// we choose the local service instance since we cannot distinguish between inbound and outbound packets.
			// Note that this may not be a problem if the service port and its endpoint port are distinct.
			configs[port] = incoming.Merge(outgoing)
		} else {
			configs[port] = outgoing
		}
	}

	listeners := make([]Listener, 0)
	for port, config := range configs {
		sort.Sort(HostsByName(config.VirtualHosts))
		listeners = append(listeners, Listener{
			Port: port,
			Filters: []*NetworkFilter{&NetworkFilter{
				Type: "read",
				Name: HTTPConnectionManager,
				Config: NetworkFilterConfig{
					CodecType:  "auto",
					StatPrefix: "http",
					AccessLog: []AccessLog{AccessLog{
						Path: DefaultAccessLog,
					}},
					RouteConfig: config,
					Filters: []Filter{Filter{
						Type:   "decoder",
						Name:   "router",
						Config: FilterRouterConfig{},
					}},
				},
			}},
		})
	}
	sort.Sort(ListenersByPort(listeners))
	return listeners, clusters.Normalize()
}

func buildOutboundFilters(instances []*model.ServiceInstance, services []*model.Service,
	rules *model.IstioRegistry, mesh *MeshConfig) (RouteConfigs, Clusters) {
	// used for shortcut domain names for outbound hostnames
	suffix := sharedInstanceHost(instances)
	httpConfigs := make(RouteConfigs)
	clusters := buildClusters(services)

	// outbound connections/requests are redirected to service ports; we create a
	// map for each service port to define filters
	for _, service := range services {
		for _, port := range service.Ports {
			switch port.Protocol {
			case model.ProtocolHTTP, model.ProtocolHTTP2, model.ProtocolGRPC:
				host := buildVirtualHost(service, port, suffix)
				host.Routes = append(host.Routes, buildDefaultRoute(service, port))
				http := httpConfigs.EnsurePort(port.Port)
				http.VirtualHosts = append(http.VirtualHosts, host)
			default:
				glog.Warningf("Unsupported inbound protocol: %v", port.Protocol)
			}
		}
	}

	return httpConfigs, clusters
}

func buildInboundFilters(instances []*model.ServiceInstance) (RouteConfigs, Clusters) {
	// used for shortcut domain names for hostnames
	suffix := sharedInstanceHost(instances)
	httpConfigs := make(RouteConfigs)
	clusters := make(Clusters, 0)

	// inbound connections/requests are redirected to endpoint port but appear to be sent
	// to the service port
	for _, instance := range instances {
		port := instance.Endpoint.ServicePort
		cluster := Cluster{
			Name:             fmt.Sprintf("%s%d", InboundClusterPrefix, instance.Endpoint.Port),
			Type:             "static",
			ConnectTimeoutMs: DefaultTimeoutMs,
			LbType:           DefaultLbType,
			Hosts:            []Host{{URL: fmt.Sprintf("tcp://%s:%d", "127.0.0.1", instance.Endpoint.Port)}},
		}
		clusters = append(clusters, cluster)

		switch port.Protocol {
		case model.ProtocolHTTP, model.ProtocolHTTP2, model.ProtocolGRPC:
			host := buildVirtualHost(instance.Service, port, suffix)
			host.Routes = append(host.Routes, Route{Prefix: "/", Cluster: cluster.Name})
			http := httpConfigs.EnsurePort(instance.Endpoint.Port)
			http.VirtualHosts = append(http.VirtualHosts, host)
		default:
			glog.Warningf("Unsupported outbound protocol: %v", port.Protocol)
		}
	}

	return httpConfigs, clusters
}
