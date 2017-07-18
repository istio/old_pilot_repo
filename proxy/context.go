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

package proxy

import (
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/model"
)

const (
	// EgressNode is the name for the egress proxy node
	EgressNode = "egress"

	// IngressNode is the name for the ingress proxy node
	IngressNode = "ingress"
)

// Environment provides an aggregate environmental API for Pilot
type Environment struct {
	// Discovery interface for listing services and instances
	model.ServiceDiscovery

	// Accounts interface for listing service accounts
	model.ServiceAccounts

	// Config interface for listing routing rules
	model.IstioConfigStore

	// Mesh is the mesh config (to be merged into the config store)
	Mesh *proxyconfig.ProxyMeshConfig
}

// Role declares the proxy node role in the mesh
type Role interface {
	isProxyRole()
}

// Sidecar defines the sidecar proxy role
type Sidecar struct {
	// IPAddress is the IP address of the proxy used to identify it and its
	// co-located service instances. Example: "10.60.1.6"
	IPAddress string

	// UID is the platform specific unique identifier of the proxy.
	// UID should serve as a key to lookup additional information associated with the
	// proxy. Example: "kubernetes://my-pod.my-namespace"
	UID string
}

func (Sidecar) isProxyRole() {}

// Egress defines the egress proxy role
type Egress struct{}

func (Egress) isProxyRole() {}

// DefaultMeshConfig configuration
func DefaultMeshConfig() proxyconfig.ProxyMeshConfig {
	return proxyconfig.ProxyMeshConfig{
		DiscoveryAddress:   "istio-pilot:8080",
		EgressProxyAddress: "istio-egress:80",

		ProxyListenPort:        15001,
		ProxyAdminPort:         15000,
		DrainDuration:          ptypes.DurationProto(2 * time.Second),
		ParentShutdownDuration: ptypes.DurationProto(3 * time.Second),
		DiscoveryRefreshDelay:  ptypes.DurationProto(1 * time.Second),
		ConnectTimeout:         ptypes.DurationProto(1 * time.Second),
		IstioServiceCluster:    "istio-proxy",

		IngressClass:          "istio",
		IngressControllerMode: proxyconfig.ProxyMeshConfig_STRICT,

		AuthPolicy:    proxyconfig.ProxyMeshConfig_NONE,
		AuthCertsPath: "/etc/certs",
	}
}

// ParsePort extracts port number from a valid proxy address
func ParsePort(addr string) int {
	port, err := strconv.Atoi(addr[strings.Index(addr, ":")+1:])
	if err != nil {
		glog.Warning(err)
	}

	return port
}
