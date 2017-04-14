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
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/golang/glog"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/manager/model"
	"istio.io/manager/platform/kube"
	"istio.io/manager/platform/kube/inject"
	"istio.io/manager/proxy/envoy"
	"istio.io/manager/test/util"
)

const (
	app = "app"

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
	auth       bool
	debug      bool
	parallel   bool
	logs       bool

	verbosity int
}

var (
	params parameters

	client      kubernetes.Interface
	istioClient *kube.Client

	// mapping from app name to pod names (write once, read only)
	apps map[string][]string

	// indicates whether the namespace is auto-generated
	namespaceCreated bool

	// Enable/disable auth, or run both for the tests.
	authmode string

	budget = 90
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
	flag.StringVar(&authmode, "auth", "both", "Enable / disable auth, or test both.")
	flag.BoolVar(&params.debug, "debug", false, "Extra logging in the containers")
	flag.BoolVar(&params.parallel, "parallel", true, "Run requests in parallel")
	flag.BoolVar(&params.logs, "logs", true, "Validate pod logs (expensive in long-running tests)")
}

func main() {
	flag.Parse()
	switch authmode {
	case "enable":
		params.auth = true
		runTests()
	case "disable":
		params.auth = false
		runTests()
	case "both":
		params.auth = false
		runTests()
		params.auth = true
		runTests()
	default:
		glog.Infof("Invald auth flag: %s. Please choose from: enable/disable/both.", params.auth)
	}
}

func runTests() {
	glog.Infof("\n--------------- Run tests with auth: %t ---------------", params.auth)

	setup()

	for i := 0; i < params.count; i++ {
		glog.Infof("Test run: %d", i)
		check((&reachability{}).run())
		check(testRouting())
	}

	teardown()
	glog.Infof("\n--------------- All tests with auth: %t passed %d time(s)! ---------------\n", params.auth, params.count)
}

// deploy complete infrastructure to ensure no conflict between components across test cases
func deployInfra(auth bool, namespace string) error {
	authPolicy := proxyconfig.ProxyMeshConfig_NONE.String()
	if auth {
		authPolicy = proxyconfig.ProxyMeshConfig_MUTUAL_TLS.String()
	}

	values := map[string]string{
		"hub":        params.hub,
		"tag":        params.tag,
		"verbosity":  strconv.Itoa(params.verbosity),
		"mixerImage": params.mixerImage,
		"caImage":    params.caImage,
		"authPolicy": authPolicy,
	}

	for _, infra := range []string{
		"config.yaml.tmpl",
		"manager.yaml.tmpl",
		"mixer.yaml.tmpl",
		"ca.yaml.tmpl",
		"ingress-proxy.yaml.tmpl",
		"egress-proxy.yaml.tmpl"} {
		if yaml, err := fill(infra, values); err != nil {
			return err
		} else if err = kubeApply(yaml, namespace); err != nil {
			return err
		}
	}

	return nil
}

func setup() {
	if params.tag == "" {
		glog.Fatal("No docker tag specified")
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
		if params.namespace, err = util.CreateNamespace(client); err != nil {
			check(err)
		}
		namespaceCreated = true
	} else {
		_, err = client.Core().Namespaces().Get(params.namespace, meta_v1.GetOptions{})
		check(err)
	}

	// setup ingress resources
	_, _ = util.Shell(fmt.Sprintf("kubectl -n %s create secret generic ingress "+
		"--from-file=tls.key=test/integration/testdata/cert.key "+
		"--from-file=tls.crt=test/integration/testdata/cert.crt",
		params.namespace))

	_, err = util.Shell(fmt.Sprintf("kubectl -n %s apply -f test/integration/testdata/ingress.yaml", params.namespace))
	check(err)

	// deploy istio-infra
	check(deployInfra(params.auth, params.namespace))

	// deploy a healthy mix of apps, with and without proxy
	check(deployApp("t", "t", "8080", "80", "9090", "90", "unversioned", false))
	check(deployApp("a", "a", "8080", "80", "9090", "90", "v1", true))
	check(deployApp("b-v1", "b", "80", "8080", "90", "9090", "v1", true))
	check(deployApp("b-v2", "b", "80", "8080", "90", "9090", "v2", true))
	check(deployApp("c", "c", "80", "8080", "90", "9090", "unversioned", true))

	apps, err = util.GetAppPods(client, params.namespace)
	check(err)
}

// check function correctly cleans up on failure
func check(err error) {
	if err != nil {
		glog.Info(err)
		if glog.V(2) {
			for _, pods := range apps {
				for _, pod := range pods {
					glog.Info(util.FetchLogs(client, pod, params.namespace, "proxy"))
				}
			}
		}
		teardown()
		glog.Info(err)
		os.Exit(1)
	}
}

// teardown removes resources
func teardown() {
	glog.Info("Cleaning up ingress secret.")
	if err := util.Run("kubectl delete secret ingress -n " + params.namespace); err != nil {
		glog.Warning(err)
	}

	if namespaceCreated {
		util.DeleteNamespace(client, params.namespace)
		params.namespace = ""
	}
	glog.Flush()
}

func deployApp(deployment, svcName, port1, port2, port3, port4, version string, injectProxy bool) error {
	w, err := fill("app.yaml.tmpl", map[string]string{
		"hub":        params.hub,
		"tag":        params.tag,
		"service":    svcName,
		"deployment": deployment,
		"port1":      port1,
		"port2":      port2,
		"port3":      port3,
		"port4":      port4,
		"version":    version,
	})
	if err != nil {
		return err
	}

	writer := new(bytes.Buffer)

	if injectProxy {
		mesh := envoy.DefaultMeshConfig
		mesh.MixerAddress = "istio-mixer:9091"
		mesh.DiscoveryAddress = "istio-manager:8080"
		if params.auth {
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
		if err := inject.IntoResourceFile(p, strings.NewReader(w), writer); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(writer, strings.NewReader(w)); err != nil {
			return err
		}
	}

	return kubeApply(writer.String(), params.namespace)
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
	out, err := util.Shell(cmd, true)
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

func deployDynamicConfig(inFile string, data map[string]string, kind, name, envoy string) {
	config, err := fill(inFile, data)
	check(err)
	_, exists := istioClient.Get(model.Key{Kind: kind, Name: name, Namespace: params.namespace})
	addConfig([]byte(config), kind, name, !exists)
	glog.Info("Sleeping for the config to propagate")
	time.Sleep(3 * time.Second)
}

// apply a kube config
func kubeApply(yaml, namespace string) error {
	return util.RunInput(fmt.Sprintf("kubectl apply -n %s -f -", namespace), yaml)
}

// fill a file based on a template
func fill(inFile string, values map[string]string) (string, error) {
	var bytes bytes.Buffer
	w := bufio.NewWriter(&bytes)

	tmpl, err := template.ParseFiles("test/integration/testdata/" + inFile)
	if err != nil {
		return "", err
	}

	if err := tmpl.Execute(w, values); err != nil {
		return "", err
	}

	if err := w.Flush(); err != nil {
		return "", err
	}

	return bytes.String(), nil
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
