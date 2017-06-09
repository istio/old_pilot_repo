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
	"os"
	"os/user"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/golang/glog"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"

	"io/ioutil"

	"bytes"

	"istio.io/pilot/model"
	"istio.io/pilot/proxy"
	"istio.io/pilot/test/mock"
	"istio.io/pilot/test/util"
)

const (
	controllerCertFile = "testdata/cert.crt"
	controllerKeyFile  = "testdata/cert.key"
)

func makeClient(t *testing.T) *Client {
	usr, err := user.Current()
	if err != nil {
		t.Fatal(err.Error())
	}

	kubeconfig := usr.HomeDir + "/.kube/config"

	// For Bazel sandbox we search a different location:
	if _, err = os.Stat(kubeconfig); err != nil {
		kubeconfig, _ = os.Getwd()
		kubeconfig = kubeconfig + "/config"
	}

	desc := append(model.IstioConfigTypes, mock.Types...)
	cl, err := NewClient(kubeconfig, desc, "istio-test")
	if err != nil {
		t.Fatal(err)
	}

	return cl
}

func TestSecret(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err)
	}
	defer util.DeleteNamespace(cl.client, ns)

	mesh := proxy.DefaultMeshConfig()
	ctl := NewController(cl, &mesh, ControllerOptions{
		Namespace:    ns,
		ResyncPeriod: resync,
	})

	stop := make(chan struct{})
	defer close(stop)
	go ctl.Run(stop)

	// create the secret
	cert, err := ioutil.ReadFile(controllerCertFile)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ioutil.ReadFile(controllerKeyFile)
	if err != nil {
		t.Fatal(err)
	}

	secret := "istio-secret"
	_, err = cl.client.Core().Secrets(ns).Create(&v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{Name: secret},
		Data:       map[string][]byte{secretCert: cert, secretKey: key},
	})
	if err != nil {
		t.Fatal(err)
	}

	eventually(func() bool {
		secret, err := cl.client.Core().Secrets(ns).Get(secret, meta_v1.GetOptions{})
		return secret != nil && err == nil
	}, t)

	uri := fmt.Sprintf("%s.%s", secret, ns)
	if tls, err := ctl.client.GetTLSSecret(uri); err != nil {
		t.Error(err)
	} else if tls == nil {
		t.Errorf("GetTLSSecret => no secret")
	} else if !bytes.Equal(cert, tls.Certificate) || !bytes.Equal(key, tls.PrivateKey) {
		t.Errorf("GetTLSSecret => got %q and %q, want %q and %q",
			string(tls.Certificate), string(tls.PrivateKey), string(cert), string(key))
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

func TestServices(t *testing.T) {
	cl := makeClient(t)
	t.Parallel()
	ns, err := util.CreateNamespace(cl.client)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer util.DeleteNamespace(cl.client, ns)

	stop := make(chan struct{})
	defer close(stop)

	mesh := proxy.DefaultMeshConfig()
	ctl := NewController(cl, &mesh, ControllerOptions{
		Namespace:    ns,
		ResyncPeriod: resync,
		DomainSuffix: domainSuffix,
	})
	go ctl.Run(stop)

	hostname := serviceHostname(testService, ns, domainSuffix)

	var sds model.ServiceDiscovery = ctl
	makeService(testService, ns, cl.client, t)
	eventually(func() bool {
		out := sds.Services()
		glog.Info("Services: %#v", out)
		return len(out) == 1 &&
			out[0].Hostname == hostname &&
			len(out[0].Ports) == 1 &&
			out[0].Ports[0].Protocol == model.ProtocolHTTP
	}, t)

	svc, exists := sds.GetService(hostname)
	if !exists {
		t.Errorf("GetService(%q) => %t, want true", hostname, exists)
	}
	if svc.Hostname != hostname {
		t.Errorf("GetService(%q) => %q", hostname, svc.Hostname)
	}

	missing := serviceHostname("does-not-exist", ns, domainSuffix)
	_, exists = sds.GetService(missing)
	if exists {
		t.Errorf("GetService(%q) => %t, want false", missing, exists)
	}
}

func makeService(n, ns string, cl kubernetes.Interface, t *testing.T) {
	_, err := cl.Core().Services(ns).Create(&v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{Name: n},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Port: 80,
					Name: "http-example",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf(err.Error())
	}
	glog.Infof("Created service %s", n)
}

func TestController_GetIstioServiceAccounts(t *testing.T) {
	clientSet := fake.NewSimpleClientset()
	mesh := proxy.DefaultMeshConfig()
	controller := NewController(&Client{client: clientSet}, &mesh, ControllerOptions{
		Namespace:    "default",
		ResyncPeriod: resync,
		DomainSuffix: domainSuffix,
	})

	createPod(controller, map[string]string{"app": "test-app"}, "pod1", "nsA", "acct1", t)
	createPod(controller, map[string]string{"app": "prod-app"}, "pod2", "nsA", "acct2", t)
	createPod(controller, map[string]string{"app": "prod-app"}, "pod3", "nsA", "acct3", t)
	createPod(controller, map[string]string{"app": "prod-app"}, "pod4", "nsA", "acct3", t)
	createPod(controller, map[string]string{"app": "prod-app"}, "pod5", "nsB", "acct4", t)

	// Populate pod cache.
	controller.pods.keys["128.0.0.1"] = "nsA/pod1"
	controller.pods.keys["128.0.0.2"] = "nsA/pod2"
	controller.pods.keys["128.0.0.3"] = "nsA/pod3"
	controller.pods.keys["128.0.0.4"] = "nsA/pod4"
	controller.pods.keys["128.0.0.5"] = "nsB/pod5"

	createService(controller, "svc1", "nsA", []int32{8080}, map[string]string{"app": "prod-app"}, t)
	createService(controller, "svc2", "nsA", []int32{8081}, map[string]string{"app": "staging-app"}, t)

	svc1Ips := []string{"128.0.0.1", "128.0.0.2"}
	portNames := []string{"test-port"}
	createEndpoints(controller, "svc1", "nsA", portNames, svc1Ips, t)

	hostname := serviceHostname("svc1", "nsA", domainSuffix)
	sa := controller.GetIstioServiceAccounts(hostname, []string{"test-port"})
	sort.Sort(sort.StringSlice(sa))
	expected := []string{
		"spiffe://company.com/ns/nsA/sa/acct1",
		"spiffe://company.com/ns/nsA/sa/acct2",
	}
	if !reflect.DeepEqual(sa, expected) {
		t.Errorf("Unexpected service accounts %v (expecting %v)", sa, expected)
	}

	hostname = serviceHostname("svc2", "nsA", domainSuffix)
	sa = controller.GetIstioServiceAccounts(hostname, []string{})
	if len(sa) != 0 {
		t.Error("Failure: Expected to resolve 0 service accounts, but got: ", sa)
	}
}

func createEndpoints(controller *Controller, name, namespace string, portNames, ips []string, t *testing.T) {
	eas := []v1.EndpointAddress{}
	for _, ip := range ips {
		eas = append(eas, v1.EndpointAddress{IP: ip})
	}

	eps := []v1.EndpointPort{}
	for _, name := range portNames {
		eps = append(eps, v1.EndpointPort{Name: name})
	}

	endpoint := &v1.Endpoints{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Subsets: []v1.EndpointSubset{{
			Addresses: eas,
			Ports:     eps,
		}},
	}
	if err := controller.endpoints.informer.GetStore().Add(endpoint); err != nil {
		t.Errorf("failed to create endpoints %s in namespace %s (error %v)", name, namespace, err)
	}
}

func createService(controller *Controller, name, namespace string, ports []int32, selector map[string]string,
	t *testing.T) {

	svcPorts := []v1.ServicePort{}
	for _, p := range ports {
		svcPorts = append(svcPorts, v1.ServicePort{
			Name:     "test-port",
			Port:     p,
			Protocol: "http",
		})
	}
	service := &v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1.ServiceSpec{
			ClusterIP: "10.0.0.1", // FIXME: generate?
			Ports:     svcPorts,
			Selector:  selector,
			Type:      v1.ServiceTypeClusterIP,
		},
	}
	if err := controller.services.informer.GetStore().Add(service); err != nil {
		t.Errorf("Cannot create service %s in namespace %s (error: %v)", name, namespace, err)
	}
}

func createPod(controller *Controller, labels map[string]string, name string, namespace string,
	serviceAccountName string, t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Labels:    labels,
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: serviceAccountName,
		},
	}
	if err := controller.pods.informer.GetStore().Add(pod); err != nil {
		t.Errorf("Cannot create pod in namespace %s (error: %v)", namespace, err)
	}
}
