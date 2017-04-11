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

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/golang/glog"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/manager/model"
	"istio.io/manager/platform/kube"
	"istio.io/manager/platform/kube/inject"
	"istio.io/manager/proxy/envoy"
)

const (
	managerDiscovery = "manager-discovery"
	mixer            = "mixer"
	egressProxy      = "egress-proxy"
	ingressProxy     = "ingress-proxy"
	ca               = "ca"
	app              = "app"

	// budget is the maximum number of retries with 1s delays
	budget = 90

	// CA image tag is the short SHA *update manually*
	caTag = "f063b41"

	// Mixer image tag is the short SHA *update manually*
	mixerTag = "6655a67"

	ingressServiceName = "istio-ingress-controller"
)

type parameters struct {
	hub        string
	tag        string
	caImage    string
	mixerImage string
	namespace  string
	kubeconfig string
	count      int
	auth       string
	debug      bool
	parallel   bool
	logs       bool

	verbosity int
}

var (
	params parameters

	client      kubernetes.Interface
	istioClient *kube.Client

	// pods is a mapping from app name to a pod name (write once, read only)
	pods map[string]string

	// indicates whether the namespace is auto-generated
	nameSpaceCreated bool

	// Whether enable auth for the tests.
	enableAuth bool
)

func init() {
	flag.StringVar(&params.hub, "hub", "gcr.io/istio-testing", "Docker hub")
	flag.StringVar(&params.tag, "tag", "", "Docker tag")
	flag.StringVar(&params.caImage, "ca", "gcr.io/istio-testing/istio-ca:"+caTag,
		"CA Docker image")
	flag.StringVar(&params.mixerImage, "mixer", "gcr.io/istio-testing/mixer:"+mixerTag,
		"Mixer Docker image")
	flag.StringVar(&params.namespace, "n", "",
		"Namespace to use for testing (empty to create/delete temporary one)")
	flag.StringVar(&params.kubeconfig, "kubeconfig", "platform/kube/config",
		"kube config file (missing or empty file makes the test use in-cluster kube config instead)")
	flag.IntVar(&params.count, "count", 1, "Number of times to run the tests after deploying")
	flag.StringVar(&params.auth, "auth", "both", "Enable / disable auth, or test both.")
	flag.BoolVar(&params.debug, "debug", false, "Extra logging in the containers")
	flag.BoolVar(&params.parallel, "parallel", true, "Run requests in parallel")
	flag.BoolVar(&params.logs, "logs", true, "Validate pod logs (expensive in long-running tests)")
}

func main() {
	flag.Parse()
	switch params.auth {
	case "enable":
		enableAuth = true
		runTest()
		break
	case "disable":
		enableAuth = false
		runTest()
		break
	case "both":
		enableAuth = false
		runTest()
		time.Sleep(30 * time.Second)
		enableAuth = true
		runTest()
		break
	default:
		glog.Infof("Invald auth flag: %s. Please choose from: enable/disable/both.", params.auth)
	}
}

func runTest() {
	glog.Infof("\n--------------- Run tests with auth: %t ---------------", enableAuth)

	setup()

	for i := 0; i < params.count; i++ {
		glog.Infof("Test run: %d", i)
		check((&reachability{}).run(enableAuth))
		check(testRouting())
	}

	teardown()
	glog.Infof("\n--------------- All tests with auth: %t passed %d time(s)! ---------------\n", enableAuth, params.count)
}

func setup() {
	if params.tag == "" {
		glog.Fatal("No docker tag specified")
	}
	if params.mixerImage == "" {
		glog.Fatal("No mixer image specified")
	}
	glog.Infof("params %#v", params)

	if params.debug {
		params.verbosity = 3
	} else {
		params.verbosity = 2
	}

	check(setupClient())

	var err error
	if params.namespace == "" {
		if params.namespace, err = generateNamespace(client); err != nil {
			check(err)
		}
	} else {
		_, err = client.Core().Namespaces().Get(params.namespace, meta_v1.GetOptions{})
		check(err)
	}

	pods = make(map[string]string)

	if enableAuth {
		check(deploy("ca", "ca", ca, "", "", "", "", "unversioned", false))
		_, err = shell(fmt.Sprintf("kubectl -n %s apply -f test/integration/config-auth.yaml", params.namespace))
	} else {
		_, err = shell(fmt.Sprintf("kubectl -n %s apply -f test/integration/config.yaml", params.namespace))
	}
	check(err)

	// setup ingress resources
	_, _ = shell(fmt.Sprintf("kubectl -n %s create secret generic ingress "+
		"--from-file=tls.key=test/integration/cert.key "+
		"--from-file=tls.crt=test/integration/cert.crt",
		params.namespace))

	_, err = shell(fmt.Sprintf("kubectl -n %s apply -f test/integration/ingress.yaml", params.namespace))
	check(err)

	// deploy istio-infra
	check(deploy("http-discovery", "http-discovery", managerDiscovery, "8080", "80", "9090", "90", "unversioned", false))
	check(deploy("mixer", "mixer", mixer, "8080", "80", "9090", "90", "unversioned", false))
	check(deploy("istio-egress", "istio-egress", egressProxy, "8080", "80", "9090", "90", "unversioned", false))
	check(deploy("istio-ingress", "istio-ingress", ingressProxy, "8080", "80", "9090", "90", "unversioned", false))

	// deploy a healthy mix of apps, with and without proxy
	check(deploy("t", "t", app, "8080", "80", "9090", "90", "unversioned", false))
	check(deploy("a", "a", app, "8080", "80", "9090", "90", "unversioned", true))
	check(deploy("b", "b", app, "80", "8080", "90", "9090", "unversioned", true))
	check(deploy("hello", "hello", app, "8080", "80", "9090", "90", "v1", true))
	check(deploy("world-v1", "world", app, "80", "8000", "90", "9090", "v1", true))
	check(deploy("world-v2", "world", app, "80", "8000", "90", "9090", "v2", true))
	check(setPods())
}

// check function correctly cleans up on failure
func check(err error) {
	if err != nil {
		glog.Info(err)
		if glog.V(2) {
			for _, pod := range pods {
				glog.Info(podLogs(pod, "proxy"))
			}
		}
		teardown()
		os.Exit(1)
	}
}

// teardown removes resources
func teardown() {
	glog.Info("Cleaning up ingress secret.")
	if err := run("kubectl delete secret ingress -n " + params.namespace); err != nil {
		glog.Warning(err)
	}

	if nameSpaceCreated {
		deleteNamespace(client, params.namespace)
		params.namespace = ""
	}
}

func deploy(name, svcName, dType, port1, port2, port3, port4, version string, injectProxy bool) error {
	// write template
	configFile := name + "-" + dType + ".yaml"

	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			glog.Info("Error closing file " + configFile)
		}
	}()

	w := &bytes.Buffer{}
	if err := write("test/integration/"+dType+".yaml.tmpl", map[string]string{
		"service": svcName,
		"name":    name,
		"port1":   port1,
		"port2":   port2,
		"port3":   port3,
		"port4":   port4,
		"version": version,
	}, w); err != nil {
		return err
	}

	writer := bufio.NewWriter(f)
	if injectProxy {
		mesh := envoy.DefaultMeshConfig
		mesh.MixerAddress = "istio-mixer:9091"
		mesh.DiscoveryAddress = "istio-manager:8080"
		if enableAuth {
			mesh.AuthPolicy = proxyconfig.ProxyMeshConfig_MUTUAL_TLS
		}
		p := &inject.Params{
			InitImage:       inject.InitImageName(params.hub, params.tag),
			ProxyImage:      inject.ProxyImageName(params.hub, params.tag),
			Verbosity:       params.verbosity,
			SidecarProxyUID: inject.DefaultSidecarProxyUID,
			Version:         "manager-integration-test",
			Mesh:            &mesh,
		}
		if err := inject.IntoResourceFile(p, w, writer); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(writer, w); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	return run("kubectl apply -f " + configFile + " -n " + params.namespace)
}

/*
func waitForNewRestartEpoch(pod string, start int) error {
	log.Println("Waiting for Envoy restart epoch to increment from ", start)
	for n := 0; n < budget; n++ {
		current, err := getRestartEpoch(pod)
		if err != nil {
			log.Printf("Could not obtain Envoy restart epoch for %s: %v", pod, err)
		}

		if current > start {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("exceeded budget for waiting for envoy restart epoch to increment")
}

// getRestartEpoch gets the current restart epoch of a pod by calling the Envoy admin API.
func getRestartEpoch(pod string) (int, error) {
	url := "http://localhost:5000/server_info"
	cmd := fmt.Sprintf("kubectl exec %s -n %s -c app client %s", pods[pod], params.namespace, url)
	out, err := shell(cmd, true)
	if err != nil {
		return 0, err
	}

	// Response body is of the form: envoy 267724/RELEASE live 1571 1571 0
	// The last value is the restart epoch.
	match := regexp.MustCompile(`envoy .+/\w+ \w+ \d+ \d+ (\d+)`).FindStringSubmatch(out)
	if len(match) > 1 {
		epoch, err := strconv.ParseInt(match[1], 10, 32)
		return int(epoch), err
	}

	return 0, fmt.Errorf("could not obtain envoy restart epoch")
}
*/

func addConfig(config []byte, kind, name string, create bool) {
	glog.Infof("Add config %s", string(config))
	istioKind, ok := model.IstioConfig[kind]
	if !ok {
		check(fmt.Errorf("Invalid kind %s", kind))
	}
	v, err := istioKind.FromYAML(string(config))
	check(err)
	key := model.Key{
		Kind:      kind,
		Name:      name,
		Namespace: params.namespace,
	}
	if create {
		check(istioClient.Post(key, v))
	} else {
		check(istioClient.Put(key, v))
	}
}

func deployDynamicConfig(in string, data map[string]string, kind, name, envoy string) {
	config, err := writeString(in, data)
	check(err)
	_, exists := istioClient.Get(model.Key{Kind: kind, Name: name, Namespace: params.namespace})
	addConfig(config, kind, name, !exists)
	glog.Info("Sleeping for the config to propagate")
	time.Sleep(3 * time.Second)
}

func write(in string, data map[string]string, out io.Writer) error {
	// fallback to params values in data
	values := make(map[string]string)
	values["hub"] = params.hub
	values["tag"] = params.tag
	values["caImage"] = params.caImage
	values["mixerImage"] = params.mixerImage
	values["namespace"] = params.namespace
	values["verbosity"] = strconv.Itoa(params.verbosity)

	for k, v := range data {
		values[k] = v
	}
	tmpl, err := template.ParseFiles(in)

	if err != nil {
		return err
	}
	if err := tmpl.Execute(out, values); err != nil {
		return err
	}
	return nil
}

func writeString(in string, data map[string]string) ([]byte, error) {
	var bytes bytes.Buffer
	w := bufio.NewWriter(&bytes)
	if err := write(in, data, w); err != nil {
		return nil, err
	}

	if err := w.Flush(); err != nil {
		return nil, err
	}

	return bytes.Bytes(), nil
}

func run(command string) error {
	glog.Info(command)
	parts := strings.Split(command, " ")
	/* #nosec */
	c := exec.Command(parts[0], parts[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func shell(command string) (string, error) {
	glog.Info(command)
	parts := strings.Split(command, " ")
	/* #nosec */
	c := exec.Command(parts[0], parts[1:]...)
	bytes, err := c.CombinedOutput()
	if err != nil {
		glog.V(2).Info(string(bytes))
		return "", fmt.Errorf("command failed: %q %v", string(bytes), err)
	}
	return string(bytes), nil
}

// connect to K8S cluster and register TPRs
func setupClient() error {
	var err error
	istioClient, err = kube.NewClient(params.kubeconfig, model.IstioConfig)
	if err != nil {
		return err
	}
	client = istioClient.GetKubernetesClient()
	return istioClient.RegisterResources()
}

func setPods() error {
	var items []v1.Pod
	for n := 0; ; n++ {
		glog.Info("Checking all pods are running...")
		list, err := client.CoreV1().Pods(params.namespace).List(meta_v1.ListOptions{})
		if err != nil {
			return err
		}
		items = list.Items
		ready := true

		for _, pod := range items {
			if pod.Status.Phase != "Running" {
				glog.Infof("Pod %s has status %s\n", pod.Name, pod.Status.Phase)
				ready = false
				break
			}
		}

		if ready {
			break
		}

		if n > budget {
			for _, pod := range items {
				glog.Infof(podLogs(pod.Name, "proxy"))
			}
			return fmt.Errorf("exceeded budget for checking pod status")
		}

		time.Sleep(time.Second)
	}

	for _, pod := range items {
		if app, exists := pod.Labels["app"]; exists {
			pods[app] = pod.Name
		}
	}

	return nil
}

// podLogs gets pod logs by container
func podLogs(name string, container string) string {
	glog.Infof("Pod proxy logs %q", name)
	raw, err := client.CoreV1().Pods(params.namespace).
		GetLogs(name, &v1.PodLogOptions{Container: container}).
		Do().Raw()
	if err != nil {
		glog.Info("Request error", err)
		return ""
	}

	return string(raw)
}

func generateNamespace(cl kubernetes.Interface) (string, error) {
	ns, err := cl.Core().Namespaces().Create(&v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			GenerateName: "istio-integration-",
		},
	})
	if err != nil {
		return "", err
	}
	glog.Infof("Created namespace %s\n", ns.Name)
	nameSpaceCreated = true
	return ns.Name, nil
}

func deleteNamespace(cl kubernetes.Interface, ns string) {
	if cl != nil && ns != "" && ns != "default" {
		if err := cl.Core().Namespaces().Delete(ns, &meta_v1.DeleteOptions{}); err != nil {
			glog.Infof("Error deleting namespace: %v\n", err)
		}
		glog.Infof("Deleted namespace %s\n", ns)
	}
}
