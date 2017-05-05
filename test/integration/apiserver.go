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

// API server tests

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	net_http "net/http"
	"reflect"
	"time"

	multierror "github.com/hashicorp/go-multierror"

	"istio.io/manager/apiserver"
	"istio.io/manager/cmd"
	"istio.io/manager/model"
	"istio.io/manager/platform/kube"
)

type apiServerTest struct {
	*infra

	stopChannel chan struct{}
	// readyChannel chan struct{}
}

type httpRequest struct {
	method               string
	url                  string
	data                 interface{}
	expectedResponseCode int
	expectedBody         interface{}
}

const (
//	routeRule = `{"type":"route-rule","name":"reviews-default","spec":{"destination":"reviews.default.svc.cluster.local",` +
//		`"precedence":1,"route":[{"tags":{"version":"v1"},"weight":100}]}}`
//	routeRule2 = `{"type":"route-rule","name":"reviews-default","spec":{"destination":"reviews.default.svc.cluster.local",` +
//		`"precedence":1,"route":[{"tags":{"version":"v2"},"weight":100}]}}`
//	invalidRule = `{"type":"route-rule","name":"reviews-invalid","spec":{"destination":"reviews.default.svc.cluster.local",` +
//		`"precedence":1,"route":[{"tags":{"version":"v1"},"weight":999}]}}`
)

var (
	jsonRule = map[string]interface{} {
		"type": "route-rule",
		"name": "reviews-default",
		"spec": map[string]interface{} {
			"destination": "reviews.default.svc.cluster.local",
			"precedence": float64(1),
			"route": []interface{} { map[string]interface{} {
				"tags": map[string]interface{} {
					"version": "v1",
				},
				"weight": float64(100),
			} },
		},
	}

	jsonRule2 = map[string]interface{} {
		"type": "route-rule",
		"name": "reviews-default",
		"spec": map[string]interface{} {
			"destination": "reviews.default.svc.cluster.local",
			"precedence": float64(1),
			"route": []interface{} { map[string]interface{} {
				"tags": map[string]interface{} {
					"version": "v2",
				},
				"weight": float64(100),
			} },
		},
	}

	jsonInvalidRule = map[string]interface{} {
		"type": "route-rule",
		"name": "reviews-default",
		"spec": map[string]interface{} {
			"destination": "reviews.default.svc.cluster.local",
			"precedence": float64(1),
			"route": []interface{} { map[string]interface{} {
				"tags": map[string]interface{} {
					"version": "v1",
				},
				"weight": float64(999),
			} },
		},
	}

)

func (r *apiServerTest) String() string {
	return "apiserver"
}

func (r *apiServerTest) setup() error {

	// Start apiserver outside the cluster.

	var err error
	// receive mesh configuration
	mesh, err := cmd.GetMeshConfig(istioClient.GetKubernetesClient(), r.Namespace, "istio")
	if err != nil {
		return fmt.Errorf("failed to retrieve mesh configuration.")
		return multierror.Append(err, fmt.Errorf("failed to retrieve mesh configuration."))
	}

	controllerOptions := kube.ControllerOptions{}
	controller := kube.NewController(istioClient, mesh, controllerOptions)
	server := apiserver.NewAPI(apiserver.APIServiceOptions{
		Version:  kube.IstioResourceVersion,
		Port:     8081,
		Registry: &model.IstioRegistry{ConfigRegistry: controller},
	})
	r.stopChannel = make(chan struct{})
	go controller.Run(r.stopChannel)
	go server.Run()

	// Wait until apiserver is ready.  (As far as I can see there is no ready channel)
	for i := 0; i < 10; i++ {
		_, err := net_http.Get("http://localhost:8081/")
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

func (r *apiServerTest) teardown() {
	if r.stopChannel != nil {
		close(r.stopChannel)
	}
}

func (r *apiServerTest) run() error {
	if err := r.routeRuleCRUD(); err != nil {
		return err
	}
	return nil
}

// routeRuleCRUD attempts to talk to the apiserver to Create, Retrieve, Update, and Delete a rule
func (r *apiServerTest) routeRuleCRUD() error {
	client := &net_http.Client{}

	httpSequence := []httpRequest{
		// Can't get before created
		{
			"GET", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", "",
			net_http.StatusNotFound, nil,
		},
		// Can create
		{
			"POST", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", jsonRule,
			net_http.StatusCreated, jsonRule,
		},
		// Can't create twice
		{
			// TODO should be StatusForbidden but the server is returning 500 (incorrectly?)
			"POST", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", jsonRule,
			net_http.StatusInternalServerError, nil,
		},
		// Can't create invalid rules
		{
			// TODO should be net_http.StatusForbidden but the server is returning 500 (incorrectly?)
			"POST", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-invalid", jsonInvalidRule,
			net_http.StatusInternalServerError, nil,
		},
		// Can get
		{
			"GET", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", "",
			net_http.StatusOK, jsonRule,
		},
		// Can update
		{
			"PUT", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", jsonRule2,
			net_http.StatusOK, jsonRule2,
		},
		// Can still GET after update
		{
			"GET", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", "",
			net_http.StatusOK, jsonRule2,
		},
		// Can delete
		{
			// Should (perhaps?) be StatusNoContent
			"DELETE", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", "",
			net_http.StatusOK, nil,
		},
		// Can't delete twice
		{
			// Should be StatusNotFound
			"DELETE", "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default", "",
			net_http.StatusInternalServerError, nil,
		},
	}

	for _, hreq := range httpSequence {
		var err error
		var bytesToSend []byte
		if hreq.data != nil {
			bytesToSend, err = json.Marshal(hreq.data)
			if err != nil {
				return err
			}
		}
		req, err := net_http.NewRequest(hreq.method, hreq.url, bytes.NewBuffer(bytesToSend))
		if err != nil {
			return err
		}
		req.Header.Add("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != hreq.expectedResponseCode {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return fmt.Errorf("%v to %q expected %v but got %v %q", hreq.method, hreq.url, hreq.expectedResponseCode, resp.StatusCode, body)
		}
		if hreq.expectedBody != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			var jsonBody interface{}
    		if err = json.Unmarshal([]byte(body), &jsonBody); err == nil {
				if !reflect.DeepEqual(jsonBody, hreq.expectedBody) {
					serializedExpectedBody, _ := json.Marshal(hreq.expectedBody)
					if err != nil {
						return err
					}
					return fmt.Errorf("%v to %q expected JSON body %v but got %v", hreq.method, hreq.url, string(serializedExpectedBody), string(body))
				}
    		} else {
				// The returned data was not JSON, compare anyway
				if !reflect.DeepEqual(body, hreq.expectedBody) {
					return fmt.Errorf("%v to %q expected body %v but got %q", hreq.method, hreq.url, hreq.expectedBody, body)
				}
			}
		}
	}

	return nil
}

