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

package consul

import (
	"github.com/golang/glog"
	"github.com/hashicorp/consul/api"
	"istio.io/pilot/model"
)

type Controller struct {
	client *api.Client
}

func NewController() (*Controller, error) {
	client, err := api.NewClient(api.DefaultConfig())
	return &Controller{
		client: client,
	}, err
}

// Services list declarations of all services in the system
func (c *Controller) Services() []*model.Service {

	data := c.getServices()

	services := make([]*model.Service, 0, len(data))
	for name := range data {
		endpoints := c.getCatalogService(name, nil)
		services = append(services, convertService(endpoints))
	}

	return services
}

// GetService retrieves a service by host name if it exists
func (c *Controller) GetService(hostname string) (*model.Service, bool) {
	// Get actual service by name
	name, _, err := parseHostname(hostname)
	if err != nil {
		glog.V(2).Infof("parseHostname(%s) => error %v", hostname, err)
		return nil, false
	}

	endpoints := c.getCatalogService(name, nil)
	if len(endpoints) == 0 {
		return nil, false
	}

	return convertService(endpoints), true
}

func (c *Controller) getServices() map[string][]string {
	data, meta, err := c.client.Catalog().Services(nil)
	if err != nil {
		glog.Warningf("Could not retrieve services from consul: %v", err)
		return make(map[string][]string, 0)
	}

	//TODO log or process query metadata?
	glog.V(4).Infof("Response time: %v", meta.RequestTime)

	return data
}

func (c *Controller) getCatalogService(name string, q *api.QueryOptions) []*api.CatalogService {
	endpoints, meta, err := c.client.Catalog().Service(name, "", q)
	if err != nil {
		glog.Warningf("Could not retrieve service catalogue from consul: %v", err)
		return []*api.CatalogService{}
	}

	//TODO log or process query metadata info?
	glog.V(4).Infof("Response time: %v", meta.RequestTime)
	return endpoints
}

// Instances retrieves instances for a service and its ports that match
// any of the supplied tags. All instances match an empty tag list.
func (c *Controller) Instances(hostname string, ports []string, tags model.TagsList) []*model.ServiceInstance {
	// Get actual service by name
	name, _, err := parseHostname(hostname)
	if err != nil {
		glog.V(2).Infof("parseHostname(%s) => error %v", hostname, err)
		return nil
	}

	endpoints := c.getCatalogService(name, nil)

	instances := []*model.ServiceInstance{}
	for _, endpoint := range endpoints {
		inst := convertInstance(endpoint)
		if tags.HasSubsetOf(inst.Tags) && portMatch(inst, ports) {
			instances = append(instances, inst)
		}
	}

	return instances
}

// returns true if an instance's port matches with any in the provided list
func portMatch(inst *model.ServiceInstance, ports []string) bool {
	if len(ports) == 0 {
		return true
	}

	for _, port := range ports {
		if inst.Endpoint.ServicePort.Name == port {
			return true
		}
	}

	return false
}

// HostInstances lists service instances for a given set of IPv4 addresses.
func (c *Controller) HostInstances(addrs map[string]bool) []*model.ServiceInstance {

	data := c.getServices()
	out := make([]*model.ServiceInstance, 0)
	for svcName := range data {
		for addr := range addrs {
			// TODO assume provided address is a datacenter name?
			endpoints := c.getCatalogService(svcName, &api.QueryOptions{
				Datacenter: addr,
			})
			for _, endpoint := range endpoints {
				out = append(out, convertInstance(endpoint))
			}
		}
	}

	return out
}
