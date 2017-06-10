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

package tpr

import (
	"os"
	"os/user"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/model"
	"istio.io/pilot/platform/kube"
	"istio.io/pilot/proxy"
	"istio.io/pilot/test/mock"
	"istio.io/pilot/test/util"
)

func TestThirdPartyResourcesClient(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)

	cl.namespace = ns
	mock.CheckMapInvariant(cl, t, 5)

	// TODO(kuat) initial watch always fails, takes time to register TPR, keep
	// around as a work-around
	// kr.DeregisterResources()
}

func makeClient(t *testing.T) *Client {
	usr, err := user.Current()
	if err != nil {
		t.Fatal(err.Error())
	}

	kubeconfig := usr.HomeDir + "/.kube/config"

	// For Bazel sandbox we search a different location:
	if _, err = os.Stat(kubeconfig); err != nil {
		kubeconfig, _ = os.Getwd()
		kubeconfig = kubeconfig + "/../../../platform/kube/config"
	}

	desc := append(model.IstioConfigTypes, mock.Types...)
	cl, err := NewClient(kubeconfig, desc, "istio-test")
	if err != nil {
		t.Fatal(err)
	}

	err = cl.RegisterResources()
	if err != nil {
		t.Fatal(err)
	}

	return cl
}

func TestIngressController(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)
	cl.namespace = ns

	mesh := proxy.DefaultMeshConfig()
	ctl := NewController(cl, &mesh, kube.ControllerOptions{
		Namespace:    ns,
		ResyncPeriod: resync,
	})

	stop := make(chan struct{})
	defer close(stop)
	go ctl.Run(stop)

	// Append an ingress notification handler that just counts number of notifications
	notificationCount := 0
	ctl.RegisterEventHandler(model.IngressRule, func(model.Config, model.Event) {
		notificationCount++
	})

	// Create an ingress resource of a different class,
	// So that we can later verify it doesn't generate a notification,
	// nor returned with List(), Get() etc.
	nginxIngress := v1beta1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "nginx-ingress",
			Namespace: ns,
			Annotations: map[string]string{
				kube.IngressClassAnnotation: "nginx",
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "service1",
				ServicePort: intstr.FromInt(80),
			},
		},
	}
	createIngress(&nginxIngress, cl.client, t)

	// Create a "real" ingress resource, with 4 host/path rules and an additional "default" rule.
	const expectedRuleCount = 5
	ingress := v1beta1.Ingress{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: ns,
			Annotations: map[string]string{
				kube.IngressClassAnnotation: mesh.IngressClass,
			},
		},
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: "default-service",
				ServicePort: intstr.FromInt(80),
			},
			Rules: []v1beta1.IngressRule{
				{
					Host: "host1.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/path1",
									Backend: v1beta1.IngressBackend{
										ServiceName: "service1",
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "/path2",
									Backend: v1beta1.IngressBackend{
										ServiceName: "service2",
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
				{
					Host: "host2.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/path3",
									Backend: v1beta1.IngressBackend{
										ServiceName: "service3",
										ServicePort: intstr.FromInt(80),
									},
								},
								{
									Path: "/path4",
									Backend: v1beta1.IngressBackend{
										ServiceName: "service4",
										ServicePort: intstr.FromInt(80),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	createIngress(&ingress, cl.client, t)

	eventually(func() bool {
		return notificationCount == expectedRuleCount
	}, t)
	if notificationCount != expectedRuleCount {
		t.Errorf("expected %d IngressRule events to be notified, found %d", expectedRuleCount, notificationCount)
	}

	eventually(func() bool {
		rules, _ := ctl.List(model.IngressRule)
		return len(rules) == expectedRuleCount
	}, t)
	rules, err := ctl.List(model.IngressRule)
	if err != nil {
		t.Errorf("ctl.List(model.IngressRule, %s) => error: %v", ns, err)
	}
	if len(rules) != expectedRuleCount {
		t.Errorf("expected %d IngressRule objects to be created, found %d", expectedRuleCount, len(rules))
	}

	for _, listMsg := range rules {
		getMsg, exists, _ := ctl.Get(model.IngressRule, listMsg.Key)
		if !exists {
			t.Errorf("expected IngressRule with key %v to exist", listMsg.Key)

			listRule := listMsg.Content.(*proxyconfig.IngressRule)
			getRule := getMsg.(*proxyconfig.IngressRule)

			// TODO:  Compare listRule and getRule objects
			if listRule == nil {
				t.Errorf("expected listRule to be of type *proxyconfig.RouteRule")
			}
			if getRule == nil {
				t.Errorf("expected getRule to be of type *proxyconfig.RouteRule")
			}
		}
	}

}

func TestIngressClass(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)
	cl.namespace = ns

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
		ingress := v1beta1.Ingress{
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
		ctl := NewController(cl, &mesh, kube.ControllerOptions{
			Namespace:    ns,
			ResyncPeriod: resync,
		})

		if c.ingressClass != "" {
			ingress.Annotations["kubernetes.io/ingress.class"] = c.ingressClass
		}

		if c.shouldProcess != ctl.shouldProcessIngress(&ingress) {
			t.Errorf("shouldProcessIngress(<ingress of class '%s'>) => %v, want %v",
				c.ingressClass, !c.shouldProcess, c.shouldProcess)
		}
	}
}

func TestController(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)
	cl.namespace = ns

	stop := make(chan struct{})
	defer close(stop)

	mesh := proxy.DefaultMeshConfig()
	ctl := NewController(cl, &mesh, kube.ControllerOptions{Namespace: ns, ResyncPeriod: resync})
	added, deleted := 0, 0
	n := 5
	ctl.RegisterEventHandler(mock.Type, func(c model.Config, ev model.Event) {
		switch ev {
		case model.EventAdd:
			if deleted != 0 {
				t.Errorf("Events are not serialized (add)")
			}
			added++
		case model.EventDelete:
			if added != n {
				t.Errorf("Events are not serialized (delete)")
			}
			deleted++
		}
		glog.Infof("Added %d, deleted %d", added, deleted)
	})
	go ctl.Run(stop)

	mock.CheckMapInvariant(cl, t, n)
	glog.Infof("Waiting till all events are received")
	eventually(func() bool { return added == n && deleted == n }, t)
}

func TestControllerCacheFreshness(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)
	cl.namespace = ns
	stop := make(chan struct{})
	mesh := proxy.DefaultMeshConfig()
	ctl := NewController(cl, &mesh, kube.ControllerOptions{Namespace: ns, ResyncPeriod: resync})

	// test interface implementation
	var _ model.ConfigStoreCache = ctl

	var doneMu sync.Mutex
	done := false

	// validate cache consistency
	ctl.RegisterEventHandler(mock.Type, func(config model.Config, ev model.Event) {
		elts, _ := ctl.List(mock.Type)
		switch ev {
		case model.EventAdd:
			if len(elts) != 1 {
				t.Errorf("Got %#v, expected %d element(s) on ADD event", elts, 1)
			}
			glog.Infof("Calling Delete(%#v)", config.Key)
			err = ctl.Delete(mock.Type, config.Key)
			if err != nil {
				t.Error(err)
			}
		case model.EventDelete:
			if len(elts) != 0 {
				t.Errorf("Got %#v, expected zero elements on DELETE event", elts)
			}
			glog.Infof("Stopping channel for (%#v)", config.Key)
			close(stop)
			doneMu.Lock()
			done = true
			doneMu.Unlock()
		}
	})

	go ctl.Run(stop)
	o := mock.Make(0)

	// add and remove
	glog.Infof("Calling Post(%#v)", o)
	if _, err := ctl.Post(o); err != nil {
		t.Error(err)
	}
	eventually(func() bool {
		doneMu.Lock()
		defer doneMu.Unlock()
		return done
	}, t)
}

func TestControllerClientSync(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	n := 5
	defer util.DeleteNamespace(cl.client, ns)
	cl.namespace = ns
	stop := make(chan struct{})
	defer close(stop)

	keys := make(map[int]*mock.MockConfig)
	// add elements directly through client
	for i := 0; i < n; i++ {
		keys[i] = mock.Make(i)
		if _, err := cl.Post(keys[i]); err != nil {
			t.Error(err)
		}
	}

	// check in the controller cache
	mesh := proxy.DefaultMeshConfig()
	ctl := NewController(cl, &mesh, kube.ControllerOptions{Namespace: ns, ResyncPeriod: resync})
	go ctl.Run(stop)
	eventually(func() bool { return ctl.HasSynced() }, t)
	os, _ := ctl.List(mock.Type)
	if len(os) != n {
		t.Errorf("ctl.List => Got %d, expected %d", len(os), n)
	}

	// remove elements directly through client
	for i := 0; i < n; i++ {
		if err := cl.Delete(mock.Type, keys[i].Key); err != nil {
			t.Error(err)
		}
	}

	// check again in the controller cache
	eventually(func() bool {
		os, _ = ctl.List(mock.Type)
		glog.Infof("ctl.List => Got %d, expected %d", len(os), 0)
		return len(os) == 0
	}, t)

	// now add through the controller
	for i := 0; i < n; i++ {
		if _, err := ctl.Post(mock.Make(i)); err != nil {
			t.Error(err)
		}
	}

	// check directly through the client
	eventually(func() bool {
		cs, _ := ctl.List(mock.Type)
		os, _ := cl.List(mock.Type)
		glog.Infof("ctl.List => Got %d, expected %d", len(cs), n)
		glog.Infof("cl.List => Got %d, expected %d", len(os), n)
		return len(os) == n && len(cs) == n
	}, t)

	// remove elements directly through the client
	for i := 0; i < n; i++ {
		if err := cl.Delete(mock.Type, keys[i].Key); err != nil {
			t.Error(err)
		}
	}
}

func eventually(f func() bool, t *testing.T) {
	interval := 64 * time.Millisecond
	for i := 0; i < 10; i++ {
		if f() {
			return
		}
		glog.Infof("Sleeping %v", interval)
		time.Sleep(interval)
		interval = 2 * interval
	}
	t.Errorf("Failed to satisfy function")
}

const (
	testService = "test"
	resync      = 1 * time.Second
)

func createIngress(ingress *v1beta1.Ingress, client kubernetes.Interface, t *testing.T) {
	if _, err := client.ExtensionsV1beta1().Ingresses(ingress.Namespace).Create(ingress); err != nil {
		t.Errorf("Cannot create ingress in namespace %s (error: %v)", ingress.Namespace, err)
	}
}

func TestIstioConfig(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)
	cl.namespace = ns

	rule := &proxyconfig.RouteRule{
		Destination: "foo",
		Name:        "test",
		Match: &proxyconfig.MatchCondition{
			HttpHeaders: map[string]*proxyconfig.StringMatch{
				"uri": {
					MatchType: &proxyconfig.StringMatch_Exact{
						Exact: "test",
					},
				},
			},
		},
	}

	if _, err := cl.Post(rule); err != nil {
		t.Errorf("cl.Post() => error %v, want no error", err)
	}

	out, exists, _ := cl.Get(model.RouteRule, rule.Name)
	if !exists {
		t.Errorf("cl.Get() => missing")
		return
	}

	if !reflect.DeepEqual(rule, out) {
		t.Errorf("cl.Get(%v) => %v, want %v", rule.Name, out, rule)
	}

	registry := model.MakeIstioStore(cl)

	rules := registry.RouteRules()
	if len(rules) != 1 || !reflect.DeepEqual(rules[rule.Name], rule) {
		t.Errorf("RouteRules() => %v, want %v", rules, rule)
	}

	destinations := registry.DestinationPolicies()
	if len(destinations) > 0 {
		t.Errorf("DestinationPolicies() => %v, want empty", destinations)
	}
}
