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

	// These HTTP methods are not correct, but allowed before the rules database becomes consistent
	retryOn map[int]bool

	// These response values are not correct, but allowed before the rules database becomes consistent
	retryIfMatch []interface{}
}

var (
	jsonRule = map[string]interface{}{
		"type": "route-rule",
		"name": "reviews-default",
		"spec": map[string]interface{}{
			"destination": "reviews.default.svc.cluster.local",
			"precedence":  float64(1),
			"route": []interface{}{map[string]interface{}{
				"tags": map[string]interface{}{
					"version": "v1",
				},
				"weight": float64(100),
			}},
		},
	}

	jsonRule2 = map[string]interface{}{
		"type": "route-rule",
		"name": "reviews-default",
		"spec": map[string]interface{}{
			"destination": "reviews.default.svc.cluster.local",
			"precedence":  float64(1),
			"route": []interface{}{map[string]interface{}{
				"tags": map[string]interface{}{
					"version": "v2",
				},
				"weight": float64(100),
			}},
		},
	}

	jsonInvalidRule = map[string]interface{}{
		"type": "route-rule",
		"name": "reviews-default",
		"spec": map[string]interface{}{
			"destination": "reviews.default.svc.cluster.local",
			"precedence":  float64(1),
			"route": []interface{}{map[string]interface{}{
				"tags": map[string]interface{}{
					"version": "v1",
				},
				"weight": float64(999),
			}},
		},
	}

	// The URL we will use for talking directly to apiserver
	testURL string
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
		return multierror.Append(err, fmt.Errorf("failed to retrieve mesh configuration"))
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

	testURL = "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-default"

	return nil
}

func (r *apiServerTest) teardown() {
	if r.stopChannel != nil {
		close(r.stopChannel)
	}
}

func (r *apiServerTest) run() error {
	if err := r.routeRuleInvalidDetected(); err != nil {
		return err
	}
	if err := r.routeRuleCRUD(); err != nil {
		return err
	}
	return nil
}

func (r *apiServerTest) routeRuleInvalidDetected() error {
	httpSequence := []httpRequest{
		// Can't create invalid rules
		{
			method: "POST", url: "http://localhost:8081/v1alpha1/config/route-rule/" + r.Namespace + "/reviews-invalid",
			data:                 jsonInvalidRule,
			expectedResponseCode: net_http.StatusInternalServerError,
		},
	}

	return verifySequence(httpSequence)
}

// routeRuleCRUD attempts to talk to the apiserver to Create, Retrieve, Update, and Delete a rule
func (r *apiServerTest) routeRuleCRUD() error {

	// Run through the lifecycle of updating a rule, with errors, to verify the sequence works as expected.
	httpSequence := []httpRequest{
		// Step 0 Can't get before created
		{
			method: "GET", url: testURL,
			expectedResponseCode: net_http.StatusNotFound,
		},
		// Step 1 Can create
		{
			method: "POST", url: testURL,
			data:                 jsonRule,
			expectedResponseCode: net_http.StatusCreated,
			expectedBody:         jsonRule,
			// @@ retryOn:              map[int]bool{net_http.StatusInternalServerError: true},
		},
		// Step 2 Can't create twice
		{
			// TODO should be StatusForbidden but the server is returning 500 (incorrectly?)
			method: "POST", url: testURL,
			data:                 jsonRule,
			expectedResponseCode: net_http.StatusInternalServerError,
		},
		// Step 3 Can get
		{
			method: "GET", url: testURL,
			expectedResponseCode: net_http.StatusOK,
			expectedBody:         jsonRule,
			retryOn:              map[int]bool{net_http.StatusNotFound: true},
		},
		// Step 4: Can update
		{
			method: "PUT", url: testURL,
			data:                 jsonRule2,
			expectedResponseCode: net_http.StatusOK,
			expectedBody:         jsonRule2,
		},
		// Can still GET after update
		{
			method: "GET", url: testURL,
			expectedResponseCode: net_http.StatusOK,
			expectedBody:         jsonRule2,
			retryOn:              map[int]bool{net_http.StatusNotFound: true},
			retryIfMatch:         []interface{}{jsonRule},
		},
		// Can delete
		{
			method: "DELETE", url: testURL,
			expectedResponseCode: net_http.StatusOK, // Should (perhaps?) be StatusNoContent
			retryOn:              map[int]bool{net_http.StatusNotFound: true},
		},
		// Can't delete twice
		{
			method: "DELETE", url: testURL,
			expectedResponseCode: net_http.StatusInternalServerError, // Should be StatusNotFound
		},
	}

	err := verifySequence(httpSequence)

	// On error, clean up any rule we created that didn't get deleted by the final requests
	if err != nil {
		// Attempt to delete rule, in case we created this and left it around due to a failure.
		client := &net_http.Client{}
		if req, err2 := net_http.NewRequest("DELETE", testURL, nil); err2 != nil {
			client.Do(req) // #nosec
		}
	}

	return err
}

// Verify a sequence of HTTP requests produce the expected responses
func verifySequence(httpSequence []httpRequest) error {

	for rulenum, hreq := range httpSequence {

		var err error
		var bytesToSend []byte
		if hreq.data != nil {
			bytesToSend, err = json.Marshal(hreq.data)
			if err != nil {
				return err
			}
		}

		// Verify a correct response comes back.  Retry up to 5 times in case data is initially incorrect
		if err = verifyRequest(hreq, bytesToSend, 5, rulenum); err != nil {
			return err
		}
	}

	return nil
}

// Verify an HTTP requests produces the expected responses or a small number of the expected error responses
func verifyRequest(hreq httpRequest, bytesToSend []byte, maxRetries int, rulenum int) error {
	client := &net_http.Client{}

	// Keep track if we need to retry before apiserver becomes consistent
	retries := 0

	for {
		req, err := net_http.NewRequest(hreq.method, hreq.url, bytes.NewBuffer(bytesToSend))
		if err != nil {
			return err
		}

		if hreq.method == "POST" || hreq.method == "PUT" {
			req.Header.Add("Content-Type", "application/json")
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if err = resp.Body.Close(); err != nil {
			return err
		}

		// Did apiserver fail to return the expected response code?
		if resp.StatusCode != hreq.expectedResponseCode {
			// Did it return a code that could lead to a correct code after data becomes consistent?
			if rt, ok := hreq.retryOn[resp.StatusCode]; !ok || !rt || retries > maxRetries {
				return fmt.Errorf("%v to %q expected %v but got %v %q on step %v after %v attempts", hreq.method, hreq.url,
					hreq.expectedResponseCode, resp.StatusCode, body, rulenum, retries)
			}

			// Give the system a moment after data-changing operations
			time.Sleep(1 * time.Second)
			fmt.Printf("@@@ ecs retrying %v %v because of status code\n", hreq.method, hreq.url)
			retries++
			continue
		}

		// Response code matches expectation.  We are done if we don't require a particular response body
		if hreq.expectedBody == nil {
			return nil
		}

		var jsonBody interface{}
		if err = json.Unmarshal(body, &jsonBody); err == nil {
			if reflect.DeepEqual(jsonBody, hreq.expectedBody) {
				return nil
			}

			if retries >= maxRetries {
				serializedExpectedBody, err := json.Marshal(hreq.expectedBody)
				if err != nil {
					return err
				}
				return fmt.Errorf("%v to %q expected JSON body %v but got %v on step %v after %v attempts", hreq.method, hreq.url,
					string(serializedExpectedBody), string(body), rulenum, retries+1)
			}

			// Does the data match transient data we tolerate while waiting for consistency?
			for retryIfMatch := range hreq.retryIfMatch {
				if reflect.DeepEqual(jsonBody, retryIfMatch) {
					// Give the system a moment after data-changing operations
					time.Sleep(1 * time.Second)
					retries++
					fmt.Printf("@@@ ecs retrying %v %v because of body\n", hreq.method, hreq.url)
					continue
				}
			}
		} else {
			// The returned data was not JSON
			if retries >= maxRetries {
				return fmt.Errorf("%v to %q expected body %v but got %q on step %v after %v attempts", hreq.method, hreq.url,
					hreq.expectedBody, body, rulenum, retries+1)
			}

			retries++
		}

	} // end for (ever)
}
