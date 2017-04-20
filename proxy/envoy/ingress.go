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
	"io/ioutil"
	"sort"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/hashicorp/go-multierror"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/manager/model"
	"istio.io/manager/proxy"
)

const (
	ingressNode = "ingress"
	certFile    = "/etc/tls.crt"
	keyFile     = "/etc/tls.key"
)

type ingressWatcher struct {
	agent proxy.Agent
	mesh  *proxyconfig.ProxyMeshConfig
}

// NewIngressWatcher creates a new ingress watcher instance with an agent
func NewIngressWatcher(mesh *proxyconfig.ProxyMeshConfig) (Watcher, error) {
	agent := proxy.NewAgent(runEnvoy(mesh, ingressNode), cleanupEnvoy(mesh), 10, 100*time.Millisecond)
	out := &ingressWatcher{
		agent: agent,
		mesh:  mesh,
	}
	return out, nil
}

func (w *ingressWatcher) Run(stop <-chan struct{}) {
	go w.agent.Run(stop)
	//w.agent.ScheduleConfigUpdate(generateIngress(w.context))
}

func generateIngress(mesh *proxyconfig.ProxyMeshConfig, tls *model.TLSSecret) *Config {
	listeners := []*Listener{
		buildHTTPListener(mesh, nil, WildcardAddress, 80, true),
	}

	if tls != nil {
		if err := writeTLS(certFile, keyFile, tls); err != nil {
			glog.Warning("Failed to write cert/key")
		} else {
			ssl := &SSLContext{
				CertChainFile:  certFile,
				PrivateKeyFile: keyFile,
			}
			listener := buildHTTPListener(mesh, nil, WildcardAddress, 443, true)
			listener.SSLContext = ssl
			listeners = append(listeners, listener)
		}
	}

	return buildConfig(listeners, nil, mesh)
}

func writeTLS(certFile, keyFile string, tls *model.TLSSecret) error {
	if err := ioutil.WriteFile(certFile, tls.Certificate, 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(keyFile, tls.PrivateKey, 0755); err != nil {
		return err
	}

	return nil
}

func buildIngressRoutes(ingressRules []*proxyconfig.RouteRule) (HTTPRouteConfigs, string) {
	// build vhosts
	vhosts := make(map[string][]*HTTPRoute)
	vhostsTLS := make(map[string][]*HTTPRoute)
	tlsAll := ""

	for _, rule := range ingressRules {
		route, tls, err := buildIngressRoute(rule)
		if err != nil {
			glog.Warningf("Error constructing Envoy route from ingress rule: %v", err)
			continue
		}

		host := "*"
		if rule.Match != nil {
			if authority, ok := rule.Match.HttpHeaders["authority"]; ok {
				switch match := authority.GetMatchType().(type) {
				case *proxyconfig.StringMatch_Exact:
					host = match.Exact
				default:
					glog.Warningf("Unsupported match type for authority condition %T, falling back to %q", match, host)
					continue
				}
			}
		}
		if tls != "" {
			vhostsTLS[host] = append(vhostsTLS[host], route)
			if tlsAll == "" {
				tlsAll = tls
			} else {
				glog.Warningf("Multiple secrets detected %q and %q", tls, tlsAll)
				if tls < tlsAll {
					tlsAll = tls
				}
			}
		} else {
			vhosts[host] = append(vhosts[host], route)
		}
	}

	// normalize config
	rc := &HTTPRouteConfig{}
	for host, routes := range vhosts {
		sort.Sort(RoutesByPath(routes))
		rc.VirtualHosts = append(rc.VirtualHosts, &VirtualHost{
			Name:    host,
			Domains: []string{host},
			Routes:  routes,
		})
	}

	rcTLS := &HTTPRouteConfig{}
	for host, routes := range vhostsTLS {
		sort.Sort(RoutesByPath(routes))
		rcTLS.VirtualHosts = append(rcTLS.VirtualHosts, &VirtualHost{
			Name:    host,
			Domains: []string{host},
			Routes:  routes,
		})
	}

	configs := HTTPRouteConfigs{80: rc, 443: rcTLS}
	configs.normalize()
	return configs, tlsAll
}

// buildIngressRoute translates an ingress rule to an Envoy route
func buildIngressRoute(rule *proxyconfig.RouteRule) (*HTTPRoute, string, error) {
	route := &HTTPRoute{
		Path:   "",
		Prefix: "/",
	}

	if rule.Match != nil && rule.Match.HttpHeaders != nil {
		if uri, ok := rule.Match.HttpHeaders[HeaderURI]; ok {
			switch m := uri.MatchType.(type) {
			case *proxyconfig.StringMatch_Exact:
				route.Path = m.Exact
				route.Prefix = ""
			case *proxyconfig.StringMatch_Prefix:
				route.Path = ""
				route.Prefix = m.Prefix
			case *proxyconfig.StringMatch_Regex:
				return nil, "", fmt.Errorf("unsupported route match condition: regex")
			}
		}
	}

	clusters := make([]*WeightedClusterEntry, 0)
	tlsAll := ""
	for _, dst := range rule.Route {
		// fetch route destination, or fallback to rule destination
		destination := dst.Destination
		if destination == "" {
			destination = rule.Destination
		}

		port, tags, tls, err := extractPortAndTags(dst)
		if err != nil {
			return nil, "", multierror.Prefix(err, "failed to extract routing rule destination port")
		}

		if tls != "" {
			if tlsAll == "" {
				tlsAll = tls
			} else {
				return nil, "", fmt.Errorf("multiple TLS secrets detected %q and %q", tlsAll, tls)
			}
		}

		cluster := buildOutboundCluster(destination, port, tags)
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

	// Ensure all destination clusters have the same port number.
	//
	// This is currently required for doing host header rewrite (host:port),
	// which is scoped to the entire route.
	// This restriction can be relaxed by constructing multiple envoy.Route objects
	// per config.RouteRule, and doing weighted load balancing using Runtime.
	portSet := make(map[int]struct{}, 1)
	for _, cluster := range route.clusters {
		portSet[cluster.port.Port] = struct{}{}
	}
	if len(portSet) > 1 {
		return nil, "", fmt.Errorf("unsupported multiple destination ports per ingress route rule")
	}

	// Rewrite the host header so that inbound proxies can match incoming traffic
	route.HostRewrite = fmt.Sprintf("%s:%d", rule.Destination, route.clusters[0].port.Port)

	return route, tlsAll, nil
}

// extractPortAndTags extracts the destination service port from the given destination,
// as well as its tags (after clearing meta-tags describing the port).
// Note that this is a temporary measure to communicate the destination service's port
// to the proxy configuration generator. This can be improved by using
// a dedicated model object for IngressRule (instead of reusing RouteRule),
// which exposes the necessary target port field within the "Route" field.
func extractPortAndTags(dst *proxyconfig.DestinationWeight) (*model.Port, model.Tags, string, error) {
	portNum, err := strconv.Atoi(dst.Tags["servicePort.port"])
	if err != nil {
		return nil, nil, "", err
	}
	portName, ok := dst.Tags["servicePort.name"]
	if !ok {
		return nil, nil, "", fmt.Errorf("no name specified for service port %d", portNum)
	}
	portProto, ok := dst.Tags["servicePort.protocol"]
	if !ok {
		return nil, nil, "", fmt.Errorf("no protocol specified for service port %d", portNum)
	}
	tls, ok := dst.Tags["tlsSecret"]
	if !ok {
		return nil, nil, "", fmt.Errorf("missing TLS secret %d", portNum)
	}

	port := &model.Port{
		Port:     portNum,
		Name:     portName,
		Protocol: model.Protocol(portProto),
	}

	var tags model.Tags
	if len(dst.Tags) > 4 {
		tags = make(model.Tags, len(dst.Tags)-3)
		for k, v := range dst.Tags {
			tags[k] = v
		}
		delete(tags, "servicePort.port")
		delete(tags, "servicePort.name")
		delete(tags, "servicePort.protocol")
		delete(tags, "tlsSecret")
	}

	return port, tags, tls, nil
}
