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
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/pkg/api/v1"
)

type verifyIngress struct {
	addr   string
	client *http.Client

	accessLogs  map[string][]string
	accessMutex sync.Mutex
}

// TODO: verify this works in all test environments
func (v *verifyIngress) getAddress() error {
	nodes, err := client.Core().Nodes().List(v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == "InternalIP" {
				v.addr = fmt.Sprintf("https://%v:%v", addr.Address, 32443)
				glog.Infof("Ingress address: %v", v.addr)
				return nil
			}
		}
	}
	return fmt.Errorf("could not find node ip")
}

// TODO: use TLS
func (v *verifyIngress) setupClient() {
	v.client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: time.Second,
	}
}

func (v *verifyIngress) makeRequest(path, host, dst string) func() error {
	re := regexp.MustCompile("X-Request-Id=(.*)")
	return func() error {
		u := v.addr + path
		glog.Infof("Making request to ingress %s\n", u)
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return err
		}
		req.Host = host

		resp, err := v.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if matches := re.FindSubmatch(data); len(matches) >= 1 {
			v.accessMutex.Lock()
			v.accessLogs[dst] = append(v.accessLogs[dst], string(matches[1]))
			v.accessMutex.Unlock()
			return nil
		}

		return fmt.Errorf("ingress proxy request to %s via %s with authority %v failed", dst, path, host)
	}
}

func (v *verifyIngress) makeRequests() error {
	type testCase struct {
		dst  string
		path string
		host string
	}

	tests := []testCase{
		{"a", "/a", ""},
		{"b", "/b", ""},
	}

	for _, t := range tests {
		if err := v.makeRequest(t.path, t.host, t.dst)(); err != nil {
			return err
		}
	}
	return nil
}

func (v *verifyIngress) setupResources() (err error) {
	_, err = shell(fmt.Sprintf("kubectl -n %s create secret generic ingress "+
		"--from-file=tls.key=test/integration/cert.key "+
		"--from-file=tls.crt=test/integration/cert.crt",
		params.namespace))
	if err != nil {
		return
	}

	_, err = shell(fmt.Sprintf("kubectl -n %s create -f test/integration/ingress.yaml", params.namespace))
	if err != nil {
		return
	}

	// wait for resources to sync
	// TODO: add retry logic so this is not necessary.
	time.Sleep(4 * time.Second)
	return
}

func (v *verifyIngress) run() error {
	glog.Info("Verifying ingress")

	v.accessLogs = make(map[string][]string)
	if err := v.setupResources(); err != nil {
		return err
	}
	v.setupClient()
	if err := v.getAddress(); err != nil {
		return err
	}
	if err := v.makeRequests(); err != nil {
		return err
	}

	return v.checkProxyAccessLogs()
}

// FIXME: this is copy/pasted. consolidate the logic.
func (v *verifyIngress) checkProxyAccessLogs() error {
	glog.Info("Checking access logs of pods to correlate request IDs...")
	for n := 0; n < budget; n++ {
		found := true
		for _, pod := range []string{"a", "b"} {
			glog.Infof("Checking access log of %s\n", pod)
			access := podLogs(pods[pod], "proxy")
			if strings.Contains(access, "segmentation fault") {
				return fmt.Errorf("segmentation fault in proxy %s", pod)
			}
			if strings.Contains(access, "assert failure") {
				return fmt.Errorf("assert failure in proxy %s", pod)
			}
			for _, id := range v.accessLogs[pod] {
				if !strings.Contains(access, id) {
					glog.Infof("Failed to find request id %s in log of %s\n", id, pod)
					found = false
					break
				}
			}
			if !found {
				break
			}
		}

		if found {
			return nil
		}

		time.Sleep(time.Second)
	}
	return fmt.Errorf("exceeded budget for checking access logs")
}
