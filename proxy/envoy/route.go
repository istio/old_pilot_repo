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

// Functions related to data-path routes in Envoy config: virtual hosts, clusters,
// routes.

package envoy

import (
	"fmt"
	"strings"

	"istio.io/manager/model"
	"istio.io/manager/model/proxy/alphav1/config"
)

const (
	// OutboundClusterPrefix is the prefix for service clusters external to the proxy instance
	OutboundClusterPrefix = "outbound:"

	// InboundClusterPrefix is the prefix for service clusters co-hosted on the proxy instance
	InboundClusterPrefix = "inbound:"
)

func buildInboundCluster(port int, protocol model.Protocol) Cluster {
	cluster := Cluster{
		Name:             fmt.Sprintf("%s%d", InboundClusterPrefix, port),
		Type:             "static",
		ConnectTimeoutMs: DefaultTimeoutMs,
		LbType:           DefaultLbType,
		Hosts:            []Host{{URL: fmt.Sprintf("tcp://%s:%d", "127.0.0.1", port)}},
	}
	if protocol == model.ProtocolGRPC || protocol == model.ProtocolHTTP2 {
		cluster.Features = "http2"
	}
	return cluster
}

func buildOutboundCluster(svc *model.Service, port *model.Port, tag model.Tag) Cluster {
	key := svc.Key(port, tag)
	cluster := Cluster{
		Name:             OutboundClusterPrefix + key,
		ServiceName:      key,
		Type:             "sds",
		LbType:           DefaultLbType,
		ConnectTimeoutMs: DefaultTimeoutMs,
	}
	if port.Protocol == model.ProtocolGRPC || port.Protocol == model.ProtocolHTTP2 {
		cluster.Features = "http2"
	}
	return cluster
}

func buildRoutes(svc *model.Service, port *model.Port, registry *model.IstioRegistry) []Route {
	routes := make([]Route, 0)
	for _, rule := range registry.DestinationRouteRules(svc.Hostname) {
		route := buildHTTPRoute(rule.Match)
		insertDestination(rule, port, route)
		routes = append(routes, *route)
	}
	routes = append(routes, buildDefaultRoute(svc, port))
	return routes
}

func buildDefaultRoute(svc *model.Service, port *model.Port) Route {
	return Route{
		Prefix:   "/",
		Cluster:  OutboundClusterPrefix + svc.Key(port, nil),
		Clusters: []Cluster{buildOutboundCluster(svc, port, nil)},
	}
}

// insertDestination injects weighted or unweighted destination clusters into envoy route for a service port
func insertDestination(rule *config.RouteRule, port *model.Port, route *Route) {
	if len(rule.Route) > 1 {
		clusters := make([]*WeightedClusterEntry, 0)
		for _, dst := range rule.Route {
			cluster := buildDestination(rule, port, dst)
			clusters = append(clusters, &WeightedClusterEntry{
				Name:   cluster.Name,
				Weight: int(dst.Weight),
			})
			route.Clusters = append(route.Clusters, cluster)
		}
		route.WeightedClusters = &WeightedCluster{Clusters: clusters}
	} else if len(rule.Route) == 1 {
		cluster := buildDestination(rule, port, rule.Route[0])
		route.Cluster = cluster.Name
		route.Clusters = []Cluster{cluster}
	}
}

// buildDestination produces a string for the destination service key
func buildDestination(rule *config.RouteRule, port *model.Port, dst *config.DestinationWeight) Cluster {
	destination := dst.Destination
	// fallback to rule destination
	if len(destination) == 0 {
		destination = rule.Destination
	}
	return buildOutboundCluster(&model.Service{Hostname: destination}, port, dst.Version)
}

func buildMixerCluster(mesh *MeshConfig) *Cluster {
	if len(mesh.MixerAddress) == 0 {
		return nil
	}

	return &Cluster{
		Name:             "mixer",
		Type:             "strict_dns",
		ConnectTimeoutMs: DefaultTimeoutMs,
		LbType:           DefaultLbType,
		Hosts: []Host{
			{
				URL: "tcp://" + mesh.MixerAddress,
			},
		},
	}
}

// buildVirtualHost constructs an entry for VirtualHost for a given service.
// Suffix provides the proxy context information - it is the shared subdomain between co-located
// service instances (e.g. "namespace", "svc", "cluster", "local")
func buildVirtualHost(svc *model.Service, port *model.Port, suffix []string) *VirtualHost {
	hosts := make([]string, 0)
	domains := make([]string, 0)
	parts := strings.Split(svc.Hostname, ".")
	shared := sharedHost(suffix, parts)

	// if shared is "svc.cluster.local", then we can add "name.namespace", "name.namespace.svc", etc
	host := strings.Join(parts[0:len(parts)-len(shared)], ".")
	if len(host) > 0 {
		hosts = append(hosts, host)
	}
	for _, part := range shared {
		if len(host) > 0 {
			host = host + "."
		}
		host = host + part
		hosts = append(hosts, host)
	}

	// add cluster IP host name
	if len(svc.Address) > 0 {
		hosts = append(hosts, svc.Address)
	}

	// add ports
	for _, host := range hosts {
		domains = append(domains, fmt.Sprintf("%s:%d", host, port.Port))

		// default port 80 does not need to be specified
		if port.Port == 80 {
			domains = append(domains, host)
		}
	}

	return &VirtualHost{
		Name:    svc.Key(port, nil),
		Domains: domains,
	}
}

// sharedInstanceHost computes the shared subdomain suffix for co-located instances
func sharedInstanceHost(instances []*model.ServiceInstance) []string {
	hostnames := make([][]string, 0)
	for _, instance := range instances {
		hostnames = append(hostnames, strings.Split(instance.Service.Hostname, "."))
	}
	return sharedHost(hostnames...)
}

// sharedHost computes the shared host name suffix for instances.
// Each host name is split into its domains.
func sharedHost(parts ...[]string) []string {
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	default:
		// longest common suffix
		out := make([]string, 0)
		for i := 1; i <= len(parts[0]); i++ {
			part := ""
			all := true
			for j, host := range parts {
				hostpart := host[len(host)-i]
				if j == 0 {
					part = hostpart
				} else if part != hostpart {
					all = false
					break
				}
			}
			if all {
				out = append(out, part)
			} else {
				break
			}
		}

		// reverse
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return out
	}
}
