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

package memory

import (
	"time"

	"github.com/golang/protobuf/proto"
	"istio.io/pilot/model"
)

type controller struct {
	monitor     Monitor
	configStore model.ConfigStore
}

// NewController return an implementation of model.ConfigStoreCache
func NewController(cs model.ConfigStore) model.ConfigStoreCache {
	out := &controller{
		configStore: cs,
		monitor:     NewConfigsMonitor(cs, time.Second*1),
	}
	return out
}

func (c *controller) RegisterEventHandler(typ string, f func(model.Config, model.Event)) {
	c.monitor.AppendEventHandler(typ, f)
}

// Memory implementation is always synchronized with cache
func (c *controller) HasSynced() bool {
	return true
}

func (c *controller) Run(stop <-chan struct{}) {
	c.monitor.Start(stop)
}

func (c *controller) ConfigDescriptor() model.ConfigDescriptor {
	return c.configStore.ConfigDescriptor()
}

func (c *controller) Get(typ, key string) (proto.Message, bool, string) {
	return c.configStore.Get(typ, key)
}

func (c *controller) Post(val proto.Message) (out string, err error) {
	if out, err = c.configStore.Post(val); err == nil {
		c.monitor.UpdateConfigRecord()
	}
	return
}

func (c *controller) Put(val proto.Message, revision string) (out string, err error) {
	if out, err = c.configStore.Put(val, revision); err == nil {
		c.monitor.UpdateConfigRecord()
	}
	return
}

func (c *controller) Delete(typ, key string) (err error) {
	if err = c.configStore.Delete(typ, key); err == nil {
		c.monitor.UpdateConfigRecord()
	}
	return
}

func (c *controller) List(typ string) ([]model.Config, error) {
	return c.configStore.List(typ)
}
