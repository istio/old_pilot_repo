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

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	restful "github.com/emicklei/go-restful"

	"net/url"

	"github.com/stretchr/testify/assert"
	"istio.io/pilot/adapter/config/memory"
	"istio.io/pilot/model"
)

var (
	validRouteRuleJSON = []byte(`{"content":{"name":"name",` +
		`"destination":"service.namespace.svc.cluster.local","precedence":1,` +
		`"route":[{"tags":{"version":"v1"}}]}}`)
	validRouteRuleConfig = &Config{
		Type: "route-rule",
		Key:  "name",
		Content: map[string]interface{}{
			"destination": "service.namespace.svc.cluster.local",
			"name":        "name",
			"precedence":  float64(1),
			"route": []interface{}{
				map[string]interface{}{
					"tags": map[string]interface{}{
						"version": "v1",
					},
				},
			},
		},
	}
	validUpdatedRouteRuleJSON = []byte(`{"content":{"name":"name",` +
		`"destination":"service.namespace.svc.cluster.local","precedence":1,` +
		`"route":[{"tags":{"version":"v2"}}]}}`)
	validUpdatedRouteRuleConfig = &Config{
		Type: "route-rule",
		Key:  "name",
		Content: map[string]interface{}{
			"destination": "service.namespace.svc.cluster.local",
			"name":        "name",
			"precedence":  float64(1),
			"route": []interface{}{
				map[string]interface{}{
					"tags": map[string]interface{}{
						"version": "v2",
					},
				},
			},
		},
	}
	validDiffNamespaceRouteRuleJSON = []byte(`{"content":{"name":"new-name",` +
		`"destination":"service.differentnamespace.svc.cluster.local","precedence":1,` +
		`"route":[{"tags":{"version":"v3"}}]}}`)
	validDiffNamespaceRouteRuleConfig = &Config{
		Type: "route-rule",
		Key:  "new-name",
		Content: map[string]interface{}{
			"destination": "service.differentnamespace.svc.cluster.local",
			"name":        "new-name",
			"precedence":  float64(1),
			"route": []interface{}{
				map[string]interface{}{
					"tags": map[string]interface{}{
						"version": "v3",
					},
				},
			},
		},
	}

	errItemExists  = &model.ItemAlreadyExistsError{Key: "name"}
	errNotFound    = &model.ItemNotFoundError{Key: "name"}
	errInvalidBody = errors.New("invalid character 'J' looking for beginning of value")
	errInvalidSpec = errors.New("cannot parse proto message: json: " +
		"cannot unmarshal string into Go value of type map[string]json.RawMessage")
	errInvalidType = fmt.Errorf("missing type")
)

func makeAPIServer(r model.ConfigStore) *API {
	return &API{
		version:  "test",
		registry: r,
	}
}

func makeAPIRequest(api *API, method, url string, data []byte, t *testing.T) (int, []byte) {
	httpRequest, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	httpRequest.Header.Set("Content-Type", "application/json")
	if err != nil {
		t.Fatal(err)
	}
	httpWriter := httptest.NewRecorder()
	container := restful.NewContainer()
	api.Register(container)
	container.ServeHTTP(httpWriter, httpRequest)
	result := httpWriter.Result()
	body, err := ioutil.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	return result.StatusCode, body
}

func TestNewAPIThenRun(t *testing.T) {
	apiserver := NewAPI(APIServiceOptions{
		Version:  "v1alpha1",
		Port:     8081,
		Registry: memory.Make(model.IstioConfigTypes),
	})
	go apiserver.Run()
	apiserver.Shutdown(context.Background())
}

func TestHealthcheckt(t *testing.T) {
	api := makeAPIServer(nil)
	url := "/test/health"
	status, _ := makeAPIRequest(api, "GET", url, nil, t)
	compareStatus(status, http.StatusOK, t)
}

func TestAddUpdateGetDeleteConfig(t *testing.T) {
	mockReg := memory.Make(model.IstioConfigTypes)
	api := makeAPIServer(mockReg)

	// Add the route-rule
	addURLStr := "/test/config/route-rule"
	status, _ := makeAPIRequest(api, "POST", addURLStr, validRouteRuleJSON, t)
	compareStatus(status, http.StatusCreated, t)
	compareStoredConfig(mockReg, true, t)

	// Get the route-rule
	getURLStr := "/test/config/route-rule/name"
	status, body := makeAPIRequest(api, "GET", getURLStr, nil, t)
	compareStatus(status, http.StatusOK, t)
	compareReturnedConfig(body, validRouteRuleConfig, t)

	// Update the route-rule
	// Generate update URL
	tempConfig := &Config{}
	_ = json.Unmarshal(body, tempConfig)
	rev := tempConfig.Revision
	updateURL := url.URL{Path: fmt.Sprintf("/test/config/route-rule/%s", rev)}

	// Do the update
	status, body = makeAPIRequest(api, "PUT", updateURL.String(), validUpdatedRouteRuleJSON, t)
	compareStatus(status, http.StatusOK, t)
	compareStoredConfig(mockReg, true, t)

	// Get the route-rule again to verify update
	getURLStr = "/test/config/route-rule/name"
	status, body = makeAPIRequest(api, "GET", getURLStr, nil, t)
	compareStatus(status, http.StatusOK, t)
	compareReturnedConfig(body, validUpdatedRouteRuleConfig, t)

	// Delete the route-rule
	delURLStr := "/test/config/route-rule/name"
	status, _ = makeAPIRequest(api, "DELETE", delURLStr, nil, t)
	compareStatus(status, http.StatusOK, t)
	compareStoredConfig(mockReg, false, t)
}

func TestListConfig(t *testing.T) {
	mockReg := memory.Make(model.IstioConfigTypes)
	api := makeAPIServer(mockReg)

	_, _ = makeAPIRequest(api, "POST", "/test/config/route-rule", validRouteRuleJSON, t)
	_, _ = makeAPIRequest(api, "POST", "/test/config/route-rule", validDiffNamespaceRouteRuleJSON, t)

	// List for all namespaces
	status, body := makeAPIRequest(api, "GET", "/test/config/route-rule", nil, t)
	compareStatus(status, http.StatusOK, t)
	compareListCount(body, 2, t)
}

func TestConfigErrors(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		method     string
		data       []byte
		wantStatus int
		wantBody   string
		duplicate  bool
	}{
		{
			name:       "TestNotFoundGetConfig",
			url:        "/test/config/route-rule/name",
			method:     "GET",
			wantStatus: http.StatusNotFound,
			wantBody:   errNotFound.Error(),
		},
		{
			name:       "TestInvalidConfigTypeGetConfig",
			url:        "/test/config/not-a-route-rule/missing",
			method:     "GET",
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidType.Error(),
		},
		{
			name:       "TestMultipleAddConfigsReturnConflict",
			url:        "/test/config/route-rule",
			method:     "POST",
			data:       validRouteRuleJSON,
			wantStatus: http.StatusConflict,
			wantBody:   errItemExists.Error(),
			duplicate:  true,
		},
		{
			name:       "TestInvalidConfigTypeAddConfig",
			url:        "/test/config/not-a-route-rule",
			method:     "POST",
			data:       validRouteRuleJSON,
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidType.Error(),
		},
		{
			name:       "TestInvalidBodyAddConfig",
			url:        "/test/config/route-rule",
			method:     "POST",
			data:       []byte("JUSTASTRING"),
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidBody.Error(),
		},
		{
			name:       "TestNotFoundConfigUpdateConfig",
			url:        "/test/config/route-rule/rev",
			method:     "PUT",
			data:       validRouteRuleJSON,
			wantStatus: http.StatusNotFound,
			wantBody:   errNotFound.Error(),
		},
		{
			name:       "TestInvalidConfigTypeUpdateConfig",
			url:        "/test/config/not-a-route-rule/rev",
			method:     "PUT",
			data:       validRouteRuleJSON,
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidType.Error(),
		},
		{
			name:       "TestInvalidBodyUpdateConfig",
			url:        "/test/config/route-rule/rev",
			method:     "PUT",
			data:       []byte("JUSTASTRING"),
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidBody.Error(),
		},
		{
			name:       "TestNotFoundDeleteConfig",
			url:        "/test/config/route-rule/name",
			method:     "DELETE",
			wantStatus: http.StatusNotFound,
			wantBody:   errNotFound.Error(),
		},
		{
			name:       "TestInvalidConfigTypeDeleteConfig",
			url:        "/test/config/not-a-route-rule/key",
			method:     "DELETE",
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidType.Error(),
		},
		{
			name:       "TestInvalidConfigTypeListConfig",
			url:        "/test/config/not-a-route-rule",
			method:     "GET",
			wantStatus: http.StatusBadRequest,
			wantBody:   errInvalidType.Error(),
		},
	}

	for _, c := range cases {
		api := makeAPIServer(memory.Make(model.IstioConfigTypes))
		if c.duplicate {
			makeAPIRequest(api, c.method, c.url, c.data, t)
		}
		gotStatus, gotBody := makeAPIRequest(api, c.method, c.url, c.data, t)
		if gotStatus != c.wantStatus {
			t.Errorf("%s: got status code %v, want %v", c.name, gotStatus, c.wantStatus)
		}
		if string(gotBody) != c.wantBody {
			t.Errorf("%s: got body %q, want %q", c.name, string(gotBody), c.wantBody)
		}
	}
}

// TestVersion verifies that the server responds to /version
func TestVersion(t *testing.T) {
	api := makeAPIServer(nil)

	status, body := makeAPIRequest(api, "GET", "/test/version", nil, t)
	compareStatus(status, http.StatusOK, t)
	compareObjectHasKeys(body, []string{
		"version", "revision", "branch", "golang_version"}, t)

	// Test write failure (boost code coverage)
	makeAPIRequestWriteFails(api, "GET", "/test/version", nil, t)
}

// An http.ResponseWriter that always fails.
// (For testing handler method write failure handling.)
type grouchyWriter struct{}

func (gr grouchyWriter) Header() http.Header {
	return http.Header(make(map[string][]string))
}

func (gr grouchyWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("Write() failed")
}

func (gr grouchyWriter) WriteHeader(int) {
}

// makeAPIRequestWriteFails invokes a handler, but any writes the handler does fail.
func makeAPIRequestWriteFails(api *API, method, url string, data []byte, t *testing.T) {
	httpRequest, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	httpRequest.Header.Set("Content-Type", "application/json")
	if err != nil {
		t.Fatal(err)
	}
	httpWriter := grouchyWriter{}
	container := restful.NewContainer()
	api.Register(container)
	container.ServeHTTP(httpWriter, httpRequest)
}

func compareObjectHasKeys(body []byte, expectedKeys []string, t *testing.T) {
	version := make(map[string]interface{})
	if err := json.Unmarshal(body, &version); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, expectedKey := range expectedKeys {
		if _, ok := version[expectedKey]; !ok {
			t.Errorf("/version did not include %q: %v", expectedKey, string(body))
		}
	}
}

func compareListCount(body []byte, expected int, t *testing.T) {
	configSlice := []Config{}
	if err := json.Unmarshal(body, &configSlice); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	fmt.Printf("%+v\n", configSlice)
	if len(configSlice) != expected {
		t.Errorf("expected %v elements back but got %v", expected, len(configSlice))
	}
}

func compareStatus(received, expected int, t *testing.T) {
	if received != expected {
		t.Errorf("Expected status code: %d, received: %d", expected, received)
	}
}

func compareReturnedConfig(got []byte, want *Config, t *testing.T) {
	gotConfig := &Config{}
	_ = json.Unmarshal(got, gotConfig)
	want.Revision = gotConfig.Revision // Has to be dynamically assigned because its a datetime
	assert.Equal(t, want, gotConfig)
}

func compareStoredConfig(mockReg model.ConfigStore, present bool, t *testing.T) {
	_, ok, _ := mockReg.Get(model.RouteRule, "name")
	if !ok && present {
		t.Errorf("Expected config wasn't present in the registry for key: %+v", key)
	} else if ok && !present {
		t.Errorf("Unexpected config was present in the registry for key: %+v", key)
	}
	// To Do: compare protos
}
