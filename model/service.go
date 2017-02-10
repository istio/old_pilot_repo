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

// This file describes the abstract model of services (and their instances)
// as represented in Istio. This model is independent of the underlying
// platform (Kubernetes, Mesos, etc.). Platform specific adapters found
// under platform/* populate the model object with various fields, from the
// metadata found in the platform.  The platform independent proxy code
// under proxy/* uses the representation in the model to generate the
// configuration files for the Layer 7 proxy sidecar. The proxy code is
// specific to individual proxy implementations

// Glossary & concepts
//
// Service is a unit of an application with a unique name that other
// services use to refer to the functionality being called. Service
// instances are pods/VMs/containers that implement the service.
//
// There are multiple versions of a service - In a continuous deployment
// scenario, for a given service, there can be multiple sets of instances
// running potentially different variants of the application binary. These
// variants are not necessarily different API versions. They could be
// iterative changes to the same service, deployed in different
// environments (prod, staging, dev, etc.). Common scenarios where this
// occurs include A/B testing, canary rollouts, etc.
//
// 1. Each service has a fully qualified domain name (FQDN) and one or more
// ports where the service is listening for connections. *Optionally*, a
// service can have a single load balancer/virtual IP address associated
// with it, such that the DNS queries for the FQDN resolves to the virtual
// IP address (a load balancer IP).
//
// E.g., in kubernetes, a service foo is associated with
// foo.default.svc.cluster.local hostname, has a virtual IP of 10.0.1.1 and
// listens on ports 80, 8080

// 2. Instances: Each service has one or more instances, i.e., actual
// manifestations of the service.  Instances represent entities such as
// containers, pods (kubernetes/mesos), VMs, etc.  For example, imagine
// provisioning a nodeJs backend service called catalog with hostname
// (catalogservice.mystore.com), running on port 8080, with 10 VMs hosting
// the service.

// Note that in the example above, the VMs in the backend do not
// necessarily have to expose the service on the same port. Depending on
// the networking setup, the instances could be NAT-ed (e.g., mesos) or be
// running on an overlay network.  They could be hosting the service on any
// random port as long as the load balancer knows how to forward the
// connection to the right port.

// For e.g., A call to http://catalogservice.mystore.com:8080/getCatalog
// would resolve to a load balancer IP 10.0.0.1:8080, the load balancer
// would forward the connection to one of the 10 backend VMs, e.g.,
// 172.16.0.1:55446 or 172.16.0.2:22425, 172.16.0.3:35345, etc.

// Network Endpoint: The network IP address and port associated with each
// instance (e.g., 172.16.0.1:55446 in the above example) is called the
// NetworkEndpoint. Calls to the service's load balancer (virtual) IP
// 10.0.0.1:8080 or the hostname catalog.mystore.com:8080 will end up being
// routed to one of the actual network endpoints.

// Services do not necessarily have to have a load balancer IP. They can
// have a simple DNS SRV based system, such that the DNS srv call to
// resolve catalog.mystore.com:8080 resolves to all 10 backend IPs
// (172.16.0.1:55446, 172.16.0.2:22425,...).
//

// 3. Each version of a service can be differentiated by a unique set of
// tags associated with the version. Tags are simple key value pairs
// assigned to the instances of a particular service version, i.e., all
// instances of same version must have same tag. For example, lets say
// catalog.mystore.com has 2 versions v1 and v2.

// Lets say v1 has tags gitCommit=aeiou234, region=us-east and v2 has tags
// name=kittyCat,region=us-east. And lets say instances 172.16.0.1
// .. 171.16.0.5 run version v1 of the service.

// These instances should register themselves with a service registry,
// using the tags gitCommit=aeiou234, region=us-east, while instances
// 172.16.0.6 .. 172.16.0.10 should register themselves with the service
// registry using the tags name=kittyCat,region=us-east

// Istio expects that the underlying platform to provide a service registry
// and service discovery mechanism. Most container platforms come built in
// with a service registry (e.g., kubernetes, mesos) where a pod
// specification can contain all the version related tags. Upon launching
// the pod, the platform automatically registers the pod with the registry
// along with the tags.  In other platforms, a dedicated service
// registration agent might be needed to automatically register the service
// with a service registration/discovery solution like Consul, etc.

// At the moment, Istio integrates readily with Kubernetes service registry
// and automatically discovers various services, their pods etc., and
// groups the pods into unique sets -- each set representing a service
// version. In future, Istio will add support for pulling in similar
// information from Mesos registry and *potentially* other registries.

// 4. When listing the various instances of a service, the tags partition
// the set of instances into disjoint subsets.  E.g., grouping pods by tags
// "gitCommit=aeiou234,region=us-east", will give all instances of v1 of
// service catalog.mystore.com

// In the absence of a multiple versions, each service has a service has a
// default version that consists of all its instances. For e.g., if pods
// under catalog.mystore.com did not have any tags associated with them,
// Istio would consider catalog.mystore.com as a service with just one
// default version, consisting of 10 VMs with IPs 172.16.0.1 .. 172.16.0.10

// 5. Applications have no knowledge of different versions of the
// service. They can continue to access the services using the hostname/IP
// address of the service, while Istio will take care of routing the
// connection/request to the appropriate version based on the routing rules
// set up by the admin. This model enables the enables the application code
// to decouple itself from the evolution of its dependent services, while
// providing other benefits as well (see mixer).

// Note that Istio does not provide a DNS. Applications can try to resolve
// the FQDN using the DNS service present in the underlying platform
// (kube-dns, mesos-dns, etc.).  In certain platforms such as kubnernetes,
// the DNS name resolves to the service's load balancer (virtual) IP
// address, while in other platforms, the DNS names might resolve to one or
// more instance IP addresses (e.g., in mesos-dns, via DNS srv). Neither
// scenario has no bearing on the application code.
//
// The Istio proxy sidecar intercepts and forwards all requests/responses
// between the application and the service.  The actual choice of the
// service version is determined dynamically by the proxy sidecar process
// (e.g., Envoy, nginx) based on the routing rules set forth by the
// administrator. There are layer7 (http) and layer4 routing rules (see the
// proxy config proto definition for more details).

// Routing rules allow the proxy to select a version based on criterion
// such as (headers, url, etc.), tags associated with source/destination
// and/or by weights assigned to each version.

package model

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)


// Service describes an Istio service (e.g., catalog.mystore.com:8080)
type Service struct {
	// Hostname of the service, e.g. "catalog.mystore.com"
	// TODO need a FQDN check in rules validation
	// Hostname uniquely identifies a service and is used as a reference
	// identifier.  The hostname depends very much on the underlying
	// platform. For example in Kubernetes, for a service called myservice
	// running in namespace foobar, its hostname might be of the form
	// myservice.foobar.svc.cluster.local
	Hostname string `json:"hostname"`

	// Address specifies the service IPv4 address of the load balancer (a
	// virtual IP), if available through the underlying platform. For e.g.,
	// in Kubernetes, this translates into the Service's cluster wide
	// Virtual IP.
	Address string `json:"address,omitempty"`

	// Ports is the set of network ports where the service is listening for
	// connections.  Depending on the platform, the port can be annotated
	// with additional information such as the type of protocol used by the
	// port (e.g., in Kubernetes, one can use the port.protocol field to
	// specify whether the service is listening on TCP/UDP port, and the
	// port.name field to indicate whether the service is listening on Http
	// port or not.

	// FIXME are we obtaining port types like HTTP2, GRPC, etc., from port
	// names? If so, are we expecting users to follow naming conventions ?
	Ports PortList `json:"ports,omitempty"`
}

// Port represents a network port
type Port struct {
	// Name ascribes a human readable name for the port object. When a
	// service has multiple ports, the name field is mandatory (FIXME
	// really? the scenario we are worried about is a kubernetes loophole,
	// where a looney user can define two ports for a service such that
	// both ports have the same port number but a different name This is a
	// corner case and more like an abuse of kubernetes.

	// FIXME detect this stuff in validation and eliminate such duplicates
	Name string `json:"name,omitempty"`

	// Port number where the service can be reached Does not necessarily
	// map to the corresponding port numbers for the instances behind the
	// service. See endpoint definition below.
	Port int `json:"port"`

	// Protocol to be used for the port. How the protocol associated with a
	// port is identified depends heavily on the underlying platform. For
	// e.g., in Kubernetes, the port definition allows the user to define a
	// name for the port where the name can indicate various higher level
	// protocols like Http, Http2, gRPC, etc.
	Protocol Protocol `json:"protocol,omitempty"`
}

// PortList is a set of ports
type PortList []*Port

// Protocol defines network protocols for ports
type Protocol string

// Protocols used by the services
const (
	ProtocolGRPC  Protocol = "GRPC"
	ProtocolHTTPS Protocol = "HTTPS"
	ProtocolHTTP2 Protocol = "HTTP2"
	ProtocolHTTP  Protocol = "HTTP"
	ProtocolTCP   Protocol = "TCP"
	ProtocolUDP   Protocol = "UDP"
)

// Endpoint defines a IP:port associated with an instance of the service. This is the address to which the
// request/connection from the caller will be routed to. If a service has multiple ports, then the same
// instance IP is listening on multiple ports (one corresponding to each service port). However,
// we do not group such ports here. Instead we use a one endpoint per service port.

// For e.g., if catalog.mystore.com is accessible through port 80 and 8080,
// and it maps to an instance with IP 172.16.0.1, such that connections to
// port 80 are forwarded to port 55446, and connections to port 8080 are
// forwarded to port 33333,

// then internally, we have two two endpoint structs for the
// service catalog.mystore.com
//  --> 172.16.0.1:54546 (with ServicePort pointing to 80) and
//  --> 172.16.0.1:33333 (with ServicePort pointing to 8080)
type Endpoint struct {
	// Address of the network endpoint, typically an IPv4 address
	Address string `json:"ip_address,omitempty"`

	// Port number where this instance is listening for connections This
	// need not be the same as the port where the service is accessed.
	// e.g., catalog.mystore.com:8080 -> 172.16.0.1:55446
	Port int `json:"port"`

	// Port declaration from the service declaration This is the port for
	// the service associated with this instance (e.g.,
	// catalog.mystore.com)
	ServicePort *Port `json:"port"`
}

// Tag is non empty set of key=value pair attributes (e.g., Labels in
// kubernetes/mesos) that are assigned to pods/VMs etc. Each version of a
// service can be uniquely identified by a set of tags
type Tag map[string]string

// TagList is a set of tags

// FIXME rename to something else for clarity. Its not clear why one needs
// TagList when Tag by itself is a set of key=value pairs
type TagList []Tag

// ServiceInstance represents an individual instance of a specific version
// of a service. It binds a network endpoint (ip:port), the service
// description (which is oblivious to various versions) and a set of tags
// that describe the service version associated with this instance.

// So, if catalog.mystore.com has two versions v1, v2, with instance IP
// addresses 172.16.0.1:8888, 172.16.0.2:8888 of version v1 (tags foo=bar),

// and instance IP addresses 172.16.0.3:8888, 172.16.0.4:8888 of version v2
// (with tags kitty=kat),

// the set of service instances for catalog.mystore.com are
// catalog.myservice.com ->
//      --> Endpoint(172.16.0.1:8888), Service(catalog.myservice.com), Tag(foo=bar)
//      --> Endpoint(172.16.0.2:8888), Service(catalog.myservice.com), Tag(foo=bar)
//      --> Endpoint(172.16.0.3:8888), Service(catalog.myservice.com), Tag(kitty=cat)
//      --> Endpoint(172.16.0.4:8888), Service(catalog.myservice.com), Tag(kitty=cat)
type ServiceInstance struct {
	Endpoint Endpoint `json:"endpoint,omitempty"`
	Service  *Service `json:"service,omitempty"`
	Tag      Tag      `json:"tag,omitempty"`
}

// ServiceDiscovery enumerates Istio service instances These interfaces
// must be supported by underlying platform implementations in platform/*
// The proxy config generator in proxy/* uses the model interface to query
// the service discovery in the platform in a platform agnostic way and
// configure the proxy.
type ServiceDiscovery interface {
	// Services list declarations of all services in the system
	Services() []*Service

	// GetService retrieves a service by host name if it exists
	GetService(hostname string) (*Service, bool)

	// Instances retrieves instances for a service and its ports that match
	// any of the supplied tags. All instances match an empty tag list.

	// So, if catalog.mystore.com:80 has two versions v1, v2, with instance
	// IP addresses 172.16.0.1:8888, 172.16.0.2:8888 of version v1 (tags
	// foo=bar),

	// and instance IP addresses 172.16.0.3:8888, 172.16.0.4:8888 of
	// version v2 (with tags kitty=kat),

	// the set of service instances for catalog.mystore.com are
	// Instances(catalog.myservice.com, 80) ->
	//      --> Endpoint(172.16.0.1:8888), Service(catalog.myservice.com), Tag(foo=bar)
	//      --> Endpoint(172.16.0.2:8888), Service(catalog.myservice.com), Tag(foo=bar)
	//      --> Endpoint(172.16.0.3:8888), Service(catalog.myservice.com), Tag(kitty=cat)
	//      --> Endpoint(172.16.0.4:8888), Service(catalog.myservice.com), Tag(kitty=cat)

	// OTOH, calling with specific tags returns a trimmed list.
	// e.g., Instances(catalog.myservice.com, 80, foo=bar) ->
	//      --> Endpoint(172.16.0.1:8888), Service(catalog.myservice.com), Tag(foo=bar)
	//      --> Endpoint(172.16.0.2:8888), Service(catalog.myservice.com), Tag(foo=bar)

	// Similar concepts apply for calling this function with a specific
	// port, hostname and tags.
	Instances(hostname string, ports []string, tags TagList) []*ServiceInstance

	// HostInstances lists service instances for a given set of IPv4 addresses.
	HostInstances(addrs map[string]bool) []*ServiceInstance
}


// SubsetOf is true if the tag has identical values for the keys
func (tag Tag) SubsetOf(that Tag) bool {
	for k, v := range tag {
		if that[k] != v {
			return false
		}
	}
	return true
}

// HasSubsetOf returns true if the input tag is a super set of one of the
// tags in the list or if the tag list is empty
func (tags TagList) HasSubsetOf(that Tag) bool {
	if len(tags) == 0 {
		return true
	}
	for _, tag := range tags {
		if tag.SubsetOf(that) {
			return true
		}
	}
	return false
}

// GetNames returns port names
func (ports PortList) GetNames() []string {
	names := make([]string, 0)
	for _, port := range ports {
		names = append(names, port.Name)
	}
	return names
}

// Get retrieves a port declaration by name
func (ports PortList) Get(name string) (*Port, bool) {
	for _, port := range ports {
		if port.Name == name {
			return port, true
		}
	}
	return nil, false
}

// Key generates a unique string referencing service instances for a given port and a tag
func (s *Service) Key(port *Port, tag Tag) string {
	// TODO: check port is non nil and membership of port in service
	return ServiceKey(s.Hostname, PortList{port}, TagList{tag})
}

// ServiceKey generates a service key for a collection of ports and tags
func ServiceKey(hostname string, servicePorts PortList, serviceTags TagList) string {
	// example: name.namespace:http:env=prod;env=test,version=my-v1
	var buffer bytes.Buffer
	buffer.WriteString(hostname)
	np := len(servicePorts)
	nt := len(serviceTags)

	if nt == 1 && serviceTags[0] == nil {
		nt = 0
	}

	if np == 0 && nt == 0 {
		return buffer.String()
	} else if np == 1 && nt == 0 && servicePorts[0].Name == "" {
		return buffer.String()
	} else {
		buffer.WriteString(":")
	}

	if np > 0 {
		ports := make([]string, np)
		for i := 0; i < np; i++ {
			ports[i] = servicePorts[i].Name
		}
		sort.Strings(ports)
		for i := 0; i < np; i++ {
			if i > 0 {
				buffer.WriteString(",")
			}
			buffer.WriteString(ports[i])
		}
	}

	if nt > 0 {
		buffer.WriteString(":")
		tags := make([]string, nt)
		for i := 0; i < nt; i++ {
			tags[i] = serviceTags[i].String()
		}
		sort.Strings(tags)
		for i := 0; i < nt; i++ {
			if i > 0 {
				buffer.WriteString(";")
			}
			buffer.WriteString(tags[i])
		}
	}
	return buffer.String()
}

// ParseServiceKey is the inverse of the Service.String() method
func ParseServiceKey(s string) (hostname string, ports PortList, tags TagList) {
	parts := strings.Split(s, ":")
	hostname = parts[0]

	var names []string
	if len(parts) > 1 {
		names = strings.Split(parts[1], ",")
	} else {
		names = []string{""}
	}

	for _, name := range names {
		ports = append(ports, &Port{Name: name})
	}

	if len(parts) > 2 && len(parts[2]) > 0 {
		for _, tag := range strings.Split(parts[2], ";") {
			tags = append(tags, ParseTagString(tag))
		}
	}
	return
}

func (tag Tag) String() string {
	labels := make([]string, 0)
	for k, v := range tag {
		if len(v) > 0 {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		} else {
			labels = append(labels, k)
		}
	}
	sort.Strings(labels)

	var buffer bytes.Buffer
	var first = true
	for _, label := range labels {
		if !first {
			buffer.WriteString(",")
		} else {
			first = false
		}
		buffer.WriteString(label)
	}
	return buffer.String()
}

// ParseTagString extracts a tag from a string
func ParseTagString(s string) Tag {
	tag := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		kv := strings.Split(pair, "=")
		if len(kv) > 1 {
			tag[kv[0]] = kv[1]
		} else {
			tag[kv[0]] = ""
		}
	}
	return tag
}
