package vms

import (
	"sort"
	"testing"

	"github.com/amalgam8/amalgam8/registry/client"
	"istio.io/pilot/model"
	"github.com/stretchr/testify/assert"
)

type Services []*model.Service
type Instances []*model.ServiceInstance

func GetGroundTrueServiceInstances() Instances {
	services := GetGroundTrueServices()
	detailsSvcPort, _ := services.GetService("details").Ports.Get("http")
	productpageSvcPort, _ := services.GetService("productpage").Ports.Get("http")
	ratingsSvcPort, _ := services.GetService("ratings").Ports.Get("http")
	reviewsSvcPort, _ := services.GetService("reviews").Ports.Get("http")
	instances := Instances{
		{
			Service: services.GetService("details"),
			Endpoint: model.NetworkEndpoint{
				Address:     "details-v1",
				Port:        6379,
				ServicePort: detailsSvcPort,
			},
			Tags:        model.Tags{"version": "v1"},
		},
		{
			Service: services.GetService("productpage"),
			Endpoint: model.NetworkEndpoint{
				Address:     "productpage-v1",
				Port:        6379,
				ServicePort: productpageSvcPort,
			},
			Tags:        model.Tags{"version": "v1"},
		},
		{
			Service: services.GetService("ratings"),
			Endpoint: model.NetworkEndpoint{
				Address:     "ratings-v1",
				Port:        6379,
				ServicePort: ratingsSvcPort,
			},
				Tags:        model.Tags{"version": "v1"},
		},
		{
			Service: services.GetService("reviews"),
			Endpoint: model.NetworkEndpoint{
				Address:     "reviews-v1",
				Port:        6379,
				ServicePort: reviewsSvcPort,
			},
				Tags:        model.Tags{"version": "v1"},
		},
		{
			Service: services.GetService("reviews"),
			Endpoint: model.NetworkEndpoint{
				Address:     "reviews-v2",
				Port:        6379,
				ServicePort: reviewsSvcPort,
			},
				Tags:        model.Tags{"version": "v2"},
		},
		{
			Service: services.GetService("reviews"),
			Endpoint: model.NetworkEndpoint{
				Address:     "reviews-v3",
				Port:        6379,
				ServicePort:  reviewsSvcPort,
			},
				Tags:        model.Tags{"version": "v3"},
		},
	}
	return instances
}

func GetGroundTrueServices() Services {
	services := Services{
		{
			Hostname: "details",
			Address:     "0.0.0.0",
			Ports: model.PortList{
				{
					Name:     "http",
					Port:     80,
					Protocol: "TCP",
				},
			},
			ExternalName: "",
		},
		{
			Hostname: "productpage",
			Address:     "0.0.0.0",
			Ports: model.PortList{
				{
					Name:     "http",
					Port:     80,
					Protocol: "TCP",
				},
			},
			ExternalName: "",
		},
		{
			Hostname: "ratings",
			Address:     "0.0.0.0",
			Ports: model.PortList{
				{
					Name:     "http",
					Port:     80,
					Protocol: "TCP",
				},
			},
			ExternalName: "",
		},
		{
			Hostname: "reviews",
			Address:     "0.0.0.0",
			Ports: model.PortList{
				{
					Name:     "http",
					Port:     80,
					Protocol: "TCP",
				},
			},
			ExternalName: "",
		},
	}

	return services
}

func newClient(t *testing.T) *client.Client {
	config := client.Config{
		URL: "http://localhost:31300",
	}

	c, err := client.New(config)

	if err != nil {
		t.Fatalf(err.Error())
	}

	return c
}

// Bookinfo containers are already on the cloud when the following tests
// functions are called.

// Test whether the returned Services information are the same as defined
func TestServices(t *testing.T) {
	client := newClient(t)
	controller := NewController(ControllerConfig{
		Discovery: client,
	})

	gtSvcs := GetGroundTrueServices()

	// construct a sorted list of service models
	retSvcs := Services(controller.Services())

	// sort list
	sort.Sort(retSvcs)

	// compare the groundtruth to the returned result
	CompareServices(t, gtSvcs, retSvcs)
}

func TestGetService(t *testing.T) {
	client := newClient(t)
	controller := NewController(ControllerConfig{
		Discovery: client,
	})

	gtSvcs := GetGroundTrueServices()

	testHostName := "reviews"

	retSvc, exits := controller.GetService(testHostName)
	if !exits {
		t.Fatal("Service of hostname:", testHostName, "does not exist on cloud.")
	}
	gtSvc := gtSvcs.GetService(testHostName)
	if gtSvc == nil {
		t.Fatal("Service of hostname:", testHostName, "does not exist in groundtruth.")
	}

	CompareService(t, gtSvc, retSvc)
}

func TestInstances(t *testing.T) {
	client := newClient(t)
	controller := NewController(ControllerConfig{
		Discovery: client,
	})

	gtInstances := GetGroundTrueServiceInstances()

	testHostname := "reviews"
	testPorts := []string{"http"}

	testTags := make(model.TagsList, 0)
	testTags = append(testTags, model.Tags{"version":"v1"})
	testTags = append(testTags, model.Tags{"version":"v2"})

	gtInstances = gtInstances.GetInstances(testHostname)
	gtInsts := make([]*model.ServiceInstance, 0)
	for _, inst := range gtInstances {
		for _, port := range testPorts {
			if inst.Endpoint.ServicePort.Name == port &&
				testTags.HasSubsetOf(inst.Tags) {
				gtInsts = append(gtInsts, inst)
			}
		}
	}

	retInsts := Instances(controller.Instances(testHostname, testPorts, testTags))
	sort.Sort(retInsts)

	CompareInstances(t, gtInsts, retInsts)
}

func TestHostInstances(t *testing.T) {
	client := newClient(t)
	controller := NewController(ControllerConfig{
		Discovery: client,
	})

	gtInstances := GetGroundTrueServiceInstances()

	testAddress := "0.0.0.0"

	testAddrs := map[string]bool{testAddress: true}

	gtInsts := gtInstances.GetHostInstances(testAddrs)
	retInsts := Instances(controller.HostInstances(testAddrs))
	sort.Sort(retInsts)

	CompareInstances(t, gtInsts, retInsts)
}

func (slice Services) Len() int {
	return len(slice)
}

func (slice Services) Less(i, j int) bool {
	return slice[i].Hostname < slice[j].Hostname
}

func (slice Services) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (slice Instances) Len() int {
	return len(slice)
}

// Only applicable to this specific test
func (slice Instances) Less(i, j int) bool {
	return slice[i].Service.Hostname < slice[j].Service.Hostname ||
		slice[i].Tags["version"] < slice[j].Tags["version"]
}

func (slice Instances) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (slice Services) GetService(hostName string) *model.Service {
	for _, service := range slice {
		if service.Hostname == hostName {
			return service
		}
	}

	return nil
}

func (slice Instances) GetInstances(hostname string) []*model.ServiceInstance {
	out := make([]*model.ServiceInstance, 0)

	for _, ins := range slice {
		if ins.Service.Hostname == hostname {
			out = append(out, ins)
		}
	}

	return out
}

func (slice Instances) GetHostInstances(addrs map[string]bool) []*model.ServiceInstance {
	out := make([]*model.ServiceInstance, 0)

	for _, ins := range slice {
		if addrs[ins.Service.Address] {
			out = append(out, ins)
		}
	}

	return out
}

func CompareInstances(t *testing.T, gtInsts, retInsts Instances) {
	assert.Equal(t, len(gtInsts), len(retInsts))
	for idx, gtInst := range gtInsts {
		retInst := retInsts[idx]
		CompareInstance(t, gtInst, retInst)
	}
}

func CompareInstance(t *testing.T, gtInst, retInst *model.ServiceInstance) {
	CompareService(t, gtInst.Service, retInst.Service)

	assert.Equal(t, gtInst.Endpoint.Address, retInst.Endpoint.Address)
	assert.Equal(t, gtInst.Endpoint.Port, retInst.Endpoint.Port)
	ComparePorts(t, gtInst.Endpoint.ServicePort, retInst.Endpoint.ServicePort)

	if !gtInst.Tags.Equals(retInst.Tags) {
		t.Fatal("Instance Tags are not the same")
	}
}

func CompareServices(t *testing.T, gtSvcs, retSvcs Services) {
	assert.Equal(t, len(gtSvcs), len(retSvcs))
	for idx, gtSvc := range gtSvcs {
		retSvc := retSvcs[idx]
		CompareService(t, gtSvc, retSvc)
	}
}

func CompareService(t *testing.T, gtSvc, retSvc *model.Service) {
	assert.Equal(t, gtSvc.Hostname, retSvc.Hostname)
	assert.Equal(t, gtSvc.Address, retSvc.Address)
	assert.Equal(t, gtSvc.ExternalName, retSvc.ExternalName)

	for idx, gp := range gtSvc.Ports {
		rp := retSvc.Ports[idx]
		ComparePorts(t, gp, rp)
	}
}

func ComparePorts(t *testing.T, gp, rp *model.Port) {
	assert.Equal(t, gp.Name, rp.Name)
	assert.Equal(t, gp.Port, rp.Port)
	assert.Equal(t, gp.Protocol, rp.Protocol)
}
