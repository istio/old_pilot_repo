package vms

import (
	"strings"
	"strconv"
	"github.com/golang/protobuf/proto"
	"istio.io/pilot/model"
	proxyconfig "istio.io/api/proxy/v1/config"
)

const ()

type ControllerConfig struct {
	Discovery *Client
	Mesh	  *proxyconfig.ProxyMeshConfig
}

type Controller struct {
	discovery *Client
	mesh   *proxyconfig.ProxyMeshConfig
}

func NewController(config ControllerConfig) *Controller {
	controller := &Controller{
		discovery: config.Discovery,
		mesh : config.Mesh,
	}

	return controller
}

func (c *Controller) AppendConfigHandler(kind string, f func(model.Key, proto.Message, model.Event)) error {
	return nil
}

func (c *Controller) AppendServiceHandler(f func(*model.Service, model.Event)) error {
	return nil
}

func (c *Controller) AppendInstanceHandler(f func(*model.ServiceInstance, model.Event)) error {
	return nil
}

func (c *Controller) Run(stop <-chan struct{}) {}

func (c *Controller) Get(key model.Key) (proto.Message, bool) {
	return nil, true
}

func (c *Controller) List(kind string, namespace string) (map[model.Key]proto.Message, error) {
	return nil, nil
}

func (c *Controller) Post(key model.Key, v proto.Message) error {
	return nil
}

func (c *Controller) Put(key model.Key, v proto.Message) error {
	return nil
}

func (c *Controller) Delete(key model.Key) error {
	return nil
}
// Implements the Istio ServiceDiscovery interface
func (c *Controller) Services() []*model.Service {
	items, err := c.discovery.ListServiceObjects()

	// Failure in returning items, return nil
	if err != nil {
		return nil
	}

	services := make([]*model.Service, len(items), len(items))
	for idx, item := range items {
		services[idx] = convertService(item)
	}

	return services
}

func (c *Controller) GetService(hostname string) (*model.Service, bool) {
	item, err := c.discovery.GetServiceObject(hostname)

	// Failure in returning items, return nil
	if err != nil {
		return nil, false
	}

	// Each hostname should belong to one service object only
	if len(item) != 1 {
		return nil, false
	}

	return convertService(item[0]), true
}

func (c *Controller) Instances(hostname string, ports []string, tags model.TagsList) []*model.ServiceInstance {
	svc, err := c.discovery.GetServiceObject(hostname)
	if err != nil {
		return nil
	}

	service := convertService(svc[0])

	svcPorts := make(map[string]*model.Port)
	for _, p := range ports {
		if port, existed := service.Ports.Get(p); existed {
			svcPorts[p] = port
		}
	}

	items, err := c.discovery.ListServiceInstances(hostname)
	if err != nil {
		return nil
	}

	var instances []*model.ServiceInstance

	for _, item := range items {
		if svcPort, exists := svcPorts[item.Endpoint.ServicePort.Name]; exists {
			instanceTags := convertTags(item.Tags)
			if tags.HasSubsetOf(instanceTags) {
				addrPort := strings.Split(item.Endpoint.Value, ":")
				if len(addrPort) != 2 {
					return nil
				}

				port, err := strconv.Atoi(addrPort[1])
				if err != nil { return nil}

				instances = append(instances, &model.ServiceInstance{
					Endpoint: model.NetworkEndpoint {
						Address: addrPort[0],
						Port: port,
						ServicePort: svcPort,
					},
					Service: service,
					Tags: instanceTags,
				})
			}
		}
	}

	return instances
}

func (c *Controller) HostInstances(addrs map[string]bool) []*model.ServiceInstance {
	var instances []*model.ServiceInstance
	services, err := c.discovery.ListServiceObjects()

	if err != nil {
		return nil
	}

	for _, svc := range services {
		service := convertService(svc)
		if addrs[service.Address] {
			items, err := c.discovery.ListServiceInstances(service.Hostname)
			if err != nil { continue}
			for _, item := range items {
				addrPort := strings.Split(item.Endpoint.Value, ":")
				if len(addrPort) != 2 {
					return nil
				}

				port, err := strconv.Atoi(addrPort[1])
				if err != nil { return nil}

				svcPort, exists := service.Ports.Get(item.Endpoint.ServicePort.Name)

				if !exists {return nil}

				instances = append(instances, &model.ServiceInstance{
					Endpoint: model.NetworkEndpoint {
						Address: addrPort[0],
						Port: port,
						ServicePort: svcPort,
					},
					Service: service,
					Tags: convertTags(item.Tags),
				})
			}
		}
	}

	return instances
}

func (c *Controller) GetIstioServiceAccounts(hostname string, ports []string) []string {
	return nil
}
