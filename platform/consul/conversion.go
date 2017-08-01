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

package consul

import (
	"fmt"
	"strings"

	"github.com/hashicorp/consul/api"
	"istio.io/pilot/model"
)

const (
	protocolTagName = "protocol"
	externalTagName = "external"
)

func convertTags(tags []string) model.Tags {
	out := make(model.Tags, len(tags))
	for _, tag := range tags {
		vals := strings.Split(tag, "=")

		// Tags not of form "key=value" are ignored to avoid possible collisions
		if len(vals) > 1 {
			out[vals[0]] = vals[1]
		}
	}
	return out
}

func convertPort(port int, name string) *model.Port {
	if name == "" {
		name = "http"
	}

	return &model.Port{
		Name:     name,
		Port:     port,
		Protocol: convertProtocol(name),
	}
}

func convertService(endpoints []*api.CatalogService) *model.Service {
	name, addr, external := "", "", ""
	node, datacenter := "", ""

	ports := model.PortList{}
	for _, endpoint := range endpoints {
		name = endpoint.ServiceName
		ports = append(ports, convertPort(endpoint.ServicePort, endpoint.NodeMeta[protocolTagName]))

		// TODO This will not work if service is a mix of external and local services
		// or if a service has more than one external name
		if endpoint.NodeMeta[externalTagName] != "" {
			external = endpoint.NodeMeta[externalTagName]
		}

		// TODO how to handle if a service spans datacenters/nodes?
		node = endpoint.Node
		datacenter = endpoint.Datacenter

	}

	out := &model.Service{
		Hostname:     serviceHostname(name, node, datacenter),
		Ports:        ports,
		Address:      addr,
		ExternalName: external,
	}

	return out
}

func convertInstance(inst *api.CatalogService) *model.ServiceInstance {
	tags := convertTags(inst.ServiceTags)
	port := convertPort(inst.ServicePort, inst.NodeMeta[protocolTagName])

	addr := inst.ServiceAddress
	if addr == "" {
		addr = inst.Address
	}

	return &model.ServiceInstance{
		Endpoint: model.NetworkEndpoint{
			Address:     addr,
			Port:        inst.ServicePort,
			ServicePort: port,
		},

		Service: &model.Service{
			Hostname: serviceHostname(inst.ServiceName, "TODO", inst.Datacenter),
			Address:  inst.ServiceAddress,
			Ports:    model.PortList{port},
			// TODO ExternalName come from metadata?
			ExternalName: inst.NodeMeta[externalTagName],
		},
		Tags: tags,
	}
}

// serviceHostname produces FQDN for a consul service
func serviceHostname(name, node, datacenter string) string {
	// TODO include consul node in Hostname?
	// consul DNS uses "redis.service.us-east-1.consul" -> "[<optional_tag>].<svc>.service.[<optional_datacenter>].consul"
	return fmt.Sprintf("%s.service.consul", name)
}

// parseHostname extracts service name from the service hostname
func parseHostname(hostname string) (name string, err error) {
	parts := strings.Split(hostname, ".")
	if len(parts) < 1 {
		err = fmt.Errorf("missing service name from the service hostname %q", hostname)
		return
	}
	name = parts[0]
	return
}

func convertProtocol(name string) model.Protocol {
	switch name {
	case "tcp":
		return model.ProtocolTCP
	case "udp":
		return model.ProtocolUDP
	case "grpc":
		return model.ProtocolGRPC
	case "http":
		return model.ProtocolHTTP
	case "http2":
		return model.ProtocolHTTP2
	case "https":
		return model.ProtocolHTTPS
	default:
		return model.ProtocolHTTP
	}

}
