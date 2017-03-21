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
	"io/ioutil"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/pmezard/go-difflib/difflib"

	"istio.io/manager/model"
	"istio.io/manager/test/mock"
)

func TestRoutesByPath(t *testing.T) {
	cases := []struct {
		in       []*HTTPRoute
		expected []*HTTPRoute
	}{

		// Case 2: Prefix before path
		{
			in: []*HTTPRoute{
				{Prefix: "/api"},
				{Path: "/api/v1"},
			},
			expected: []*HTTPRoute{
				{Path: "/api/v1"},
				{Prefix: "/api"},
			},
		},

		// Case 3: Longer prefix before shorter prefix
		{
			in: []*HTTPRoute{
				{Prefix: "/api"},
				{Prefix: "/api/v1"},
			},
			expected: []*HTTPRoute{
				{Prefix: "/api/v1"},
				{Prefix: "/api"},
			},
		},
	}

	// Function to determine if two *Route slices
	// are the same (same Routes, same order)
	sameOrder := func(r1, r2 []*HTTPRoute) bool {
		for i, r := range r1 {
			if r.Path != r2[i].Path || r.Prefix != r2[i].Prefix {
				return false
			}
		}
		return true
	}

	for i, c := range cases {
		sort.Sort(RoutesByPath(c.in))
		if !sameOrder(c.in, c.expected) {
			t.Errorf("Invalid sort order for case %d", i)
		}
	}
}

func TestTCPRouteConfigByRoute(t *testing.T) {
	cases := []struct {
		name string
		in   []TCPRoute
		want []TCPRoute
	}{
		{
			name: "sorted by cluster",
			in: []TCPRoute{{
				Cluster:           "cluster-b",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.2/32", "192.168.1.1/32"},
				DestinationPorts:  "5000",
			}},
			want: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.2/32", "192.168.1.1/32"},
				DestinationPorts:  "5000",
			}, {
				Cluster:           "cluster-b",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
			}},
		},
		{
			name: "sorted by DestinationIPList",
			in: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.2.1/32", "192.168.2.2/32"},
				DestinationPorts:  "5000",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
			}},
			want: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.2.1/32", "192.168.2.2/32"},
				DestinationPorts:  "5000",
			}},
		},
		{
			name: "sorted by DestinationPorts",
			in: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5001",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
			}},
			want: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5001",
			}},
		},
		{
			name: "sorted by SourceIPList",
			in: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.3.1/32", "192.168.3.2/32"},
				SourcePorts:       "5002",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.2.1/32", "192.168.2.2/32"},
				SourcePorts:       "5002",
			}},
			want: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.2.1/32", "192.168.2.2/32"},
				SourcePorts:       "5002",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.3.1/32", "192.168.3.2/32"},
				SourcePorts:       "5002",
			}},
		},
		{
			name: "sorted by SourcePorts",
			in: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.2.1/32", "192.168.2.2/32"},
				SourcePorts:       "5003",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.2.1/32", "192.168.2.2/32"},
				SourcePorts:       "5002",
			}},
			want: []TCPRoute{{
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.2.1/32", "192.168.2.2/32"},
				SourcePorts:       "5002",
			}, {
				Cluster:           "cluster-a",
				DestinationIPList: []string{"192.168.1.1/32", "192.168.1.2/32"},
				DestinationPorts:  "5000",
				SourceIPList:      []string{"192.168.2.1/32", "192.168.2.2/32"},
				SourcePorts:       "5003",
			}},
		},
	}

	for _, c := range cases {
		sort.Sort(TCPRouteByRoute(c.in))
		if !reflect.DeepEqual(c.in, c.want) {
			t.Errorf("Invalid sort order for case %q:\n got  %#v\n want %#v", c.name, c.in, c.want)
		}
	}
}

const (
	envoyV0Config         = "testdata/envoy-v0.json"
	envoyV1Config         = "testdata/envoy-v1.json"
	envoyFaultConfig      = "testdata/envoy-fault.json"
	envoySslContextConfig = "testdata/envoy-ssl-context.json"
	cbPolicy              = "testdata/cb-policy.yaml.golden"
	timeoutRouteRule      = "testdata/timeout-route-rule.yaml.golden"
	weightedRouteRule     = "testdata/weighted-route.yaml.golden"
	faultRouteRule        = "testdata/fault-route.yaml.golden"
)

func compareJSON(jsonFile string, t *testing.T) {
	file, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		t.Fatalf(err.Error())
	}
	golden, err := ioutil.ReadFile(jsonFile + ".golden")
	if err != nil {
		t.Fatalf(err.Error())
	}

	data := strings.TrimSpace(string(file))
	expected := strings.TrimSpace(string(golden))

	if data != expected {
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(expected),
			B:        difflib.SplitLines(data),
			FromFile: jsonFile + ".golden",
			ToFile:   jsonFile,
			Context:  2,
		}
		text, _ := difflib.GetUnifiedDiffString(diff)
		fmt.Println(text)
		t.Errorf("Failed validating golden artifact %s.golden", jsonFile)
	}
}

func testConfig(r *model.IstioRegistry, instance, envoyConfig string, t *testing.T) {
	ds := mock.Discovery

	config := Generate(&ProxyContext{
		Discovery:  ds,
		Config:     r,
		MeshConfig: DefaultMeshConfig,
		Addrs:      map[string]bool{instance: true},
	})
	if config == nil {
		t.Fatal("Failed to generate config")
	}

	err := config.WriteFile(envoyConfig)
	if err != nil {
		t.Fatalf(err.Error())
	}

	compareJSON(envoyConfig, t)
}

func testConfigWithSslContext(r *model.IstioRegistry, instance, envoyConfig string, t *testing.T) {
        ds := mock.Discovery
        meshConfigWithSslContext := &MeshConfig{
                DiscoveryAddress: DefaultMeshConfig.DiscoveryAddress,
                MixerAddress:     DefaultMeshConfig.MixerAddress,
                ProxyPort:        DefaultMeshConfig.ProxyPort,
                AdminPort:        DefaultMeshConfig.AdminPort,
                BinaryPath:       DefaultMeshConfig.BinaryPath,
                ConfigPath:       DefaultMeshConfig.ConfigPath,
                EnableAuth:       true,
                AuthConfigPath:   "/etc/envoyauth",
        }
        config := Generate(&ProxyContext{
                Discovery:  ds,
                Config:     r,
                MeshConfig: meshConfigWithSslContext,
                Addrs:      map[string]bool{instance: true},
        })
        if config == nil {
                t.Fatal("Failed to generate config")
        }

        err := config.WriteFile(envoyConfig)
        if err != nil {
                t.Fatalf(err.Error())
        }

        compareJSON(envoyConfig, t)
}

func configObjectFromYAML(kind, file string) (proto.Message, error) {
	schema, ok := model.IstioConfig[kind]
	if !ok {
		return nil, fmt.Errorf("Missing kind %q", kind)
	}
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return schema.FromYAML(string(content))
}

func addCircuitBreaker(r *model.IstioRegistry, t *testing.T) {
	msg, err := configObjectFromYAML(model.DestinationPolicy, cbPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Post(model.Key{
		Kind: model.DestinationPolicy,
		Name: "circuit-breaker"},
		msg); err != nil {
		t.Fatal(err)
	}
}

func addTimeout(r *model.IstioRegistry, t *testing.T) {
	msg, err := configObjectFromYAML(model.RouteRule, timeoutRouteRule)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Post(model.Key{Kind: model.RouteRule, Name: "timeouts"}, msg); err != nil {
		t.Fatal(err)
	}
}

func addWeightedRoute(r *model.IstioRegistry, t *testing.T) {
	msg, err := configObjectFromYAML(model.RouteRule, weightedRouteRule)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Post(model.Key{Kind: model.RouteRule, Name: "weighted-route"}, msg); err != nil {
		t.Fatal(err)
	}
}

func addFaultRoute(r *model.IstioRegistry, t *testing.T) {
	msg, err := configObjectFromYAML(model.RouteRule, faultRouteRule)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Post(model.Key{Kind: model.RouteRule, Name: "fault-route"}, msg); err != nil {
		t.Fatal(err)
	}
}

func TestMockConfig(t *testing.T) {
	r := mock.MakeRegistry()
	testConfig(r, mock.HostInstanceV0, envoyV0Config, t)
	testConfig(r, mock.HostInstanceV1, envoyV1Config, t)
}

func TestMockConfigTimeout(t *testing.T) {
	r := mock.MakeRegistry()
	addTimeout(r, t)
	testConfig(r, mock.HostInstanceV0, envoyV0Config, t)
	testConfig(r, mock.HostInstanceV1, envoyV1Config, t)
}

func TestMockConfigCircuitBreaker(t *testing.T) {
	r := mock.MakeRegistry()
	addCircuitBreaker(r, t)
	testConfig(r, mock.HostInstanceV0, envoyV0Config, t)
	testConfig(r, mock.HostInstanceV1, envoyV1Config, t)
}

func TestMockConfigWeighted(t *testing.T) {
	r := mock.MakeRegistry()
	addWeightedRoute(r, t)
	testConfig(r, mock.HostInstanceV0, envoyV0Config, t)
	testConfig(r, mock.HostInstanceV1, envoyV1Config, t)
}

func TestMockConfigFault(t *testing.T) {
	r := mock.MakeRegistry()
	addFaultRoute(r, t)
	// Fault rule uses source condition, hence the different golden artifacts
	testConfig(r, mock.HostInstanceV0, envoyFaultConfig, t)
	testConfig(r, mock.HostInstanceV1, envoyV1Config, t)
}

func TestMockConfigSslContext(t *testing.T) {
	r := mock.MakeRegistry()
	testConfigWithSslContext(r, mock.HostInstanceV0, envoySslContextConfig, t)
}
