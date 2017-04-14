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
	"os"
	"text/template"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/manager/model"
	"istio.io/manager/platform/kube"
)

const (
	// CA image tag is the short SHA *update manually*
	caTag = "f063b41"

	// Mixer image tag is the short SHA *update manually*
	mixerTag = "6655a67"
)

type parameters struct {
	infra
	kubeconfig string
	count      int
	debug      bool
	parallel   bool
	logs       bool
}

var (
	params parameters

	client      kubernetes.Interface
	istioClient *kube.Client

	// Enable/disable auth, or run both for the tests.
	authmode string

	budget = 90
)

func init() {
	flag.StringVar(&params.infra.Hub, "hub", "gcr.io/istio-testing", "Docker hub")
	flag.StringVar(&params.infra.Tag, "tag", "", "Docker tag")
	flag.StringVar(&params.infra.CaImage, "ca", "gcr.io/istio-testing/istio-ca:"+caTag,
		"CA Docker image")
	flag.StringVar(&params.infra.MixerImage, "mixer", "gcr.io/istio-testing/mixer:"+mixerTag,
		"Mixer Docker image")
	flag.StringVar(&params.infra.Namespace, "n", "",
		"Namespace to use for testing (empty to create/delete temporary one)")
	flag.StringVar(&params.kubeconfig, "kubeconfig", "platform/kube/config",
		"kube config file (missing or empty file makes the test use in-cluster kube config instead)")
	flag.IntVar(&params.count, "count", 1, "Number of times to run the tests after deploying")
	flag.StringVar(&authmode, "auth", "both", "Enable / disable auth, or test both.")
	flag.BoolVar(&params.debug, "debug", false, "Extra logging in the containers")
	flag.BoolVar(&params.parallel, "parallel", true, "Run requests in parallel")
	flag.BoolVar(&params.logs, "logs", true, "Validate pod logs (expensive in long-running tests)")
}

type test interface {
	setup() error
	run() error
	teardown()
}

func main() {
	flag.Parse()
	if params.infra.Tag == "" {
		glog.Fatal("No docker tag specified")
	}

	if params.debug {
		params.infra.Verbosity = 3
	} else {
		params.infra.Verbosity = 2
	}

	glog.Infof("params %#v", params)

	check(setupClient())

	switch authmode {
	case "enable":
		params.Auth = proxyconfig.ProxyMeshConfig_MUTUAL_TLS
		runTests()
	case "disable":
		params.Auth = proxyconfig.ProxyMeshConfig_NONE
		runTests()
	case "both":
		params.Auth = proxyconfig.ProxyMeshConfig_NONE
		runTests()
		params.Auth = proxyconfig.ProxyMeshConfig_MUTUAL_TLS
		runTests()
	default:
		glog.Infof("Invald auth flag: %s. Please choose from: enable/disable/both.", params.Auth)
	}
}

func runTests() {
	glog.Infof("\n--------------- Run tests with auth: %t ---------------", params.Auth)

	infra := params.infra
	infra.Mixer = true
	infra.Egress = true
	infra.Ingress = true

	check(infra.setup())
	check(infra.deployApps())

	tests := []test{
		&reachability{},
		&ingress{&infra},
		&egress{&infra},
		// testRouting
	}

	for i := 0; i < params.count; i++ {
		glog.Infof("Test run: %d", i)
		for _, test := range tests {
			check(test.setup())
			check(test.run())
			test.teardown()
		}
	}

	infra.teardown()
	glog.Infof("\n--------------- All tests with auth: %t passed %d time(s)! ---------------\n", params.Auth, params.count)
}

// check function correctly cleans up on failure
func check(err error) {
	if err != nil {
		glog.Info(err)
		/*
			if glog.V(2) {
				for _, pods := range apps {
					for _, pod := range pods {
						glog.Info(util.FetchLogs(client, pod, params.Namespace, "proxy"))
					}
				}
			}
		*/
		// TODO: teardown()
		glog.Info(err)
		os.Exit(1)
	}
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
	cmd := fmt.Sprintf("kubectl exec %s -n %s -c app client %s", pods[pod], params.Namespace, url)
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
		Namespace: params.Namespace,
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
	_, exists := istioClient.Get(model.Key{Kind: kind, Name: name, Namespace: params.Namespace})
	addConfig([]byte(config), kind, name, !exists)
	glog.Info("Sleeping for the config to propagate")
	time.Sleep(3 * time.Second)
}

// fill a file based on a template
func fill(inFile string, values interface{}) (string, error) {
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
