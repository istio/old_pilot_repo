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
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"text/template"

	"strings"

	"istio.io/pilot/apiserver"
	"istio.io/pilot/client/proxy"
	"istio.io/pilot/cmd/version"
)

type StubClient struct {
	AddConfigCalled bool
	KeyConfigMap    map[proxy.Key]apiserver.Config
	WantKeys        map[proxy.Key]struct{}
	Error           error
}

func (st *StubClient) GetConfig(proxy.Key) (*apiserver.Config, error) {
	if st.Error != nil {
		return nil, st.Error
	}
	config, ok := st.KeyConfigMap[key]
	if !ok {
		return nil, fmt.Errorf("received unexpected key: %v", key)
	}
	if err := config.ParseSpec(); err != nil {
		return nil, err
	}
	return &config, nil
}

func (st *StubClient) AddConfig(key proxy.Key, config apiserver.Config) error {
	st.AddConfigCalled = true
	if st.Error != nil {
		return st.Error
	}
	return st.verifyKeyConfig(key, config)
}

func (st *StubClient) UpdateConfig(key proxy.Key, config apiserver.Config) error {
	if st.Error != nil {
		return st.Error
	}
	return st.verifyKeyConfig(key, config)
}

func (st *StubClient) DeleteConfig(key proxy.Key) error {
	if st.Error != nil {
		return st.Error
	}
	if _, ok := st.WantKeys[key]; !ok {
		return fmt.Errorf("received unexpected key: %v", key)
	}
	return nil

}

func (st *StubClient) ListConfig(string, string) ([]apiserver.Config, error) {
	if st.Error != nil {
		return nil, st.Error
	}
	var res []apiserver.Config
	for _, config := range st.KeyConfigMap {
		if err := config.ParseSpec(); err != nil {
			return nil, err
		}
		res = append(res, config)
	}
	return res, nil
}

func (st *StubClient) Version() (*version.BuildInfo, error) {
	return &version.BuildInfo{
		Version:       "StubClient version",
		GitRevision:   "StubClient git revision",
		GitBranch:     "StubClient branch",
		User:          "StubClient-user",
		Host:          "StubClient-host",
		GolangVersion: "StubClient golang version",
	}, nil
}

func (st *StubClient) setupTwoRouteRuleMap() {
	st.KeyConfigMap = make(map[proxy.Key]apiserver.Config)
	key1 := proxy.Key{
		Name:      "test-v1",
		Namespace: namespace,
		Kind:      "route-rule",
	}
	key2 := proxy.Key{
		Name:      "test-v2",
		Namespace: namespace,
		Kind:      "route-rule",
	}
	st.KeyConfigMap[key1] = apiserver.Config{
		Name: "test-v1",
		Type: "route-rule",
	}
	st.KeyConfigMap[key2] = apiserver.Config{
		Name: "test-v2",
		Type: "route-rule",
	}
}

func (st *StubClient) setupDeleteKeys() {
	st.WantKeys = make(map[proxy.Key]struct{})
	st.WantKeys[proxy.Key{Name: "test-v1", Namespace: "default", Kind: "route-rule"}] = struct{}{}
	st.WantKeys[proxy.Key{Name: "test-v2", Namespace: "default", Kind: "route-rule"}] = struct{}{}
}

func (st *StubClient) verifyKeyConfig(key proxy.Key, config apiserver.Config) error {
	wantConfig, ok := st.KeyConfigMap[key]
	if !ok {
		return fmt.Errorf("received unexpected key/config pair\n key: %+v\nconfig: %+v", key, config)
	}
	// ToDo: test spec as well
	if strings.Compare(wantConfig.Name, config.Name) != 0 {
		return fmt.Errorf("received unexpected config name: %s, wanted: %s", config.Name, wantConfig.Name)
	}
	if strings.Compare(wantConfig.Type, config.Type) != 0 {
		return fmt.Errorf("received unexpected config type: %s, wanted: %s", config.Type, wantConfig.Type)
	}
	return nil
}

func TestClientSideValidation(t *testing.T) {
	cases := []struct {
		name string
		file string
	}{
		{
			name: "TestCreateInvalidFile",
			file: "does-not-exist.yaml",
		},
		{
			name: "TestInvalidType",
			file: "testdata/invalid-type.yaml",
		},
		{
			name: "TestInvalidRouteRule",
			file: "testdata/invalid-route-rule.yaml",
		},
		{
			name: "TestInvalidDestinationPolicy",
			file: "testdata/invalid-destination-policy2.yaml",
		},
	}

	for _, c := range cases {
		stubClient := &StubClient{}
		apiClient = stubClient
		file = c.file
		if err := postCmd.RunE(postCmd, []string{}); err == nil {
			t.Fatalf("%s failed: %v", c.name, err)
		}
		if stubClient.AddConfigCalled {
			t.Fatalf("%s failed: AddConfig was called but it should have errored prior to calling", c.name)
		}
	}
}

func TestCreateUpdateDeleteGet(t *testing.T) {

	cases := []struct {
		name              string
		command           string
		file              string
		configKeyMapReq   bool
		deleteKeySliceReq bool
		wantError         bool
		arg               []string
		outFormat         string
	}{
		{
			name:            "TestCreateSuccess",
			command:         "post",
			file:            "testdata/two-route-rules.yaml",
			configKeyMapReq: true,
		},
		{
			name:      "TestCreateErrorsPassedBack",
			command:   "post",
			file:      "testdata/two-route-rules.yaml",
			wantError: true,
		},
		{
			name:      "TestCreateErrorsWithArg",
			command:   "post",
			file:      "testdata/two-route-rules.yaml",
			wantError: true,
			arg:       []string{"arg-im-a-pirate"},
		},
		{
			name:      "TestCreateNoFile",
			command:   "post",
			wantError: true,
		},
		{
			name:            "TestUpdateSuccess",
			command:         "put",
			file:            "testdata/two-route-rules.yaml",
			configKeyMapReq: true,
		},
		{
			name:      "TestUpdateErrorsPassedBack",
			command:   "put",
			file:      "testdata/two-route-rules.yaml",
			wantError: true,
		},
		{
			name:      "TestUpdateErrorsWithArg",
			command:   "put",
			file:      "testdata/two-route-rules.yaml",
			wantError: true,
			arg:       []string{"arg-im-a-pirate"},
		},
		{
			name:      "TestUpdateNoFile",
			command:   "put",
			wantError: true,
		},
		{
			name:              "TestDeleteSuccessWithFile",
			command:           "delete",
			deleteKeySliceReq: true,
			file:              "testdata/two-route-rules.yaml",
		},
		{
			name:      "TestDeleteArgsErrorWithFile",
			command:   "delete",
			file:      "testdata/two-route-rules.yaml",
			arg:       []string{"arg-im-a-pirate"},
			wantError: true,
		},
		{
			name:      "TestDeleteNoArgsErrorWithoutFile",
			command:   "delete",
			wantError: true,
		},
		{
			name:      "TestDeleteErrorsPassedBackWithFile",
			command:   "delete",
			file:      "testdata/two-route-rules.yaml",
			wantError: true,
		},
		{
			name:              "TestDeleteSuccessWithoutFile",
			command:           "delete",
			arg:               []string{"route-rule", "test-v1"},
			deleteKeySliceReq: true,
		},
		{
			name:      "TestDeleteErrorsPassedBackWithoutFile",
			command:   "delete",
			arg:       []string{"route-rule", "test-v1"},
			wantError: true,
		},
		{
			name:      "TestGetNoArgs",
			command:   "get",
			wantError: true,
		},
		{
			name:      "TestListPassesBackErrors",
			command:   "get",
			arg:       []string{"route-rule"},
			wantError: true,
			outFormat: "short",
		},
		{
			name:            "TestGetRouteRule",
			command:         "get",
			arg:             []string{"route-rule"},
			configKeyMapReq: true,
			outFormat:       "short",
		},
		{
			name:            "TestGetRouteRules",
			command:         "get",
			arg:             []string{"route-rules"},
			configKeyMapReq: true,
			outFormat:       "short",
		},
		{
			name:            "TestGetDestPolicy",
			command:         "get",
			arg:             []string{"destination-policy"},
			configKeyMapReq: true,
			outFormat:       "short",
		},
		{
			name:            "TestGetDestPolicies",
			command:         "get",
			arg:             []string{"destination-policies"},
			configKeyMapReq: true,
			outFormat:       "short",
		},
		{
			name:            "TestGetYAML",
			command:         "get",
			arg:             []string{"route-rule"},
			configKeyMapReq: true,
			outFormat:       "yaml",
		},
		{
			name:            "TestGetNotYAMLOrShort",
			command:         "get",
			arg:             []string{"route-rule"},
			configKeyMapReq: true,
			outFormat:       "not-an-output-format",
			wantError:       true,
		},
		{
			name:            "TestGetRouteRuleByName",
			command:         "get",
			arg:             []string{"route-rule", "test-v1"},
			configKeyMapReq: true,
			outFormat:       "short",
		},
		{
			name:      "TestGetRouteRuleByNamePassesBackErrors",
			command:   "get",
			arg:       []string{"route-rule", "test-v1"},
			wantError: true,
			outFormat: "short",
		},
	}

	for _, c := range cases {
		stubClient := &StubClient{}
		apiClient = stubClient
		if c.configKeyMapReq {
			stubClient.setupTwoRouteRuleMap()
		}
		if c.deleteKeySliceReq {
			stubClient.setupDeleteKeys()
		}
		if c.wantError {
			stubClient.Error = errors.New("an error")
		}
		outputFormat = c.outFormat
		file = c.file

		var err error
		switch c.command {
		case "post":
			err = postCmd.RunE(postCmd, c.arg)
		case "put":
			err = putCmd.RunE(putCmd, c.arg)
		case "delete":
			err = deleteCmd.RunE(deleteCmd, c.arg)
		case "get":
			err = getCmd.RunE(getCmd, c.arg)
		}
		if err != nil && !c.wantError {
			t.Fatalf("%v: %v", c.name, err)
		} else if err == nil && c.wantError {
			t.Fatalf("%v: expected an error", c.name)
		}
	}
}

// TestVersions invokes the 'istioctl version' subcommand
func TestVersions(t *testing.T) {

	if err := apiVersionCmd.RunE(apiVersionCmd, []string{}); err != nil {
		t.Errorf("istioctl version failed: %v", err)
	}
}

func TestEnvInterpolation(t *testing.T) {

	testEnv := map[string]string{"foo": "bar", "baz": "banana.com"}

	cases := []struct {
		rule    apiserver.Config
		env     map[string]string
		expects map[string]string
	}{
		{
			rule: apiserver.Config{
				Type: "route-rule",
				Name: "rr",
				Spec: map[string]interface{}{
					"destination": "${foo}.example.com",
					"precedence":  1,
				},
			},
			env: testEnv,
			expects: map[string]string{
				"{{ .Spec.destination }}": "bar.example.com",
			},
		},
		{
			rule: apiserver.Config{
				Type: "route-rule",
				Name: "rr2",
				Spec: map[string]interface{}{
					"destination": "notinterpolated.example.com",
					"precedence":  3,
					"match": map[string]interface{}{
						"source": "myhost.${baz}",
					},
				},
			},
			env: testEnv,
			expects: map[string]string{
				"{{ .Spec.match.source }}": "myhost.banana.com",
				"{{ .Spec.destination }}":  "notinterpolated.example.com",
			},
		},
		{
			rule: apiserver.Config{Type: "desination-policy",
				Name: "dp",
				Spec: map[string]interface{}{
					"destination": "${foo}.example.com",
					"policy": []map[string]interface{}{
						{"tags": map[string]string{"a": "b"}},
					},
				},
			},
			env: testEnv,
			expects: map[string]string{
				"{{ .Spec.destination }}": "bar.example.com",
			},
		},
	}

	for i, c := range cases {
		origEnv, created := pushEnv(c.env, t)
		interpolateRule(c.rule)
		popEnv(origEnv, created, t)
		for k, v := range c.expects {
			ev := testPathEvaluate(c.rule, k, t)
			if v != ev {
				t.Errorf("Case %d: After interpolation expected %s to be %q but got %q", i, k, v, ev)
			}
		}
	}
}

// pushEnv sets all of the additions to os.Environ.  Any old values are returned
// in `originals`.  If any vars were created their names are returned in `created`.
func pushEnv(additions map[string]string, t *testing.T) (map[string]string, []string) {
	originals := make(map[string]string)
	var created []string
	for addition, value := range additions {
		oldVal, existed := os.LookupEnv(addition)
		if existed {
			originals[addition] = oldVal
		} else {
			created = append(created, addition)
		}
		if err := os.Setenv(addition, value); err != nil {
			t.Error(err)
		}
	}

	return originals, created
}

// popEnv can restore env variables set by pushEnv()
func popEnv(originals map[string]string, created []string, t *testing.T) {
	for original, value := range originals {
		if err := os.Setenv(original, value); err != nil {
			t.Error(err)
		}
	}
	for _, cr := range created {
		if err := os.Unsetenv(cr); err != nil {
			t.Error(err)
		}
	}
}

func testPathEvaluate(v interface{}, path string, t *testing.T) string {
	buf := new(bytes.Buffer)
	tpl, err := template.New("test").Parse(path)
	if err != nil {
		t.Fatal(fmt.Errorf("could not parse template %q: %v", path, err))
	}
	if err = tpl.Execute(buf, v); err != nil {
		t.Fatal(fmt.Errorf("could not execute template %q: %v", path, err))
	}
	return buf.String()
}

type SimplerStubClient struct {
	KeyConfigMap map[model.Key]apiserver.Config
}

func NewSimplerStubClient() *SimplerStubClient {
	return &SimplerStubClient{KeyConfigMap: make(map[model.Key]apiserver.Config)}
}

func (ssc *SimplerStubClient) AddConfig(key model.Key, config apiserver.Config) error {
	if _, ok := ssc.KeyConfigMap[key]; ok {
		return fmt.Errorf("key %#v already present", key)
	}
	ssc.KeyConfigMap[key] = config
	return nil
}

func (ssc *SimplerStubClient) DeleteConfig(key model.Key) error {
	if _, ok := ssc.KeyConfigMap[key]; !ok {
		return fmt.Errorf("key %#v not present", key)
	}
	delete(ssc.KeyConfigMap, key)
	return nil
}

func (ssc *SimplerStubClient) GetConfig(model.Key) (*apiserver.Config, error) {
	config, ok := ssc.KeyConfigMap[key]
	if !ok {
		return nil, fmt.Errorf("key %#v not present", key)
	}
	return &config, nil
}

func (ssc *SimplerStubClient) ListConfig(string, string) ([]apiserver.Config, error) {
	retval := make([]apiserver.Config, len(ssc.KeyConfigMap))
	idx := 0
	for _, value := range ssc.KeyConfigMap {
		retval[idx] = value
		idx++
	}
	return retval, nil
}

func (ssc *SimplerStubClient) UpdateConfig(key model.Key, config apiserver.Config) error {
	if _, ok := ssc.KeyConfigMap[key]; !ok {
		return fmt.Errorf("key %#v not present", key)
	}
	ssc.KeyConfigMap[key] = config
	return nil
}

func (*SimplerStubClient) Version() (*version.BuildInfo, error) {
	return &version.BuildInfo{
		Version:       "SimplerStubClient version",
		GitRevision:   "SimplerStubClient git revision",
		GitBranch:     "SimplerStubClient branch",
		User:          "SimplerStubClient-user",
		Host:          "SimplerStubClient-host",
		GolangVersion: "SimplerStubClient golang version",
	}, nil
}

func TestCreateInterpolation(t *testing.T) {
	// Set env vars for test
	testEnv := map[string]string{"env1": "myapp", "env2": "banana.com"}
	origEnv, created := pushEnv(testEnv, t)

	stubClient := NewSimplerStubClient()
	file = "testdata/interpolated-rule.yaml"
	apiClient = stubClient
	err := postCmd.RunE(postCmd, []string{})
	if err != nil {
		t.Errorf("create failed: %v", err)
	}

	// Restore env vars
	popEnv(origEnv, created, t)

	testkey := model.Key{
		Name:      "interpolate1",
		Namespace: "default",
		Kind:      "route-rule",
	}
	if _, ok := stubClient.KeyConfigMap[testkey]; !ok {
		t.Fatalf("No key %v present in %v", testkey, stubClient.KeyConfigMap)
	}

	expect := map[string]string{
		"{{ .Spec.match.source }}": "somehost.banana.com",
		"{{ .Spec.destination }}":  "myapp.default.svc.cluster.local",
	}

	for k, v := range expect {
		ev := testPathEvaluate(stubClient.KeyConfigMap[testkey], k, t)
		if v != ev {
			t.Errorf("After interpolation expected %s to be %q but got %q", k, v, ev)
		}
	}
}
