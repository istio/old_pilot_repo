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

package kube

import (
	"fmt"
	"strings"

	"sort"
	"strconv"

	multierror "github.com/hashicorp/go-multierror"
	"istio.io/pilot/model"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// IngressClassAnnotation is the annotation on ingress resources for the class of controllers
	// responsible for it
	IngressClassAnnotation = "kubernetes.io/ingress.class"
)

func convertTags(obj meta_v1.ObjectMeta) model.Tags {
	out := make(model.Tags, len(obj.Labels))
	for k, v := range obj.Labels {
		out[k] = v
	}
	return out
}

func convertPort(port v1.ServicePort) *model.Port {
	return &model.Port{
		Name:     port.Name,
		Port:     int(port.Port),
		Protocol: convertProtocol(port.Name, port.Protocol),
	}
}

func convertService(svc v1.Service, domainSuffix string) *model.Service {
	addr, external := "", ""
	if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != v1.ClusterIPNone {
		addr = svc.Spec.ClusterIP
	}

	if svc.Spec.Type == v1.ServiceTypeExternalName && svc.Spec.ExternalName != "" {
		external = svc.Spec.ExternalName
	}

	// must have address or be external (but not both)
	if (addr == "" && external == "") || (addr != "" && external != "") {
		return nil
	}

	ports := make([]*model.Port, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		ports = append(ports, convertPort(port))
	}

	return &model.Service{
		Hostname:     serviceHostname(svc.Name, svc.Namespace, domainSuffix),
		Ports:        ports,
		Address:      addr,
		ExternalName: external,
	}
}

// serviceHostname produces FQDN for a k8s service
func serviceHostname(name, namespace, domainSuffix string) string {
	return fmt.Sprintf("%s.%s.svc.%s", name, namespace, domainSuffix)
}

// KeyFunc is the internal API key function that returns "namespace"/"name" or
// "name" if "namespace" is empty
func KeyFunc(name, namespace string) string {
	if len(namespace) == 0 {
		return name
	}
	return namespace + "/" + name
}

// parseHostname extracts service name and namespace from the service hostnamei
func parseHostname(hostname string) (name string, namespace string, err error) {
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		err = fmt.Errorf("missing service name and namespace from the service hostname %q", hostname)
		return
	}
	name = parts[0]
	namespace = parts[1]
	return
}

func convertProtocol(name string, proto v1.Protocol) model.Protocol {
	out := model.ProtocolTCP
	switch proto {
	case v1.ProtocolUDP:
		out = model.ProtocolUDP
	case v1.ProtocolTCP:
		prefix := name
		i := strings.Index(name, "-")
		if i >= 0 {
			prefix = name[:i]
		}
		switch prefix {
		case "grpc":
			out = model.ProtocolGRPC
		case "http":
			out = model.ProtocolHTTP
		case "http2":
			out = model.ProtocolHTTP2
		case "https":
			out = model.ProtocolHTTPS
		}
	}
	return out
}

func convertProbePort(c v1.Container, port intstr.IntOrString) (int, error) {
	switch port.Type {
	case intstr.Int:
		return port.IntValue(), nil
	case intstr.String:
		for _, named := range c.Ports {
			if named.Name == port.String() {
				return int(named.ContainerPort), nil
			}
		}
		return 0, fmt.Errorf("missing named port %q", port)
	default:
		return 0, fmt.Errorf("incorrect port type %q", port)
	}
}

// convertProbesToPorts returns a PortList consisting of the ports where the
// pod is configured to do Liveness and Readiness probes
func convertProbesToPorts(t *v1.PodSpec) (model.PortList, error) {
	set := make(map[string]*model.Port)
	var errs error
	for _, container := range t.Containers {
		if container.LivenessProbe != nil {
			if container.LivenessProbe.HTTPGet != nil {
				port, err := convertProbePort(container, container.LivenessProbe.Handler.HTTPGet.Port)
				if err != nil {
					errs = multierror.Append(errs, err)
				} else {
					p := &model.Port{
						Name:     "mgmt-" + strconv.Itoa(port),
						Port:     port,
						Protocol: model.ProtocolHTTP,
					}
					// Deduplicate along the way. We don't differentiate between HTTP vs TCP mgmt ports
					if set[p.Name] == nil {
						set[p.Name] = p
					}
				}
			} else if container.LivenessProbe.TCPSocket != nil {
				// Only one type of handler is allowed by Kubernetes (HTTPGet or TCPSocket)
				port, err := convertProbePort(container, container.LivenessProbe.TCPSocket.Port)
				if err != nil {
					errs = multierror.Append(errs, err)
				} else {
					p := &model.Port{
						Name:     "mgmt-" + strconv.Itoa(port),
						Port:     port,
						Protocol: model.ProtocolTCP,
					}
					// Deduplicate along the way. We don't differentiate between HTTP vs TCP mgmt ports
					if set[p.Name] == nil {
						set[p.Name] = p
					}
				}
			}
		}

		if container.ReadinessProbe != nil {
			if container.ReadinessProbe.HTTPGet != nil {
				port, err := convertProbePort(container, container.ReadinessProbe.HTTPGet.Port)
				if err != nil {
					errs = multierror.Append(errs, err)
				} else {
					p := &model.Port{
						Name:     "mgmt-" + strconv.Itoa(port),
						Port:     port,
						Protocol: model.ProtocolHTTP,
					}
					// Deduplicate along the way. We don't differentiate between HTTP vs TCP mgmt ports
					if set[p.Name] == nil {
						set[p.Name] = p
					}
				}
			} else if container.ReadinessProbe.TCPSocket != nil {
				port, err := convertProbePort(container, container.ReadinessProbe.TCPSocket.Port)
				if err != nil {
					errs = multierror.Append(errs, err)
				} else {
					p := &model.Port{
						Name:     "mgmt-" + strconv.Itoa(port),
						Port:     port,
						Protocol: model.ProtocolTCP,
					}
					// Deduplicate along the way. We don't differentiate between HTTP vs TCP mgmt ports
					if set[p.Name] == nil {
						set[p.Name] = p
					}
				}
			}
		}
	}

	mgmtPorts := make(model.PortList, 0, len(set))
	for _, p := range set {
		mgmtPorts = append(mgmtPorts, p)
	}
	sort.Slice(mgmtPorts, func(i, j int) bool { return mgmtPorts[i].Port < mgmtPorts[j].Port })

	return mgmtPorts, errs
}
