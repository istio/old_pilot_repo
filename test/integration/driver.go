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

// Basic template engine using go templates

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ghodss/yaml"
	flag "github.com/spf13/pflag"

	"istio.io/manager/model"
	"istio.io/manager/platform/kube"
)

const (
	appDeployment = "echo.yaml"

	// budget is the maximum number of retries with 1s delays
	budget = 30
)

var (
	kubeconfig  string
	hub         string
	tag         string
	namespace   string
	dump        bool
	client      *kubernetes.Clientset
	istioClient *kube.Client
)

func teardown() {
	if err := istioClient.DeregisterResources(); err != nil {
		log.Printf("Error unregistering TPRs: %v\n", err)
	}
	deleteNamespace(client, namespace)
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	log.Printf("Test failure: %v\n", msg)
	teardown()
	os.Exit(1)
}

func init() {
	flag.StringVarP(&kubeconfig, "config", "c", "platform/kube/config",
		"kube config file or empty for in-cluster")
	flag.StringVarP(&hub, "hub", "h", "gcr.io/istio-testing",
		"Docker hub")
	flag.StringVarP(&tag, "tag", "t", "test",
		"Docker tag")
	flag.StringVarP(&namespace, "namespace", "n", "",
		"Namespace to use for testing (empty to create/delete temporary one)")
	flag.BoolVarP(&dump, "dump", "d", false,
		"Dump proxy logs from all containers")
}

func main() {
	flag.Parse()
	log.Printf("hub %v, tag %v", hub, tag)

	// connect to k8s and set up TPRs
	setup()
	if namespace == "" {
		namespace = generateNamespace(client)
	}

	pods := createAppDeployment()
	checkBasicReachability(pods)
	checkRouting(pods)
	teardown()
}

func createAppDeployment() map[string]string {
	// write template
	f, err := os.Create(appDeployment)
	check(err)
	w := bufio.NewWriter(f)

	check(write("test/integration/manager.yaml.tmpl", map[string]string{
		"hub": hub,
		"tag": tag,
	}, w))

	check(write("test/integration/http-service.yaml.tmpl", map[string]string{
		"hub":     hub,
		"tag":     tag,
		"name":    "a",
		"service": "a",
		"port1":   "8080",
		"port2":   "80",
		"version": "v1",
	}, w))

	check(write("test/integration/http-service.yaml.tmpl", map[string]string{
		"hub":     hub,
		"tag":     tag,
		"name":    "b",
		"service": "b",
		"port1":   "80",
		"port2":   "8000",
		"version": "v1",
	}, w))

	check(write("test/integration/http-service.yaml.tmpl", map[string]string{
		"hub":     hub,
		"tag":     tag,
		"name":    "b-v2",
		"service": "b",
		"port1":   "80",
		"port2":   "8000",
		"version": "v2",
	}, w))

	check(write("test/integration/external-services.yaml.tmpl", map[string]string{
		"hub":       hub,
		"tag":       tag,
		"namespace": namespace,
	}, w))

	check(w.Flush())
	check(f.Close())

	run("kubectl apply -f " + appDeployment + " -n " + namespace)
	pods := getPods()
	log.Println("pods:", pods)
	if dump {
		dumpProxyLogs(pods["a"])
		dumpProxyLogs(pods["b"])
	}

	return pods
}

func checkBasicReachability(pods map[string]string) {
	log.Printf("Verifying basic reachability across pods/services (a, b (b-v1), and t)..")
	ids := makeRequests(pods)
	log.Println("requests:", ids)
	checkAccessLogs(pods, ids)
	log.Println("Success!")
}

func checkRouting(pods map[string]string) {
	// First test default routing
	// Create a bytes buffer to hold the YAML form of rules
	log.Println("Routing all traffic to b-v1 and verifying..")
	var defaultRoute bytes.Buffer
	w := bufio.NewWriter(&defaultRoute)

	check(write("test/integration/rule-default-route.yaml.tmpl", map[string]string{
		"destination": "b",
		"namespace":   namespace,
	}, w))

	check(w.Flush())
	check(addRule(defaultRoute.Bytes(), "route-rule", "default-route", namespace))
	verifyRouting(pods, "a", "b", 100, map[string]int{
		"v1": 100,
		"v2": 0,
	})
	log.Println("Success!")

	log.Println("Routing 75% to b-v1 and 25% to b-v2 and verifying..")
	// Create a bytes buffer to hold the YAML form of rules
	var weightedRoute bytes.Buffer
	w = bufio.NewWriter(&weightedRoute)

	check(write("test/integration/rule-weighted-route.yaml.tmpl", map[string]string{
		"destination": "b",
		"namespace":   namespace,
	}, w))

	check(w.Flush())
	check(addRule(weightedRoute.Bytes(), "route-rule", "default-route", namespace))
	verifyRouting(pods, "a", "b", 100, map[string]int{
		"v1": 75,
		"v2": 25,
	})
	log.Println("Success!")
}

func addRule(ruleConfig []byte, kind string, name string, namespace string) error {

	if namespace == "" {
		namespace = api.NamespaceDefault
	}

	out, err := yaml.YAMLToJSON(ruleConfig)
	if err != nil {
		return fmt.Errorf("Cannot convert YAML rule to JSON: %v", err)
	}

	istioKind, ok := model.IstioConfig[kind]
	if !ok {
		return fmt.Errorf("Invalid kind %s", kind)
	}
	v, err := istioKind.FromJSON(string(out))
	if err != nil {
		return fmt.Errorf("Cannot parse proto message from JSON: %v", err)
	}

	istioClient := kube.Client{}
	err = istioClient.Put(model.Key{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}, v)

	return err
}

func write(in string, data map[string]string, out io.Writer) error {
	tmpl, err := template.ParseFiles(in)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(out, data); err != nil {
		return err
	}
	return nil
}

func run(command string) {
	log.Println(command)
	parts := strings.Split(command, " ")
	/* #nosec */
	c := exec.Command(parts[0], parts[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	check(c.Run())
}

func shell(command string) string {
	log.Println(command)
	parts := strings.Split(command, " ")
	/* #nosec */
	c := exec.Command(parts[0], parts[1:]...)
	bytes, err := c.CombinedOutput()
	if err != nil {
		log.Println(string(bytes))
		fail(err.Error())
	}
	return string(bytes)
}

// connect to K8S cluster and register TPRs
func setup() {
	var err error
	var config *rest.Config

	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	check(err)

	client, err = kubernetes.NewForConfig(config)
	check(err)

	istioClient = &kube.Client{}
	check(istioClient.RegisterResources())
}

// pods returns pod names by app label as soon as all pods are ready
func getPods() map[string]string {
	pods := make([]v1.Pod, 0)
	out := make(map[string]string)
	for n := 0; ; n++ {
		log.Println("Checking all pods are running...")
		list, err := client.Pods(namespace).List(v1.ListOptions{})
		check(err)
		pods = list.Items
		ready := true

		for _, pod := range pods {
			if pod.Status.Phase != "Running" {
				log.Printf("Pod %s has status %s\n", pod.Name, pod.Status.Phase)
				ready = false
				break
			}
		}

		if ready {
			break
		}

		if n > budget {
			for _, pod := range pods {
				dumpProxyLogs(pod.Name)
			}
			fail("Exceeded budget for checking pod status")
		}

		time.Sleep(time.Second)
	}

	for _, pod := range pods {
		if app, exists := pod.Labels["app"]; exists {
			out[app] = pod.Name
		}
	}

	return out
}

func dumpProxyLogs(name string) {
	log.Println("Pod proxy logs", name)
	raw, err := client.Pods(namespace).
		GetLogs(name, &v1.PodLogOptions{Container: "proxy"}).
		Do().Raw()
	if err != nil {
		log.Println("Request error", err)
	} else {
		log.Println(string(raw))
	}
}

// makeRequests executes requests in pods and collects request ids per pod to check against access logs
func makeRequests(pods map[string]string) map[string][]string {
	out := make(map[string][]string)
	for app := range pods {
		out[app] = make([]string, 0)
	}

	testPods := []string{"a", "b", "t"}
	for _, src := range testPods {
		for _, dst := range testPods {
			for _, port := range []string{"", ":80", ":8080"} {
				for _, domain := range []string{"", "." + namespace} {
					for n := 0; ; n++ {
						url := fmt.Sprintf("http://%s%s%s/%s", dst, domain, port, src)
						log.Printf("Making a request %s from %s (attempt %d)...\n", url, src, n)
						request := shell(fmt.Sprintf("kubectl exec %s -n %s -c app client %s",
							pods[src], namespace, url))
						log.Println(request)
						match := regexp.MustCompile("X-Request-Id=(.*)").FindStringSubmatch(request)
						if len(match) > 1 {
							id := match[1]
							log.Printf("id=%s\n", id)
							out[src] = append(out[src], id)
							out[dst] = append(out[dst], id)
							break
						}

						if src == "t" && dst == "t" {
							log.Println("Expected no match")
							break
						}

						if n > budget {
							fail(fmt.Sprintf("Failed to inject proxy from %s to %s (url %s)", src, dst, url))
						}

						time.Sleep(1 * time.Second)
					}
				}
			}
		}
	}

	return out
}

// checkAccessLogs searches for request ids in the access logs
func checkAccessLogs(pods map[string]string, ids map[string][]string) {
	for n := 0; ; n++ {
		found := true
		for _, pod := range []string{"a", "b"} {
			log.Printf("Checking access log of %s\n", pod)
			access := shell(fmt.Sprintf("kubectl logs %s -n %s -c proxy", pods[pod], namespace))
			for _, id := range ids[pod] {
				if !strings.Contains(access, id) {
					log.Printf("Failed to find request id %s in log of %s\n", id, pod)
					found = false
					break
				}
			}
			if !found {
				break
			}
		}

		if found {
			break
		}

		if n > budget {
			fail("Exceeded budget for checking access logs")
		}

		time.Sleep(time.Second)
	}
}

// verifyRouting verifies if the traffic is split as specified across different deployments in a service
func verifyRouting(pods map[string]string, src, dst string, samples int, expectedCount map[string]int) {
	count := make(map[string]int)
	for version := range expectedCount {
		count[version] = 0
	}

	domain := ""
	port := ""
	for i := 0; i < samples; i++ {
		url := fmt.Sprintf("http://%s%s%s/%s", dst, domain, port, src)
		log.Printf("Making a request %s from %s...\n", url, src)
		request := shell(fmt.Sprintf("kubectl exec %s -n %s -c app client %s",
			pods[src], namespace, url))
		log.Println(request)
		match := regexp.MustCompile("ServiceVersion=(.*)").FindStringSubmatch(request)
		if len(match) > 1 {
			id := match[1]
			count[id]++
			log.Printf("id=%s\n", id)
		}
	}

	epsilon := 2

	var failures int
	for version, expected := range expectedCount {
		if count[version] > expected+epsilon || count[version] < expected-epsilon {
			log.Printf("Expected %v requests (+/-%v) to reach %s => Got %v\n", expected, epsilon, version, count[version])
			failures++
		}
	}

	if failures > 0 {
		fail("Routing verification failed\n")
	}
}

func generateNamespace(cl *kubernetes.Clientset) string {
	ns, err := cl.Core().Namespaces().Create(&v1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: "istio-integration-",
		},
	})
	check(err)
	log.Printf("Created namespace %s\n", ns.Name)
	return ns.Name
}

func deleteNamespace(cl *kubernetes.Clientset, ns string) {
	if cl != nil && ns != "" && ns != "default" {
		if err := cl.Core().Namespaces().Delete(ns, &v1.DeleteOptions{}); err != nil {
			log.Printf("Error deleting namespace: %v\n", err)
		}
		log.Printf("Deleted namespace %s\n", ns)
	}
}
