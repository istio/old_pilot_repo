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

package ingress

import (
	"testing"

	"istio.io/pilot/adapter/config/ingress"
	"istio.io/pilot/platform/kube"
	"istio.io/pilot/proxy"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func TestDecodeIngressRuleName(t *testing.T) {
	cases := []struct {
		ingressName      string
		ingressNamespace string
		ruleNum          int
		pathNum          int
	}{
		{"myingress", "test", 0, 0},
		{"myingress", "default", 1, 2},
		{"my-ingress", "test-namespace", 1, 2},
		{"my-cool-ingress", "new-space", 1, 2},
	}

	for _, c := range cases {
		encoded := encodeIngressRuleName(c.ingressName, c.ingressNamespace, c.ruleNum, c.pathNum)
		ingressName, ingressNamespace, ruleNum, pathNum, err := decodeIngressRuleName(encoded)
		if err != nil {
			t.Errorf("decodeIngressRuleName(%q) => error %v", encoded, err)
		}
		if ingressName != c.ingressName || ingressNamespace != c.ingressNamespace ||
			ruleNum != c.ruleNum || pathNum != c.pathNum {
			t.Errorf("decodeIngressRuleName(%q) => (%q, %q, %d, %d), want (%q, %q, %d, %d)",
				encoded,
				ingressName, ingressNamespace, ruleNum, pathNum,
				c.ingressName, c.ingressNamespace, c.ruleNum, c.pathNum,
			)
		}
	}
}

func TestIsRegularExpression(t *testing.T) {
	cases := []struct {
		s       string
		isRegex bool
	}{
		{"/api/v1/", false},
		{"/api/v1/.*", true},
		{"/api/.*/resource", true},
		{"/api/v[1-9]/resource", true},
		{"/api/.*/.*", true},
	}

	for _, c := range cases {
		if isRegularExpression(c.s) != c.isRegex {
			t.Errorf("isRegularExpression(%q) => %v, want %v", c.s, !c.isRegex, c.isRegex)
		}
	}
}

func TestIngressClass(t *testing.T) {
	ns, cl, cleanup := makeTempClient(t)
	defer cleanup()

	cases := []struct {
		ingressMode   proxyconfig.ProxyMeshConfig_IngressControllerMode
		ingressClass  string
		shouldProcess bool
	}{
		{ingressMode: proxyconfig.ProxyMeshConfig_DEFAULT, ingressClass: "nginx", shouldProcess: false},
		{ingressMode: proxyconfig.ProxyMeshConfig_STRICT, ingressClass: "nginx", shouldProcess: false},
		{ingressMode: proxyconfig.ProxyMeshConfig_DEFAULT, ingressClass: "istio", shouldProcess: true},
		{ingressMode: proxyconfig.ProxyMeshConfig_STRICT, ingressClass: "istio", shouldProcess: true},
		{ingressMode: proxyconfig.ProxyMeshConfig_DEFAULT, ingressClass: "", shouldProcess: true},
		{ingressMode: proxyconfig.ProxyMeshConfig_STRICT, ingressClass: "", shouldProcess: false},
	}

	for _, c := range cases {
		ing := v1beta1.Ingress{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:        "test-ingress",
				Namespace:   "default",
				Annotations: make(map[string]string),
			},
			Spec: v1beta1.IngressSpec{
				Backend: &v1beta1.IngressBackend{
					ServiceName: "default-http-backend",
					ServicePort: intstr.FromInt(80),
				},
			},
		}

		mesh := proxy.DefaultMeshConfig()
		mesh.IngressControllerMode = c.ingressMode
		ctl := ingress.NewController(cl, &mesh, kube.ControllerOptions{
			Namespace:    ns,
			ResyncPeriod: resync,
		})

		if c.ingressClass != "" {
			ing.Annotations["kubernetes.io/ingress.class"] = c.ingressClass
		}

		if c.shouldProcess != ctl.shouldProcessIngress(&ing) {
			t.Errorf("shouldProcessIngress(<ingress of class '%s'>) => %v, want %v",
				c.ingressClass, !c.shouldProcess, c.shouldProcess)
		}
	}
}
