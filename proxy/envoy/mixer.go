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

// Mixer filter configuration

package envoy

import (
	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/proxy"
)

const (
	// MixerCluster is the name of the mixer cluster
	MixerCluster = "mixer_server"

	MixerFilter       = "mixer"
	AttrSourceIP      = "source.ip"
	AttrSourceUID     = "source.uid"
	AttrTargetIP      = "target.ip"
	AttrTargetUID     = "target.uid"
	MixerRequestCount = "RequestCount"
)

// FilterMixerConfig definition
type FilterMixerConfig struct {
	// MixerAttributes specifies the static list of attributes that are sent with
	// each request to Mixer.
	MixerAttributes map[string]string `json:"mixer_attributes,omitempty"`

	// ForwardAttributes specifies the list of attribute keys and values that
	// are forwarded as an HTTP header to the server side proxy
	ForwardAttributes map[string]string `json:"forward_attributes,omitempty"`

	// QuotaName specifies the name of the quota bucket to withdraw tokens from;
	// an empty name means no quota will be charged.
	QuotaName string `json:"quota_name,omitempty"`
}

func buildMixerCluster(mesh *proxyconfig.ProxyMeshConfig) *Cluster {
	mixerCluster := buildCluster(mesh.MixerAddress, MixerCluster, mesh.ConnectTimeout)
	mixerCluster.CircuitBreaker = &CircuitBreaker{
		Default: DefaultCBPriority{
			MaxPendingRequests: 10000,
			MaxRequests:        10000,
		},
	}
	mixerCluster.Features = ClusterFeatureHTTP2
	return mixerCluster
}

func buildMixerInboundOpaqueConfig() map[string]string {
	return map[string]string{
		"mixer_control": "on",
		"mixer_forward": "off",
	}
}

func mixerHTTPRouteConfig(sidecar proxy.Sidecar) *FilterMixerConfig {
	return &FilterMixerConfig{
		MixerAttributes: map[string]string{
			AttrTargetIP:  sidecar.IPAddress,
			AttrTargetUID: "kubernetes://" + sidecar.ID,
		},
		ForwardAttributes: map[string]string{
			AttrTargetIP:  sidecar.IPAddress,
			AttrTargetUID: "kubernetes://" + sidecar.ID,
		},
		QuotaName: MixerRequestCount,
	}
}

func insertMixerFilter(listeners []*Listener, sidecar proxy.Sidecar) {
	config := mixerHTTPRouteConfig(sidecar)
	for _, l := range listeners {
		for _, f := range l.Filters {
			if f.Name == HTTPConnectionManager {
				http := (f.Config).(*HTTPFilterConfig)
				http.Filters = append([]HTTPFilter{{
					Type:   decoder,
					Name:   MixerFilter,
					Config: config,
				}}, http.Filters...)
			}
		}
	}
}
