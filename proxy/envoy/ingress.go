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
	"errors"
	"fmt"
	"path"
	"sort"

	"github.com/golang/glog"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/model"
	"istio.io/pilot/proxy"
)

func buildIngressListeners(mesh *proxyconfig.MeshConfig,
	discovery model.ServiceDiscovery,
	config model.IstioConfigStore,
	ingress proxy.Node) Listeners {
	listeners := Listeners{
		buildHTTPListener(mesh, ingress, nil, nil, WildcardAddress, 80, "80", true),
	}

	// lack of SNI in Envoy implies that TLS secrets are attached to listeners
	// therefore, we should first check that TLS endpoint is needed before shipping TLS listener
	_, secret := buildIngressRoutes(mesh, discovery, config)
	if secret != "" {
		listener := buildHTTPListener(mesh, ingress, nil, nil, WildcardAddress, 443, "443", true)
		listener.SSLContext = &SSLContext{
			CertChainFile:  path.Join(proxy.IngressCertsPath, proxy.IngressCertFilename),
			PrivateKeyFile: path.Join(proxy.IngressCertsPath, proxy.IngressKeyFilename),
		}
		listeners = append(listeners, listener)
	}

	return listeners
}

func buildIngressRoutes(mesh *proxyconfig.MeshConfig,
	discovery model.ServiceDiscovery,
	config model.IstioConfigStore) (HTTPRouteConfigs, string) {
	// build vhosts
	vhosts := make(map[string][]*HTTPRoute)
	vhostsTLS := make(map[string][]*HTTPRoute)
	tlsAll := ""

	rules, _ := config.List(model.IngressRule.Type, model.NamespaceAll)
	for _, rule := range rules {
		routes, tls, err := buildIngressRoute(mesh, rule, discovery, config)
		if err != nil {
			glog.Warningf("Error constructing Envoy route from ingress rule: %v", err)
			continue
		}

		host := getAuthorityFromIngressRule(rule)
		if host == "*" {
			continue
		}
		if tls != "" {
			vhostsTLS[host] = append(vhostsTLS[host], routes...)
			if tlsAll == "" {
				tlsAll = tls
			} else if tlsAll != tls {
				glog.Warningf("Multiple secrets detected %s and %s", tls, tlsAll)
				if tls < tlsAll {
					tlsAll = tls
				}
			}
		} else {
			vhosts[host] = append(vhosts[host], routes...)
		}
	}

	// normalize config
	rc := &HTTPRouteConfig{VirtualHosts: make([]*VirtualHost, 0)}
	for host, routes := range vhosts {
		sort.Sort(RoutesByPath(routes))
		rc.VirtualHosts = append(rc.VirtualHosts, &VirtualHost{
			Name:    host,
			Domains: []string{host},
			Routes:  routes,
		})
	}

	rcTLS := &HTTPRouteConfig{VirtualHosts: make([]*VirtualHost, 0)}
	for host, routes := range vhostsTLS {
		sort.Sort(RoutesByPath(routes))
		rcTLS.VirtualHosts = append(rcTLS.VirtualHosts, &VirtualHost{
			Name:    host,
			Domains: []string{host},
			Routes:  routes,
		})
	}

	configs := HTTPRouteConfigs{80: rc, 443: rcTLS}
	return configs.normalize(), tlsAll
}

// buildIngressRoute translates an ingress rule to an Envoy route
func buildIngressRoute(mesh *proxyconfig.MeshConfig,
	rule model.Config,
	discovery model.ServiceDiscovery,
	config model.IstioConfigStore) ([]*HTTPRoute, string, error) {
	ingressRule := rule.Spec.(*proxyconfig.IngressRule)
	destination := model.ResolveHostname(rule.ConfigMeta, ingressRule.Destination)
	service, exists := discovery.GetService(destination)
	if !exists {
		return nil, "", fmt.Errorf("cannot find service %q", destination)
	}
	tls := ingressRule.TlsSecret
	servicePort, err := extractPort(service, ingressRule)
	if err != nil {
		return nil, "", err
	}
	if !servicePort.Protocol.IsHTTP() {
		return nil, "", fmt.Errorf("unsupported protocol %q for %q", servicePort.Protocol, service.Hostname)
	}

	// unfold the rules for the destination port
	routes := buildDestinationHTTPRoutes(service, servicePort, nil, config)

	// Select the routes for a destination that are applicable from Ingress.
	// Instead of creating composite rules (combining prefixes), to avoid ambiguity
	// we require end users to create dedicated route rules that match exactly with
	// the match condition allowed at the Ingress controller (path/prefix/regex + host)
	// Once a matching route is found, other properties of the route such as header match,
	// can be applied to the ingress route.
	out := make([]*HTTPRoute, 0)

	// filter by path, prefix, regex from the ingress
	ingressRoute := buildHTTPRouteMatch(ingressRule.Match)
	ingressHost := getAuthorityFromIngressRule(rule)

	for _, route := range routes {
		// Only consider routes whose path/prefix/regex and host
		// match EXACTLY with the ingress Rule's values.
		if route.Path == ingressRoute.Path &&
			route.Prefix == ingressRoute.Prefix &&
			route.Regex == ingressRoute.Regex {

			matchFound := true
			for _, h := range route.Headers {
				if h.Name == model.HeaderAuthority {
					if len(h.Value) > 0 && h.Value != ingressHost {
						// The rule's authority field does not match with ingressHost
						matchFound = false
					}
					break
				}
			}

			// if the rule did not have any Authority header based match, its valid as well.
			if matchFound {
				// We don't have to add a header match for authority since we are already
				// setting the virtual host domain field.

				// enable mixer check on the route
				if mesh.MixerAddress != "" {
					route.OpaqueConfig = buildMixerOpaqueConfig(!mesh.DisablePolicyChecks, true)
				}
				out = append(out, route)
			}
		}
	}

	return out, tls, nil
}

// extractPort extracts the destination service port from the given destination,
func extractPort(svc *model.Service, ingress *proxyconfig.IngressRule) (*model.Port, error) {
	switch p := ingress.GetDestinationServicePort().(type) {
	case *proxyconfig.IngressRule_DestinationPort:
		num := p.DestinationPort
		port, exists := svc.Ports.GetByPort(int(num))
		if !exists {
			return nil, fmt.Errorf("cannot find port %d in %q", num, svc.Hostname)
		}
		return port, nil
	case *proxyconfig.IngressRule_DestinationPortName:
		name := p.DestinationPortName
		port, exists := svc.Ports.Get(name)
		if !exists {
			return nil, fmt.Errorf("cannot find port %q in %q", name, svc.Hostname)
		}
		return port, nil
	}
	return nil, errors.New("unrecognized destination port")
}

func getAuthorityFromIngressRule(rule model.Config) string {
	ingressRule := rule.Spec.(*proxyconfig.IngressRule)
	ingressHost := "*"
	if ingressRule.Match != nil && ingressRule.Match.Request != nil {
		if authority, ok := ingress.Match.Request.Headers[model.HeaderAuthority]; ok {
			switch match := authority.GetMatchType().(type) {
			case *proxyconfig.StringMatch_Exact:
				ingressHost = match.Exact
			default:
				glog.Warningf("Unsupported match type for authority condition %T, falling back to %q", match, host)
			}
		}
	}
	return ingressHost
}
