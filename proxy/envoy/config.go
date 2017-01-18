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
	"io"
	"os"
	"sort"

	"istio.io/manager/model"

	"fmt"
	"io/ioutil"
	"math/rand"
	"path/filepath"
)

func (conf *Config) WriteFile(fname string) error {
	file, err := os.Create(fname)
	if err != nil {
		return err
	}

	if err := conf.Write(file); err != nil {
		file.Close()
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
	if err != nil {
		return err
	}

	return err
}

const (
	EgressClusterPrefix  = "egress_"
	IngressClusterPrefix = "ingress_"
	ServerConfig         = "server_config.pb.txt"
	Stdout               = "/dev/stdout"
)

// Generate Envoy configuration for service instances co-located with Envoy and all services in the mesh
func Generate(instances []*model.ServiceInstance, services []*model.Service, mesh *MeshConfig) (*Config, error) {
	listeners, clusters := buildListeners(instances, services)
	listeners = append(listeners, Listener{
		Port:           mesh.ProxyPort,
		BindToPort:     true,
		UseOriginalDst: true,
		Filters:        make([]NetworkFilter, 0),
	})

	// TODO: egress routing rules
	/*
		if err := buildFS(); err != nil {
			return &Config{}, err
		}
	*/

	return &Config{
		/*
			RootRuntime: RootRuntime{
				SymlinkRoot:  RuntimePath,
				Subdirectory: "traffic_shift",
			},
		*/
		Listeners: listeners,
		Admin: Admin{
			AccessLogPath: Stdout,
			Port:          mesh.AdminPort,
		},
		ClusterManager: ClusterManager{
			Clusters: clusters,
			SDS: SDS{
				Cluster: Cluster{
					Name:             "sds",
					Type:             "strict_dns",
					ConnectTimeoutMs: 1000,
					LbType:           LbTypeRoundRobin,
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

func buildListeners(instances []*model.ServiceInstance, services []*model.Service) ([]Listener, []Cluster) {
	clusters := buildClusters(services)
	listeners := make([]Listener, 0)

	// group by port values
	type listener struct {
		instances map[model.Protocol]bool
		// services map service, port for a given port, protocol pair
		services map[model.Protocol][]*model.Service
	}

	ports := make(map[int]*listener, 0)
	ensure := func(port model.Port) {
		if _, ok := ports[port.Port]; !ok {
			ports[port.Port] = &listener{
				instances: make(map[model.Protocol]bool),
				services:  make(map[model.Protocol][]*model.Service),
			}
		}
	}

	for _, instance := range instances {
		port := instance.Endpoint.Port
		ensure(port)
		ports[port.Port].instances[port.Protocol] = true
	}

	for _, svc := range services {
		for _, port := range svc.Ports {
			ensure(port)
			ports[port.Port].services[port.Protocol] = append(ports[port.Port].services[port.Protocol], &model.Service{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Ports:     []model.Port{port},
			})
		}
	}

	for port, lst := range ports {
		listener := Listener{
			Port:       port,
			BindToPort: false,
		}

		ingressClusterName := fmt.Sprintf("%s%d", IngressClusterPrefix, port)
		if len(lst.instances) > 0 {
			clusters = append(clusters, Cluster{
				Name:             ingressClusterName,
				Type:             "static",
				ConnectTimeoutMs: 1000,
				LbType:           LbTypeRoundRobin,
				Hosts:            []Host{{URL: fmt.Sprintf("tcp://%s:%d", "127.0.0.1", port)}},
			})
		}

		// These two filters operate at different levels of the network stack.
		// In practice, no port has two protocols used in the same set of instances, but we
		// should be careful with not stepping on our feet.
		// The order of the filter insertion is important.
		if lst.instances[model.ProtocolTCP] {
			listener.Filters = append(listener.Filters, NetworkFilter{
				Type: "read",
				Name: "tcp_proxy",
				Config: NetworkFilterConfig{
					Cluster:    ingressClusterName,
					StatPrefix: "ingress_tcp",
				},
			})
		}

		// HTTP-level filter is shared between ingress and egress traffic.
		// The routing decision is based on the virtual host.
		hosts := make([]VirtualHost, 0)
		sort.Sort(HostsByName(hosts))
		for _, proto := range []model.Protocol{model.ProtocolHTTP, model.ProtocolHTTP2, model.ProtocolGRPC} {
			for _, svc := range lst.services[proto] {
				// TODO: support all variants for services: name.<my namespace>, name.namespace.svc.cluster.local
				hosts = append(hosts, VirtualHost{
					Name:    svc.String(),
					Domains: []string{svc.Name, svc.Name + "." + svc.Namespace},
					Routes:  []Route{{Cluster: EgressClusterPrefix + svc.String()}},
				})
			}
		}

		// The order of the hosts is important: host * is a fallback
		if lst.instances[model.ProtocolHTTP] || lst.instances[model.ProtocolHTTP2] || lst.instances[model.ProtocolGRPC] {
			hosts = append(hosts, VirtualHost{
				Name:    "ingress_http",
				Domains: []string{"*"},
				Routes:  []Route{{Cluster: ingressClusterName}},
			})
		}

		// TODO: HTTPS protocol ingress and egress configuration

		if len(hosts) > 0 {
			listener.Filters = append(listener.Filters, NetworkFilter{
				Type: "read",
				Name: "http_connection_manager",
				Config: NetworkFilterConfig{
					CodecType:   "auto",
					StatPrefix:  "http",
					AccessLog:   []AccessLog{{Path: Stdout}},
					RouteConfig: RouteConfig{VirtualHosts: hosts},
					Filters:     buildFilters(),
				},
			})
		}

		if len(listener.Filters) > 0 {
			listeners = append(listeners, listener)
		}
	}

	sort.Sort(ListenersByPort(listeners))
	sort.Sort(ClustersByName(clusters))
	return listeners, clusters
}

// buildClusters creates a cluster for every (service, port)
func buildClusters(services []*model.Service) []Cluster {
	clusters := make([]Cluster, 0)
	for _, svc := range services {
		for _, port := range svc.Ports {
			clusterSvc := model.Service{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Ports:     []model.Port{port},
			}
			cluster := Cluster{
				Name:             EgressClusterPrefix + clusterSvc.String(),
				ServiceName:      clusterSvc.String(),
				Type:             "sds",
				LbType:           LbTypeRoundRobin,
				ConnectTimeoutMs: 1000,
			}
			if port.Protocol == model.ProtocolGRPC ||
				port.Protocol == model.ProtocolHTTP2 {
				cluster.Features = "http2"
			}
			clusters = append(clusters, cluster)
		}
	}
	sort.Sort(ClustersByName(clusters))
	return clusters
}

func buildFilters() []Filter {
	var filters []Filter

	filters = append(filters, Filter{
		Type: "both",
		Name: "esp",
		Config: FilterEndpointsConfig{
			ServiceConfig: EnvoyConfigPath + "generic_service_config.json",
			ServerConfig:  EnvoyConfigPath + ServerConfig,
		},
	})

	filters = append(filters, Filter{
		Type:   "decoder",
		Name:   "router",
		Config: FilterRouterConfig{},
	})

	return filters
}

const (
	RuntimePath         = "/etc/envoy/runtime/routing"
	RuntimeVersionsPath = "/etc/envoy/routing_versions"

	ConfigDirPerm  = 0775
	ConfigFilePerm = 0664
)

// FIXME: doesn't check for name conflicts
// TODO: could be improved by using the full possible set of filenames.
func randFilename(prefix string) string {
	data := make([]byte, 16)
	for i := range data {
		data[i] = '0' + byte(rand.Intn(10))
	}

	return fmt.Sprintf("%s%s", prefix, data)
}

func buildFS() error {
	type weightSpec struct {
		Service string
		Cluster string
		Weight  int
	}

	var weights []weightSpec

	if err := os.MkdirAll(filepath.Dir(RuntimePath), ConfigDirPerm); err != nil { // FIXME: hack
		return err
	}

	if err := os.MkdirAll(RuntimeVersionsPath, ConfigDirPerm); err != nil {
		return err
	}

	dirName, err := ioutil.TempDir(RuntimeVersionsPath, "")
	if err != nil {
		return err
	}

	success := false
	defer func() {
		if !success {
			os.RemoveAll(dirName)
		}
	}()

	for _, weight := range weights {
		if err := os.MkdirAll(filepath.Join(dirName, "/traffic_shift/", weight.Service), ConfigDirPerm); err != nil {
			return err
		} // FIXME: filemode?

		filename := filepath.Join(dirName, "/traffic_shift/", weight.Service, weight.Cluster)
		data := []byte(fmt.Sprintf("%v", weight.Weight))
		if err := ioutil.WriteFile(filename, data, ConfigFilePerm); err != nil {
			return err
		}
	}

	oldRuntime, err := os.Readlink(RuntimePath)
	if err != nil && !os.IsNotExist(err) { // Ignore error from symlink not existing.
		return err
	}

	tmpName := randFilename("./")

	if err := os.Symlink(dirName, tmpName); err != nil {
		return err
	}

	// Atomically replace the runtime symlink
	if err := os.Rename(tmpName, RuntimePath); err != nil {
		return err
	}

	success = true

	// Clean up the old config FS if necessary
	// TODO: make this safer
	if oldRuntime != "" {
		oldRuntimeDir := filepath.Dir(oldRuntime)
		if filepath.Clean(oldRuntimeDir) == filepath.Clean(RuntimeVersionsPath) {
			toDelete := filepath.Join(RuntimeVersionsPath, filepath.Base(oldRuntime))
			if err := os.RemoveAll(toDelete); err != nil {
				return err
			}
		}
	}

	return nil
}
