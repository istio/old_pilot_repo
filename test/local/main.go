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

// Package main is used for local testing of Istio proxy mesh
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"istio.io/pilot/model"
	"istio.io/pilot/proxy"
	"istio.io/pilot/proxy/envoy"
	"istio.io/pilot/test/server"
)

const (
	localhost = "127.0.0.1"
	http      = "http"
	grpc      = "grpc"
)

var (
	baseHTTPPort  int
	baseGRPCPort  int
	n             int
	host          string
	discoveryPort int

	service = &model.Service{
		Hostname: host,
		Address:  localhost,
		Ports: []*model.Port{{
			Name:     http,
			Port:     8000,
			Protocol: model.ProtocolHTTP,
		}, {
			Name:     grpc,
			Port:     8001,
			Protocol: model.ProtocolGRPC,
		}},
	}
)

func init() {
	flag.IntVar(&baseHTTPPort, http, 9000, "Base HTTP/1.1 port")
	flag.IntVar(&baseGRPCPort, grpc, 9100, "Base gRPC port")
	flag.IntVar(&n, "n", 10, "Number of instances")
	flag.IntVar(&discoveryPort, "xds", 7000, "xDS server port")
	flag.StringVar(&host, "host", "dev", "Service hostname")
}

func main() {
	// set global settings
	mesh := proxy.DefaultMeshConfig()
	mesh.MixerAddress = ""
	mesh.DiscoveryAddress = fmt.Sprintf("%s:%d", localhost, discoveryPort)

	// spawn service instances
	for i := 0; i < n; i++ {
		go server.RunHTTP(baseHTTPPort+i, fmt.Sprintf("v%d", i))
		go server.RunGRPC(baseGRPCPort+i, fmt.Sprintf("v%d", i), "", "")
	}

	// spawn proxy
	watcher := envoy.NewWatcher(mesh, role, configpath)
	ctx, cancel := context.WithCancel(context.Background())
	go watcher.Run(ctx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
}

type registry struct{}

func (_ registry) Services() []*model.Service {
	return []*model.Service{service}
}

func (_ registry) GetService(hostname string) (*model.Service, bool) {
	if hostname == host {
		return service, true
	}
	return nil, false
}

func (_ registry) Instances(hostname string, ports []string, tagsList model.TagsList) []*model.ServiceInstance {
	if hostname != host {
		return nil
	}

	out := make([]*model.ServiceInstance, 0)

	for _, port := range ports {
		var base int
		if port == http {
			base = baseHTTPPort
		} else if port == grpc {
			base = baseGRPCPort
		} else {
			continue
		}

		for i := 0; i < n; i++ {
			tags := map[string]string{"version": fmt.Sprintf("v%d", i)}
			if !tagsList.HasSubsetOf(tags) {
				continue
			}

			svcPort, _ := service.Ports.Get(port)
			instance := &model.ServiceInstance{
				Endpoint: model.NetworkEndpoint{
					Address:     localhost,
					Port:        base + i,
					ServicePort: svcPort,
				},
				Service: service,
				Tags:    tags,
			}
			out = append(out, instance)
		}
	}

	return out
}

func (_ registry) HostInstances(addrs map[string]bool) []*model.ServiceInstance {
	// TODO: assume no inbound routes
	return nil
}
