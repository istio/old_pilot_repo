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

package memory_test

import (
	"testing"

	"istio.io/pilot/adapter/config/memory"
	"istio.io/pilot/model"
	"istio.io/pilot/test/mock"
)

func TestEventConsistency(t *testing.T) {
	// Create a Config Store
	store := memory.Make(mock.Types)

	// Create a controller
	controller := memory.NewController(store)

	testConfig := mock.Make(0)
	testEvent := model.EventAdd

	// Append notify handlers to the controller
	controller.RegisterEventHandler(mock.Type, func(config model.Config, event model.Event) {
		if event != testEvent {
			t.Errorf("desired %v, but %v", testEvent, event)
		}
		if config.Key != testConfig.Key {
			t.Errorf("desired %v, but %v", testConfig.Key, config.Key)
		}
	})

	stop := make(<-chan struct{})
	go controller.Run(stop)

	var revision string
	// Test Add Event
	if rev, err := controller.Post(testConfig); err != nil {
		t.Error(err)
	} else {
		revision = rev
	}

	testEvent = model.EventUpdate

	// Test Update Event
	if _, err := controller.Put(testConfig, revision); err != nil {
		t.Error(err)
	}

	testEvent = model.EventDelete

	// Test Delete Event
	if err := controller.Delete(mock.Type, testConfig.Key); err != nil {
		t.Error(err)
	}
}
