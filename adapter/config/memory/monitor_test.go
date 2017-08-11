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
	"bufio"
	"fmt"
	"os"
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

	recFile := "/tmp/events.log"
	f, err := os.Create(recFile)
	if err != nil {
		t.Errorf("Fail to open file %s", recFile)
	}
	w := bufio.NewWriter(f)

	// Append notify handlers to the controller
	controller.RegisterEventHandler(mock.Type, func(config model.Config, event model.Event) {
		switch event {
		case model.EventAdd:
			fmt.Fprintf(w, "Added\n")
		case model.EventUpdate:
			fmt.Fprintf(w, "Updated\n")
		case model.EventDelete:
			fmt.Fprintf(w, "Deleted\n")
		}
	})

	stop := make(<-chan struct{})
	go controller.Run(stop)
	mock.CheckMapInvariant(controller, t, 10)
	if err = w.Flush(); err != nil {
		t.Error(err)
	}
	if err = f.Close(); err != nil {
		t.Error(err)
	}

	added, updated, deleted := 0, 0, 0
	f, err = os.Open(recFile)
	if err != nil {
		t.Errorf("Fail to open file %s", recFile)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		event := scanner.Text()
		switch event {
		case "Added":
			added++
		case "Updated":
			updated++
		case "Deleted":
			deleted++
		}
	}

	if added != 10 || updated != 10 || deleted != 10 {
		t.Errorf("Event record check fails, %d %d %d", added, updated, deleted)
	}
}
