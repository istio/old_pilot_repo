package apiserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	restful "github.com/emicklei/go-restful"

	"istio.io/manager/model"
	"istio.io/manager/test/mock"
	test_util "istio.io/manager/test/util"
)

var (
	routeRuleKey = model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"}

	validRouteRuleJSON              = []byte(`{"type":"route-rule","name":"name","spec":{"destination":"service.namespace.svc.cluster.local","precedence":1,"route":[{"tags":{"version":"v1"},"weight":25}]}}`)
	validUpdatedRouteRuleJSON       = []byte(`{"type":"route-rule","name":"name","spec":{"destination":"service.namespace.svc.cluster.local","precedence":1,"route":[{"tags":{"version":"v2"},"weight":25}]}}`)
	validDiffNamespaceRouteRuleJSON = []byte(`{"type":"route-rule","name":"name","spec":{"destination":"service.differentnamespace.svc.cluster.local","precedence":1,"route":[{"tags":{"version":"v3"},"weight":25}]}}`)

	errItemExists  = &model.ItemAlreadyExistsError{Key: routeRuleKey}
	errNotFound    = &model.ItemNotFoundError{}
	errInvalidBody = errors.New("invalid character 'J' looking for beginning of value")
	errInvalidSpec = errors.New("cannot parse proto message: json: cannot unmarshal string into Go value of type map[string]json.RawMessage")
	errInvalidType = errors.New("unknown configuration type not-a-route-rule; use one of [destination-policy ingress-rule route-rule]")
)

func makeAPIServer(r *model.IstioRegistry) *API {
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

func TestAddUpdateGetDeleteConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"

	// Add the route-rule
	status, body := makeAPIRequest(api, "POST", url, validRouteRuleJSON, t)
	compareStatus(status, http.StatusCreated, t)
	test_util.CompareContent(body, "testdata/route-rule.json", t)
	compareStoredConfig(mockReg, routeRuleKey, true, t)

	// Update the route-rule
	status, body = makeAPIRequest(api, "PUT", url, validUpdatedRouteRuleJSON, t)
	compareStatus(status, http.StatusOK, t)
	test_util.CompareContent(body, "testdata/route-rule-v2.json", t)
	compareStoredConfig(mockReg, routeRuleKey, true, t)

	// Get the route-rule
	status, body = makeAPIRequest(api, "GET", url, nil, t)
	compareStatus(status, http.StatusOK, t)
	test_util.CompareContent(body, "testdata/route-rule-v2.json", t)

	// Delete the route-rule
	status, body = makeAPIRequest(api, "DELETE", url, nil, t)
	compareStatus(status, http.StatusOK, t)
	compareStoredConfig(mockReg, routeRuleKey, false, t)
}

func TestListConfig(t *testing.T) {

	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)

	// Add in two configs
	_, _ = makeAPIRequest(api, "POST", "/test/config/route-rule/namespace/v1", validRouteRuleJSON, t)
	_, _ = makeAPIRequest(api, "POST", "/test/config/route-rule/namespace/v2", validUpdatedRouteRuleJSON, t)

	// List them for a namespace
	status, body := makeAPIRequest(api, "GET", "/test/config/route-rule/namespace", nil, t)
	compareStatus(status, http.StatusOK, t)
	compareListCount(body, 2, t)

	// Add in third
	_, _ = makeAPIRequest(api, "POST", "/test/config/route-rule/differentnamespace/v3", validDiffNamespaceRouteRuleJSON, t)

	// List for all namespaces
	status, body = makeAPIRequest(api, "GET", "/test/config/route-rule", nil, t)
	compareStatus(status, http.StatusOK, t)
	compareListCount(body, 3, t)

}

///////////////////////////////////////////////////////
////////////////// GET CONFIG ERRORS //////////////////
///////////////////////////////////////////////////////

func TestNotFoundGetConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)

	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "GET", url, nil, t)
	compareStatus(status, http.StatusNotFound, t)
	compareResponseError(string(body), errNotFound, t)
}

func TestInvalidConfigTypeGetConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/not-a-route-rule/namespace/name"
	status, body := makeAPIRequest(api, "GET", url, nil, t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidType, t)
}

///////////////////////////////////////////////////////
////////////////// ADD CONFIG ERRORS //////////////////
///////////////////////////////////////////////////////

func TestMultipleAddConfigsReturnConflict(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	makeAPIRequest(api, "POST", url, validRouteRuleJSON, t)
	status, body := makeAPIRequest(api, "POST", url, validRouteRuleJSON, t)

	compareStatus(status, http.StatusConflict, t)
	compareResponseError(string(body), errItemExists, t)
}

func TestInvalidConfigTypeAddConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/not-a-route-rule/namespace/name"
	status, body := makeAPIRequest(api, "POST", url, validRouteRuleJSON, t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidType, t)
}

func TestInvalidBodyAddConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "POST", url, []byte("JUSTASTRING"), t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidBody, t)

}

func TestInvalidSpecAddConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "POST", url, []byte(`{"type":"route-rule","name":"name","spec":"NOTASPEC"}`), t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidSpec, t)
}

///////////////////////////////////////////////////////
//////////////// UPDATE CONFIG ERRORS /////////////////
///////////////////////////////////////////////////////

func TestNotFoundConfigUpdateConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "PUT", url, validRouteRuleJSON, t)

	compareStatus(status, http.StatusNotFound, t)
	compareResponseError(string(body), errNotFound, t)

}

func TestInvalidConfigTypeUpdateConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/not-a-route-rule/namespace/name"
	status, body := makeAPIRequest(api, "PUT", url, validRouteRuleJSON, t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidType, t)
}

func TestInvalidBodyUpdateConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "PUT", url, []byte("JUSTASTRING"), t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidBody, t)
}

func TestInvalidSpecUpdateConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "PUT", url, []byte(`{"type":"route-rule","name":"name","spec":"NOTASPEC"}`), t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidSpec, t)
}

///////////////////////////////////////////////////////
//////////////// DELETE CONFIG ERRORS /////////////////
///////////////////////////////////////////////////////

func TestNotFoundDeleteConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/route-rule/namespace/name"
	status, body := makeAPIRequest(api, "DELETE", url, validRouteRuleJSON, t)

	compareStatus(status, http.StatusNotFound, t)
	compareResponseError(string(body), errNotFound, t)

}

func TestInvalidConfigTypeDeleteConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/not-a-route-rule/namespace/name"
	status, body := makeAPIRequest(api, "DELETE", url, validRouteRuleJSON, t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidType, t)
}

///////////////////////////////////////////////////////
///////////////// LIST CONFIG ERRORS //////////////////
///////////////////////////////////////////////////////

func TestInvalidConfigTypeWithNamespaceListConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/not-a-route-rule/namespace"
	status, body := makeAPIRequest(api, "GET", url, nil, t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidType, t)
}

func TestInvalidConfigTypeWithoutNamespaceListConfig(t *testing.T) {
	mockReg := mock.MakeRegistry()
	api := makeAPIServer(mockReg)
	url := "/test/config/not-a-route-rule"
	status, body := makeAPIRequest(api, "GET", url, nil, t)

	compareStatus(status, http.StatusBadRequest, t)
	compareResponseError(string(body), errInvalidType, t)
}

///////////////////////////////////////////////////////
////////////////// HELPER FUNCTIONS ///////////////////
///////////////////////////////////////////////////////

func compareListCount(body []byte, expected int, t *testing.T) {
	configSlice := []Config{}
	if err := json.Unmarshal(body, &configSlice); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(configSlice) != expected {
		t.Errorf("expected %v elements back but got %v", expected, len(configSlice))
	}
}

func compareStatus(received, expected int, t *testing.T) {
	if received != expected {
		t.Errorf("Expected status code: %d, received: %d", expected, received)
	}
}

func compareResponseError(received string, expected error, t *testing.T) {
	if strings.Compare(received, expected.Error()) != 0 {
		t.Errorf("expected response body to be %v, but received: %v", expected.Error(), received)
	}
}

func compareStoredConfig(mockReg *model.IstioRegistry, key model.Key, present bool, t *testing.T) {
	_, ok := mockReg.Get(key)
	if !ok && present {
		t.Errorf("Expected config wasn't present in the registry for key: %+v", key)
	} else if ok && !present {
		t.Errorf("Unexpected config was present in the registry for key: %+v", key)
	}
	// To Do: compare protos
}
