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

	multierror "github.com/hashicorp/go-multierror"

	"istio.io/manager/model"
)

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
	clusters = append(clusters, buildClusters(services)...)

	sort.Sort(ListenersByPort(listeners))
	sort.Sort(ClustersByName(clusters))

	// TODO: add catch-all filters to prevent Envoy from crashing
	listeners = append(listeners, Listener{
		Port:           mesh.ProxyPort,
		BindToPort:     true,
		UseOriginalDst: true,
		Filters:        make([]NetworkFilter, 0),
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

// buildListeners uses iptables port redirect to route traffic either into the
// pod or outside the pod to service clusters based on the traffic metadata.
func buildListeners(instances []*model.ServiceInstance, services []*model.Service,
	rules *model.IstioRegistry, mesh *MeshConfig) ([]Listener, []Cluster) {

	localClusters := make([]Cluster, 0)
	listeners := make([]Listener, 0)

	// group by port values to service with the declared port
	type listener struct {
		instances map[model.Protocol][]*model.Service
		services  map[model.Protocol][]*model.Service
	}

	ports := make(map[int]*listener, 0)

	// helper function to work with multi-maps
	ensure := func(port int) {
		if _, ok := ports[port]; !ok {
			ports[port] = &listener{
				instances: make(map[model.Protocol][]*model.Service),
				services:  make(map[model.Protocol][]*model.Service),
			}
		}
	}

	// group all service instances by (target-)port values
	// (assumption: traffic gets redirected from service port to instance port)
	for _, instance := range instances {
		port := instance.Endpoint.Port
		ensure(port)
		ports[port].instances[instance.Endpoint.ServicePort.Protocol] = append(
			ports[port].instances[instance.Endpoint.ServicePort.Protocol], &model.Service{
				Hostname: instance.Service.Hostname,
				Address:  instance.Service.Address,
				Ports:    []*model.Port{instance.Endpoint.ServicePort},
			})
	}

	// group all services by (service-)port values for outgoing traffic
	for _, svc := range services {
		for _, port := range svc.Ports {
			ensure(port.Port)
			ports[port.Port].services[port.Protocol] = append(
				ports[port.Port].services[port.Protocol], &model.Service{
					Hostname: svc.Hostname,
					Address:  svc.Address,
					Ports:    []*model.Port{port},
				})
		}
	}

	suffix := sharedInstanceHost(instances)

	// generate listener for each port
	for port, lst := range ports {
		listener := Listener{
			Port:       port,
			BindToPort: false,
		}

		// append localhost redirect cluster
		localhost := fmt.Sprintf("%s%d", InboundClusterPrefix, port)
		if len(lst.instances) > 0 {
			localClusters = append(localClusters, Cluster{
				Name:             localhost,
				Type:             "static",
				ConnectTimeoutMs: DefaultTimeoutMs,
				LbType:           DefaultLbType,
				Hosts:            []Host{{URL: fmt.Sprintf("tcp://%s:%d", "127.0.0.1", port)}},
			})
		}

		// Envoy uses L4 and L7 filters for TCP and HTTP traffic.
		// In practice, no port has two protocols used by both filters, but we
		// should be careful with not stepping on our feet.

		// The order of the filter insertion is important.
		if len(lst.instances[model.ProtocolTCP]) > 0 {
			listener.Filters = append(listener.Filters, NetworkFilter{
				Type: "read",
				Name: "tcp_proxy",
				Config: NetworkFilterConfig{
					Cluster:    localhost,
					StatPrefix: "inbound_tcp",
				},
			})
		}

		// TODO: TCP routing for outbound based on dst IP
		// TODO: HTTPS protocol for inbound and outbound configuration using TCP routing or SNI
		// TODO: if two service ports have same port or same target port values but
		// different names, we will get duplicate host routes.  Envoy prohibits
		// duplicate entries with identical domains.

		// For HTTP, the routing decision is based on the virtual host.
		hosts := make(map[string]VirtualHost, 0)
		for _, proto := range []model.Protocol{model.ProtocolHTTP, model.ProtocolHTTP2, model.ProtocolGRPC} {
			for _, svc := range lst.services[proto] {
				host := buildVirtualHost(svc, suffix)
				host.Routes = []Route{buildDefaultRoute(svc)}
				hosts[svc.String()] = host
			}

			// If the traffic is sent to a service that has instances co-located with the proxy,
			// we choose the local service instance since we cannot distinguish between inbound and outbound packets.
			// Note that this may not be a problem if the service port and its endpoint port are distinct.
			for _, svc := range lst.instances[proto] {
				host := buildVirtualHost(svc, suffix)
				host.Routes = []Route{{Prefix: "/", Cluster: localhost}}
				hosts[svc.String()] = host
			}
		}

		if len(hosts) > 0 {
			// sort hosts by key (should be non-overlapping domains)
			vhosts := make([]VirtualHost, 0)
			for _, host := range hosts {
				vhosts = append(vhosts, host)
			}
			sort.Sort(HostsByName(vhosts))

			listener.Filters = append(listener.Filters, NetworkFilter{
				Type: "read",
				Name: "http_connection_manager",
				Config: NetworkFilterConfig{
					CodecType:   "auto",
					StatPrefix:  "http",
					AccessLog:   []AccessLog{{Path: DefaultAccessLog}},
					RouteConfig: RouteConfig{VirtualHosts: vhosts},
					Filters:     append(buildFilters(mesh)),
				},
			})
		}

		if len(listener.Filters) > 0 {
			listeners = append(listeners, listener)
		}
	}

	return listeners, localClusters
}

// buildFilter adds a filter for the the mixer and fault injection if specified by routing rule
// TODO: fault injection filter needs to go here
func buildFilters(mesh *MeshConfig) []Filter {
	filters := make([]Filter, 0)

	if len(mesh.MixerAddress) > 0 {
		filters = append(filters, Filter{
			Type: "both",
			Name: "mixer",
		})
	}

	filters = append(filters, Filter{
		Type:   "decoder",
		Name:   "router",
		Config: FilterRouterConfig{},
	})

	return filters
}
