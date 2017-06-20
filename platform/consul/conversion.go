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
		if len(vals) > 1 {
			out[vals[0]] = vals[1]
		} else {
			// TODO safe to assume all tags are of form "key=value"?
			out[tag] = ""
		}
	}
	return out
}

func convertPort(port int, name string) *model.Port {

	// TODO default HTTP/TCP?
	defaultName := "tcp"
	if name == "" {
		name = defaultName
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

		// TODO where should the loadbalancer IP be stored in consul?
		// - Address: IP of consul node for service
		// - ServiceAddress: IP of service
		// - TaggedAddresses: list of explicit LAN and WAN IP addresses for the agent
		// - NodeMetadata
		addr = endpoint.Address
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

	return &model.ServiceInstance{
		Endpoint: model.NetworkEndpoint{
			Address:     inst.Address,
			Port:        inst.ServicePort,
			ServicePort: port,
		},
		Service: &model.Service{
			Hostname: serviceHostname(inst.ServiceName, "TODO", inst.Datacenter),
			Address:  inst.Address,
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
	return fmt.Sprintf("%s.service.%s.consul", name, datacenter)
}

// parseHostname extracts service name and namespace from the service hostnamei
func parseHostname(hostname string) (name, datacenter string, err error) {
	parts := strings.Split(hostname, ".")
	if len(parts) < 3 {
		err = fmt.Errorf("missing service name and datacenter from the service hostname %q", hostname)
		return
	}
	name = parts[0]
	datacenter = parts[2]
	return
}

func convertProtocol(name string) model.Protocol {
	// TODO default TCP or HTTP?
	out := model.ProtocolTCP

	switch name {
	case "udp":
		out = model.ProtocolUDP
	case "grpc":
		out = model.ProtocolGRPC
	case "http":
		out = model.ProtocolHTTP
	case "http2":
		out = model.ProtocolHTTP2
	case "https":
		out = model.ProtocolHTTPS
	}

	return out
}
