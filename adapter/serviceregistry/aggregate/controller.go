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

package aggregate

import (
	"istio.io/pilot/model"
)

// Controller aggregates data across different registries and monitors for changes
type Controller struct {
	registries []model.Controller
}

// NewController creates a new Aggregate controller
func NewController(registries []model.Controller) *Controller {
	return &Controller{
		registries: registries,
	}
}

// Services list declarations of all services in the system
func (c *Controller) Services() []*model.Service {
	return nil
}

// GetService retrieves a service by host name if it exists
func (c *Controller) GetService(hostname string) (*model.Service, bool) {
	return nil, false
}

// ManagementPorts retries set of health check ports by instance IP.
// This does not apply to Consul service registry, as Consul does not
// manage the service instances. In future, when we integrate Nomad, we
// might revisit this function.
func (c *Controller) ManagementPorts(addr string) model.PortList {
	return nil
}

// Instances retrieves instances for a service and its ports that match
// any of the supplied tags. All instances match an empty tag list.
func (c *Controller) Instances(hostname string, ports []string, tags model.TagsList) []*model.ServiceInstance {
	return nil
}

// HostInstances lists service instances for a given set of IPv4 addresses.
func (c *Controller) HostInstances(addrs map[string]bool) []*model.ServiceInstance {
	return nil
}

// Run all controllers until a signal is received
func (c *Controller) Run(stop <-chan struct{}) {
}

// AppendServiceHandler implements a service catalog operation
func (c *Controller) AppendServiceHandler(f func(*model.Service, model.Event)) error {
	return nil
}

// AppendInstanceHandler implements a service catalog operation
func (c *Controller) AppendInstanceHandler(f func(*model.ServiceInstance, model.Event)) error {
	return nil
}

// GetIstioServiceAccounts implements model.ServiceAccounts operation
func (c *Controller) GetIstioServiceAccounts(hostname string, ports []string) []string {
	return nil
}
