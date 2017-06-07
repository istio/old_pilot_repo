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

package mock

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"

	"istio.io/pilot/model"
)

// Mock values
const (
	Kind = "mock-config"
)

// Mock values
var (
	ConfigObject = &MockConfig{
		Pairs: []*ConfigPair{
			{Key: "key", Value: "value"},
		},
	}
	Mapping = model.KindMap{
		Kind: model.ProtoSchema{
			MessageName: "mock.MockConfig",
			Validate:    func(proto.Message) error { return nil },
		},
	}
)

// MakeRegistry creates a mock config registry
func MakeRegistry() model.IstioRegistry {
	return &model.IstioConfigRegistry{
		ConfigRegistry: &ConfigRegistry{
			data: make(map[string]map[string]proto.Message),
		}}
}

// ConfigRegistry is a mock config registry
type ConfigRegistry struct {
	data map[string]map[string]proto.Message
}

// Get implements config registry method
func (cr *ConfigRegistry) Get(kind, key string) (proto.Message, bool) {
	data, ok := cr.data[kind]
	if !ok {
		return nil, false
	}
	val, ok := data[key]
	return val, ok
}

// Delete implements config registry method
func (cr *ConfigRegistry) Delete(kind, key string) error {
	if _, ok := cr.data[key]; ok {
		delete(cr.data, key)
		return nil
	}
	return &model.ItemNotFoundError{Key: key}
}

// Post implements config registry method
func (cr *ConfigRegistry) Post(kind string, v proto.Message) error {
	// TODO
	return &model.ItemAlreadyExistsError{Key: key}
}

// Put implements config registry method
func (cr *ConfigRegistry) Put(kind string, v proto.Message) error {
	// TODO
	return &model.ItemNotFoundError{Key: key}
}

// List implements config registry method
func (cr *ConfigRegistry) List(kind string) (map[string]proto.Message, error) {
	return cr.data, nil
}

// Make creates a fake config
func Make(i int) *MockConfig {
	return &MockConfig{
		Name: fmt.Sprintf("%s%d", "test-config", i),
		Pairs: []*ConfigPair{
			{Key: "key", Value: strconv.Itoa(i)},
		},
	}
}

// CheckMapInvariant validates operational invariants of a config registry
func CheckMapInvariant(r model.ConfigRegistry, t *testing.T, namespace string, n int) {
	// create configuration objects
	elts := make(map[int]*MockConfig)
	for i := 0; i < n; i++ {
		elts[i] = Make(i)
	}

	// post all elements
	for _, elt := range elts {
		if err := r.Post(Kind, elt); err != nil {
			t.Error(err)
		}
	}

	// check that elements are stored
	for _, elt := range elts {
		if v1, ok := r.Get(Kind, elt.Name); !ok || !reflect.DeepEqual(v1, elt) {
			t.Errorf("Wanted %v, got %v", elt, v1)
		}
	}

	// check for missing element
	if _, ok := r.Get(Kind, "missing"); ok {
		t.Error("Unexpected configuration object found")
	}

	// list elements
	l, err := r.List(Kind)
	if err != nil {
		t.Errorf("List error %#v, %v", l, err)
	}
	if len(l) != n {
		t.Errorf("Wanted %d element(s), got %d in %v", n, len(l), l)
	}

	// update all elements
	for i := 0; i < n; i++ {
		elts[i].Pairs[0].Value += "(updated)"
		if err = r.Put(Kind, elts[i]); err != nil {
			t.Error(err)
		}
	}

	// check that elements are stored
	for _, elt := range elts {
		if v1, ok := r.Get(Kind, elt.Name); !ok || !reflect.DeepEqual(v1, elt) {
			t.Errorf("Wanted %v, got %v", elt, v1)
		}
	}

	// delete all elements
	for _, elt := range elts {
		if err = r.Delete(Kind, elt.Name); err != nil {
			t.Error(err)
		}
	}

	l, err = r.List(Kind)
	if err != nil {
		t.Error(err)
	}
	if len(l) != 0 {
		t.Errorf("Wanted 0 element(s), got %d in %v", len(l), l)
	}
}
