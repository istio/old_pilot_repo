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
	"sort"
	"time"

	"github.com/golang/glog"

	"istio.io/manager/model"
	"istio.io/manager/proxy"
)

type egressWatcher struct {
	agent   proxy.Agent
	ctl     model.Controller
	context *EgressConfig
}

// NewEgressWatcher creates a new egress watcher instance with an agent
func NewEgressWatcher(ctl model.Controller, context *EgressConfig) (Watcher, error) {
	agent := proxy.NewAgent(runEnvoy(context.Mesh, "egress"), cleanupEnvoy(context.Mesh), 10, 100*time.Millisecond)

	out := &egressWatcher{
		agent:   agent,
		ctl:     ctl,
		context: context,
	}

	// egress depends on the external services declaration being up to date
	if err := ctl.AppendServiceHandler(func(*model.Service, model.Event) {
		out.reload()
	}); err != nil {
		return nil, err
	}

	return out, nil
}

func (w *egressWatcher) reload() {
	w.agent.ScheduleConfigUpdate(generateEgress(w.context))
}

func (w *egressWatcher) Run(stop <-chan struct{}) {
	go w.agent.Run(stop)

	// Initialize envoy according to the current model state,
	// instead of waiting for the first event to arrive.
	// Note that this is currently done synchronously (blocking),
	// to avoid racing with controller events lurking around the corner.
	// This can be improved once we switch to a mechanism where reloads
	// are linearized (e.g., by a single goroutine reloader).
	w.reload()
	w.ctl.Run(stop)
}

// EgressConfig defines information for engress
type EgressConfig struct {
	// TODO: cert/key filenames will need to be dynamic for multiple key/cert pairs
//	CertFile  string
//	KeyFile   string
	Namespace string
//	Secret    string
//	Secrets   model.SecretRegistry
	Services  model.ServiceDiscovery
	Mesh      *MeshConfig
}

func generateEgress(conf *EgressConfig) *Config {

	// Create a VirtualHost for each external service
	vhosts := make([]*VirtualHost, 0)
	services := conf.Services.Services()
	for _, service := range services {
		if service.External {
			vhosts = append(vhosts, buildEgressHTTPRoute(service))
		}
	}

	sort.Slice(vhosts, func(i, j int) bool { return vhosts[i].Name < vhosts[j].Name })

	rConfig := &HTTPRouteConfig{VirtualHosts: vhosts}

	listener := &Listener{
		Address:    fmt.Sprintf("tcp://%s:80", WildcardAddress),
		BindToPort: true,
		Filters: []*NetworkFilter{
			{
				Type: "read",
				Name: HTTPConnectionManager,
				Config: HTTPFilterConfig{
					CodecType:   "auto",
					StatPrefix:  "http",
					AccessLog:   []AccessLog{{Path: DefaultAccessLog}},
					RouteConfig: rConfig,
					Filters: []HTTPFilter{
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

	//// configure for HTTPS if provided with a secret name
	//if conf.Secret != "" {
	//	// configure Envoy
	//	listener.Address = fmt.Sprintf("tcp://%s:443", WildcardAddress)
	//	listener.SSLContext = &SSLContext{
	//		CertChainFile:  conf.CertFile,
	//		PrivateKeyFile: conf.KeyFile,
	//	}
	//
	//	if err := writeTLS(conf.CertFile, conf.KeyFile, conf.Namespace,
	//		conf.Secret, conf.Secrets); err != nil {
	//		glog.Warning("Failed to get and save secrets. Envoy will crash and trigger a retry...")
	//	}
	//}

	listeners := []*Listener{listener}
	clusters := rConfig.clusters().normalize()

	return &Config{
		Listeners: listeners,
		Admin: Admin{
			AccessLogPath: DefaultAccessLog,
			Address:       fmt.Sprintf("tcp://%s:%d", WildcardAddress, conf.Mesh.AdminPort),
		},
		ClusterManager: ClusterManager{
			Clusters: clusters,
		},
	}
}

// buildEgressRoute translates an egress rule to an Envoy route
func buildEgressHTTPRoute(svc *model.Service) *VirtualHost {
	var host *VirtualHost

	// TODO Cluster names are not unique if external service definition contains more than one port
	for _, servicePort := range svc.Ports {
		protocol := servicePort.Protocol
		switch protocol {
		case model.ProtocolHTTP, model.ProtocolHTTP2, model.ProtocolGRPC:

			route := &HTTPRoute{
				Prefix: "/",
				Cluster: svc.Address,
				AutoHostRewrite: true,
			}
			cluster := buildOutboundCluster(svc.Hostname, servicePort, nil, nil)

			// configure cluster for strict_dns
			cluster.Name = svc.Address
			cluster.ServiceName = ""
			cluster.Type = "strict_dns"
			cluster.Hosts = []Host{
				{
					URL: fmt.Sprintf("tcp://%s:%d", svc.Address, servicePort.Port),
				},
			}

			route.clusters = append(route.clusters, cluster)

			// TODO do we need to match "myapp.whatever.google.com",
			// "istio-egress.default.svc.cluster.local", and route.HostRewrite value from ingress
			host = &VirtualHost{
				Name:    svc.Address, // TODO myapp.whatever.google.com unique?
				Domains: []string{svc.Hostname, svc.Address},
				Routes:  []*HTTPRoute{route},
			}

		case model.ProtocolTCP, model.ProtocolHTTPS:
		// TODO

		default:
			glog.Warningf("Unsupported outbound protocol %v for port %#v", protocol, servicePort)
		}
	}

	return host
}
