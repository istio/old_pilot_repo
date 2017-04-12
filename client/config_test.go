package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"istio.io/manager/apiserver"
	"istio.io/manager/model"
)

type FakeHandler struct {
	expResponse   string
	expHeaders    http.Header
	expStatusCode int
}

func (f *FakeHandler) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(f.expStatusCode)
	for k, v := range f.expHeaders {
		w.Header().Set(k, v[0])
	}
	w.Write([]byte(f.expResponse))
}

var (
	errInvalidSpec = errors.New("cannot parse proto message: json: " +
		"cannot unmarshal string into Go value of type map[string]json.RawMessage")
	errNotFound    = &model.ItemNotFoundError{Key: model.Key{Kind: "kind", Name: "name", Namespace: "namespace"}}
	errInvalidType = fmt.Errorf("unknown configuration type not-a-route-rule; use one of %v",
		model.IstioConfig.Kinds())
	errItemExists = &model.ItemAlreadyExistsError{Key: model.Key{Kind: "kind",
		Name: "name", Namespace: "namespace"}}
)

func TestGetAddUpdateDeleteConfig(t *testing.T) {

	cases := []struct {
		name           string
		function       string
		key            model.Key
		kind           string
		namespace      string
		config         *apiserver.Config
		expConfig      *apiserver.Config
		expConfigSlice []apiserver.Config
		expHeaders     http.Header
		expStatus      int
		expError       error
	}{
		{
			name:       "TestConfigGet",
			function:   "get",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expConfig:  &apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusOK,
		},
		{
			name:       "TestConfigGetNotFound",
			function:   "get",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expError:   errNotFound,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusNotFound,
		},
		{
			name:       "TestConfigGetInvalidConfigType",
			function:   "get",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expError:   errInvalidType,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:       "TestConfigGetInvalidRespBody",
			function:   "get",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expError:   errInvalidType,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:       "TestConfigAdd",
			function:   "add",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusCreated,
		},
		{
			name:       "TestAddConfigConflict",
			function:   "add",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
			expError:   errItemExists,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusConflict,
		},
		{
			name:       "TestConfigAddInvalidConfigType",
			function:   "add",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "NOTATYPE", Name: "name", Spec: "spec"},
			expError:   errInvalidType,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:       "TestAddConfigInvalidSpec",
			function:   "add",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "type", Name: "name", Spec: "NOTASPEC"},
			expError:   errInvalidSpec,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:       "TestConfigUpdate",
			function:   "update",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusOK,
		},
		{
			name:       "TestConfigUpdateNotFound",
			function:   "update",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
			expError:   errNotFound,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusNotFound,
		},
		{
			name:       "TestConfigUpdateInvalidConfigType",
			function:   "update",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "NOTATYPE", Name: "name", Spec: "spec"},
			expError:   errInvalidType,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:       "TestUpdateConfigInvalidSpec",
			function:   "update",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			config:     &apiserver.Config{Type: "type", Name: "name", Spec: "NOTASPEC"},
			expError:   errInvalidSpec,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:       "TestConfigDelete",
			function:   "delete",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusOK,
		},
		{
			name:       "TestConfigDeleteNotFound",
			function:   "delete",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expError:   errNotFound,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusNotFound,
		},
		{
			name:       "TestConfigDeleteInvalidConfigType",
			function:   "delete",
			key:        model.Key{Name: "name", Namespace: "namespace", Kind: "route-rule"},
			expError:   errInvalidType,
			expHeaders: http.Header{"Content-Type": []string{"text/plain"}},
			expStatus:  http.StatusBadRequest,
		},
		{
			name:      "TestConfigListWithNamespace",
			function:  "list",
			kind:      "kind",
			namespace: "namespace",
			expConfigSlice: []apiserver.Config{
				apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
				apiserver.Config{Type: "type", Name: "name2", Spec: "spec"},
			},
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusOK,
		},
		{
			name:     "TestConfigListWithoutNamespace",
			function: "list",
			kind:     "kind",
			expConfigSlice: []apiserver.Config{
				apiserver.Config{Type: "type", Name: "name", Spec: "spec"},
				apiserver.Config{Type: "type", Name: "name2", Spec: "spec"},
			},
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusOK,
		},
		{
			name:       "TestConfigListWithNamespaceInvalidConfigType",
			function:   "list",
			kind:       "kind",
			namespace:  "namespace",
			expError:   errInvalidType,
			expHeaders: http.Header{"Content-Type": []string{"application/json"}},
			expStatus:  http.StatusBadRequest,
		},
	}
	for _, c := range cases {

		// Setup test server
		var exp string
		if c.expError != nil {
			exp = c.expError.Error()
		} else {
			if c.expConfigSlice != nil {
				e, _ := json.Marshal(c.expConfigSlice)
				exp = string(e)
			} else {
				e, _ := json.Marshal(c.expConfig)
				exp = string(e)
			}

		}
		fh := &FakeHandler{
			expResponse:   exp,
			expHeaders:    c.expHeaders,
			expStatusCode: c.expStatus,
		}
		ts := httptest.NewServer(http.HandlerFunc(fh.HandlerFunc))
		defer ts.Close()
		tsURL, _ := url.Parse(ts.URL)

		// Setup Client
		var config *apiserver.Config
		var configSlice []apiserver.Config
		var err error
		client := NewManagerClient(tsURL, "test", nil)
		switch c.function {
		case "get":
			config, err = client.GetConfig(c.key)
		case "add":
			err = client.AddConfig(c.key, *c.config)
		case "update":
			err = client.UpdateConfig(c.key, *c.config)
		case "delete":
			err = client.DeleteConfig(c.key)
		case "list":
			configSlice, err = client.ListConfig(c.kind, c.namespace)
		default:
			t.Fatal("didn't supply function to test case, don't know which client function to call")
		}

		// Verify
		if c.expError == nil && err != nil {
			t.Errorf("unexpected error: %v", err)
		} else if c.expError != nil && c.expError.Error() != err.Error() {
			t.Errorf("expected error: %v, but got %v", c.expError, err)
		} else if c.function == "get" {
			if !reflect.DeepEqual(config, c.expConfig) {
				t.Errorf("expected config: %+v, but received: %+v", c.expConfig, config)
			}
		} else if c.function == "list" {
			if !reflect.DeepEqual(configSlice, c.expConfigSlice) {
				t.Errorf("expected config slice: %+v, but received: %+v", c.expConfigSlice, configSlice)
			}
		}
	}
}
