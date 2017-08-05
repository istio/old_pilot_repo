package eureka

import (
	"github.com/golang/glog"
	"istio.io/pilot/model"
)

// NewServiceDiscovery instantiates an implementation of service discovery for Eureka
func NewServiceDiscovery(client Client) model.ServiceDiscovery {
	return &serviceDiscovery{
		client: client,
	}
}

type serviceDiscovery struct {
	client Client
}

// Services implements a service catalog operation
func (sd *serviceDiscovery) Services() []*model.Service {
	apps, err := sd.client.Applications()
	if err != nil {
		glog.Warningf("could not list Eureka instances: %v", err)
		return nil
	}
	services := convertServices(apps, nil)

	// TODO: canonical ordering?
	out := make([]*model.Service, 0, len(services))
	for _, service := range services {
		out = append(out, service)
	}
	return out
}

// GetService implements a service catalog operation
func (sd *serviceDiscovery) GetService(hostname string) (*model.Service, bool) {
	apps, err := sd.client.Applications()
	if err != nil {
		glog.Warningf("could not list Eureka instances: %v", err)
		return nil, false
	}

	services := convertServices(apps, map[string]bool{hostname: true})
	service := services[hostname]
	return service, service != nil
}

// Instances implements a service catalog operation
func (sd *serviceDiscovery) Instances(hostname string, ports []string,
	tagsList model.TagsList) []*model.ServiceInstance {

	apps, err := sd.client.Applications()
	if err != nil {
		glog.Warningf("could not list Eureka instances: %v", err)
		return nil
	}
	portSet := make(map[string]bool)
	for _, port := range ports {
		portSet[port] = true
	}
	services := convertServices(apps, map[string]bool{hostname: true})

	out := make([]*model.ServiceInstance, 0)
	for _, instance := range convertServiceInstances(services, apps) {
		if !tagsList.HasSubsetOf(instance.Tags) {
			continue
		}

		if len(portSet) > 0 && !portSet[instance.Endpoint.ServicePort.Name] {
			continue
		}

		out = append(out, instance)
	}
	return out
}

// HostInstances implements a service catalog operation
func (sd *serviceDiscovery) HostInstances(addrs map[string]bool) []*model.ServiceInstance {
	apps, err := sd.client.Applications()
	if err != nil {
		glog.Warningf("could not list Eureka instances: %v", err)
		return nil
	}
	services := convertServices(apps, nil)

	out := make([]*model.ServiceInstance, 0)
	for _, instance := range convertServiceInstances(services, apps) {
		if addrs[instance.Endpoint.Address] {
			out = append(out, instance)
		}
	}
	return out
}
