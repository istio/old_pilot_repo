// Copyright 2017 Google Inc.
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
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/clientcmd"

	flag "github.com/spf13/pflag"
)

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

const (
	yaml = "echo.yaml"
)

func check(err error) {
	if err != nil {
		log.Fatalf(err.Error())
	}
}

var (
	hub string
	tag string

	client    *kubernetes.Clientset
	namespace string
)

func init() {
	flag.StringVarP(&hub, "hub", "h", "gcr.io/istio-test", "Docker hub")
	flag.StringVarP(&tag, "tag", "t", "test", "Docker tag")
}

func main() {
	flag.Parse()
	create := false

	// write template
	f, err := os.Create(yaml)
	check(err)
	w := bufio.NewWriter(f)

	check(write("test/integration/manager.yaml.tmpl", map[string]string{
		"hub": hub,
		"tag": tag,
	}, w))

	check(write("test/integration/http-service.yaml.tmpl", map[string]string{
		"hub":   hub,
		"tag":   tag,
		"name":  "a",
		"port1": "8080",
		"port2": "80",
	}, w))

	check(write("test/integration/http-service.yaml.tmpl", map[string]string{
		"hub":   hub,
		"tag":   tag,
		"name":  "b",
		"port1": "80",
		"port2": "8000",
	}, w))

	w.Flush()
	f.Close()

	// push docker images
	if create {
		run("gcloud docker --authorize-only")
		for _, image := range []string{"app", "runtime"} {
			run(fmt.Sprintf("bazel run //docker:%s", image))
			run(fmt.Sprintf("docker tag istio/docker:%s %s/%s:%s", image, hub, image, tag))
			run(fmt.Sprintf("docker push %s/%s:%s", hub, image, tag))
		}
		run("kubectl apply -f " + yaml)
	}
	/*
	   # Wait for pods to be ready
	   while : ; do
	     kubectl get pods | grep -i "init\|creat\|error" || break
	     sleep 1
	   done

	   # Get pod names
	   for pod in a b t; do
	     declare "${pod}"="$(kubectl get pods -l app=$pod -o jsonpath='{range .items[*]}{@.metadata.name}')"
	   done
	*/

	client = connect()
	namespace = "default"
	pods := getPods()
	log.Println(pods)
}

func run(command string) {
	log.Println("run", command)
	parts := strings.Split(command, " ")
	c := exec.Command(parts[0], parts[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	check(c.Run())
}

func connect() *kubernetes.Clientset {
	config, err := clientcmd.BuildConfigFromFlags("", "platform/kube/config")
	check(err)
	cl, err := kubernetes.NewForConfig(config)
	check(err)
	return cl
}

// pods returns pod names by app label as soon as all pods are ready
func getPods() map[string]string {
	pods := make([]v1.Pod, 0)
	out := make(map[string]string)
	for true {
		log.Println("Checking all pods are running...")
		list, err := client.Pods(namespace).List(v1.ListOptions{})
		check(err)
		pods = list.Items
		ready := true

		for _, pod := range pods {
			if pod.Status.Phase != "Running" {
				ready = false
				break
			}
		}

		if ready {
			break
		}

		time.Sleep(1 * time.Second)
	}

	for _, pod := range pods {
		if app, exists := pod.Labels["app"]; exists {
			out[app] = pod.Name
		}
	}

	return out
}

/*
# Try all pairwise requests
tt=false
for src in a b t; do
  for dst in a b t; do
    for port in "" ":80" ":8080"; do
      url="http://${dst}${port}/${src}"
      echo -e "\033[1m Requesting ${url} from ${src}... \033[0m"

      request=$(kubectl exec ${!src} -c app client ${url})

      echo $request | grep "X-Request-Id" ||\
        if [[ $src == "t" && $dst == "t" ]]; then
          tt=true
          echo Expected no request
        else
          echo Failed injecting proxy: request ${url}
          exit 1
        fi

      id=$(echo $request | grep -o "X-Request-Id=\S*" | cut -d'=' -f2-)
      echo x-request-id=$id

      # query access logs in src and dst
      for log in $src $dst; do
        if [[ $log != "t" ]]; then
          echo Checking access log of $log...

          n=1
          while : ; do
            if [[ $n == 30 ]]; then
              break
            fi
            kubectl logs ${!log} -c proxy | grep "$id" && break
            sleep 1
            ((n++))
          done

          if [[ $n == 30 ]]; then
            echo Failed to find request $id in access log of $log after $n attempts for $url
            exit 1
          fi
        fi
      done
    done
  done
done

echo -e "\033[1m Success! \033[0m"
*/
