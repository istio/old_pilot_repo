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
	"reflect"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"

	"istio.io/manager/model"
	"istio.io/manager/model/proxy/alphav1/config"
	"sort"
)

type ingressWatcher struct {
	agent     Agent
	discovery model.ServiceDiscovery
	registry  *model.IstioRegistry
	mesh      *MeshConfig
}

// NewIngressWatcher creates a new ingress watcher instance with an agent
func NewIngressWatcher(discovery model.ServiceDiscovery, ctl model.Controller,
	registry *model.IstioRegistry, mesh *MeshConfig, identity *ProxyNode) (Watcher, error) {

	out := &ingressWatcher{
		agent:     NewAgent(mesh.BinaryPath, mesh.ConfigPath, identity.Name),
		discovery: discovery,
		registry:  registry,
		mesh:      mesh,
	}

	err := ctl.AppendConfigHandler(model.IngressRule,
		func(model.Key, proto.Message, model.Event) { out.reload() })

	if err != nil {
		return nil, err
	}
	return out, nil
}

func (w *ingressWatcher) reload() {
	config, err := w.generateConfig()
	if err != nil {
		glog.Warningf("Failed to generate Envoy configuration: %v", err)
		return
	}

	current := w.agent.ActiveConfig()
	if reflect.DeepEqual(config, current) {
		glog.V(2).Info("Configuration is identical, skipping reload")
		return
	}

	// TODO: add retry logic
	if err := w.agent.Reload(config); err != nil {
		glog.Warningf("Envoy reload error: %v", err)
		return
	}

	// Add a short delay to de-risk a potential race condition in envoy hot reload code.
	// The condition occurs when the active Envoy instance terminates in the middle of
	// the Reload() function.
	time.Sleep(256 * time.Millisecond)
}

func (w *ingressWatcher) generateConfig() (*Config, error) {
	// TODO: Configurable namespace?
	rules := w.registry.IngressRules("")

	// Phase 1: group rules by host
	rulesByHost := make(map[string][]*config.RouteRule, len(rules))
	for _, rule := range rules {
		host := "*"
		if rule.Match != nil {
			if authority, ok := rule.Match.Http["authority"]; ok {
				switch match := authority.GetMatchType().(type) {
				case *config.StringMatch_Exact:
					host = match.Exact
				default:
					glog.Warningf("Unsupported match type for authority condition: %T", match)
				}
			}
		}

		hostRules, ok := rulesByHost[host]
		if !ok {
			hostRules = make([]*config.RouteRule, 0, 1)
			rulesByHost[host] = hostRules
		}
		hostRules = append(hostRules, rule)
	}

	// Phase 2: create a VirtualHost for each host
	vhosts := make([]*VirtualHost, 0, len(rulesByHost))
	for host, hostRules := range rulesByHost {
		routes := make([]*Route, 0, len(hostRules))
		for _, rule := range hostRules {
			routes = append(routes, buildIngressRoute(rule))
		}
		sort.Sort(RoutesByPath(routes))
		vhost := &VirtualHost{
			Name:    host,
			Domains: []string{host},
			Routes:  routes,
		}
		vhosts = append(vhosts, vhost)
	}
	sort.Sort(HostsByName(vhosts))

	rConfig := &RouteConfig{VirtualHosts: vhosts}

	httpListener := &Listener{
		Port:       80,
		BindToPort: true,
		Filters: []*NetworkFilter{
			{
				Type: "read",
				Name: HTTPConnectionManager,
				Config: NetworkFilterConfig{
					CodecType:   "auto",
					StatPrefix:  "http",
					AccessLog:   []AccessLog{{Path: DefaultAccessLog}},
					RouteConfig: rConfig,
					Filters: []Filter{
						{
							Type:   "decoder",
							Name:   "router",
							Config: FilterRouterConfig{},
						},
					},
				},
			},
		},
	}

	// TODO: HTTPS listener
	listeners := []*Listener{httpListener}
	clusters := Clusters(rConfig.clusters()).Normalize()

	return &Config{
		Listeners: listeners,
		Admin: Admin{
			AccessLogPath: DefaultAccessLog,
			Port:          w.mesh.AdminPort,
		},
		ClusterManager: ClusterManager{
			Clusters: clusters,
			SDS: SDS{
				Cluster:        buildSDSCluster(w.mesh),
				RefreshDelayMs: 1000,
			},
		},
	}, nil

}

// buildIngressRoute translates an ingress rule to an Envoy route
func buildIngressRoute(rule *config.RouteRule) *Route {
	route := &Route{
		Path:   "",
		Prefix: "/",
	}

	if rule.Match != nil {
		if uri, ok := rule.Match.Http[HeaderURI]; ok {
			switch m := uri.MatchType.(type) {
			case *config.StringMatch_Exact:
				route.Path = m.Exact
				route.Prefix = ""
			case *config.StringMatch_Prefix:
				route.Path = ""
				route.Prefix = m.Prefix
			case *config.StringMatch_Regex:
				glog.Warningf("Unsupported route match condition: regex")
			}
		}
	}

	clusters := make([]*WeightedClusterEntry, 0)
	for _, dst := range rule.Route {
		// fetch route destination, or fallback to rule destination
		destination := dst.Destination
		if destination == "" {
			destination = rule.Destination
		}

		// A temporary measure to communicate the destination service's port
		// to the proxy configuration generator. This can be improved by using
		// a dedicated model object for IngressRule (instead of reusing RouteRule),
		// which exposes the necessary target port field within the "Route" field.
		port, err := strconv.Atoi(dst.Tags["servicePort"])
		if err != nil {
			glog.Warning("Failed to parse routing rule destination port: %v", err)
			continue
		}
		cPort := &model.Port{
			Port:     port,
			Protocol: model.ProtocolHTTP,
		}

		// Copy the destination tags, if any, but omit the servicePort
		tags := make(map[string]string, len(dst.Tags))
		for k, v := range dst.Tags {
			tags[k] = v
		}
		delete(tags, "servicePort")

		cluster := buildOutboundCluster(destination, cPort, tags)
		clusters = append(clusters, &WeightedClusterEntry{
			Name:   cluster.Name,
			Weight: int(dst.Weight),
		})
		route.clusters = append(route.clusters, cluster)
	}
	route.WeightedClusters = &WeightedCluster{Clusters: clusters}

	// rewrite to a single cluster if it's one weighted cluster
	if len(rule.Route) == 1 {
		route.Cluster = route.WeightedClusters.Clusters[0].Name
		route.WeightedClusters = nil
	}

	return route
}

func vhostMapToSlice(m map[string]VirtualHost) []VirtualHost {
	// Put aside the wildcard domain's VirtualHost,
	// so that we can ensure it's last on the ordered slice
	wildcardDomain, hasWildcard := m["*"]
	delete(m, "*")

	s := make([]VirtualHost, 0, len(m))
	for _, v := range m {
		s = append(s, v)
	}

	if hasWildcard {
		s = append(s, wildcardDomain)
	}

	return s
}

func clusterMapToSlice(m map[string]Cluster) []Cluster {
	s := make([]Cluster, 0, len(m))
	for _, v := range m {
		s = append(s, v)
	}
	return s
}
