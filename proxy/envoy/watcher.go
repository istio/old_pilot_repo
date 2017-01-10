// Copyright 2017 Google Inc.
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

package envoy

import (
	"bytes"
	"time"

	"github.com/golang/glog"
	"istio.io/manager/model"
)

// Watcher observes service registry and triggers a reload on a change
type Watcher interface {
}

type watcher struct {
	agent     Agent
	config    string
	ds        model.ServiceDiscovery
	dsAddress string
}

func NewWatcher(ds model.ServiceDiscovery, ctl model.Controller, dsAddress string, binary string) Watcher {
	out := &watcher{
		agent:     NewAgent(binary),
		ds:        ds,
		dsAddress: dsAddress,
	}
	ctl.AppendServiceHandler(out.notify)
	return out
}

func (w *watcher) notify(svc *model.Service, ev model.Event) {
	config, err := Generate(w.ds.Services(), w.dsAddress)
	if err != nil {
		glog.Warningf("Failed to generate Envoy configuration: %v", err)
		return
	}

	buf := new(bytes.Buffer)
	err = config.Write(buf)
	if err != nil {
		glog.Warningf("Failed to serialize Envoy configuration: %v", err)
		return
	}

	data := buf.String()
	if data == w.config {
		glog.V(2).Info("Configuration is identical, skipping reload")
		return
	}

	glog.V(2).Infof("Config length %d bytes, old config length %d bytes", len(data), len(w.config))
	glog.V(10).Infof("Envoy config: %s", data)

	if err := w.agent.Reload(config); err != nil {
		glog.Warningf("Envoy reload error: %v", err)
		return
	}

	w.config = data

	// add a short delay to de-risk race conditions in envoy hot reload code
	time.Sleep(256 * time.Millisecond)
}
