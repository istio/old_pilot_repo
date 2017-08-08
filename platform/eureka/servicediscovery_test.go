package eureka

import (
	"testing"

	"istio.io/pilot/model"
)

type mockClient []*application

func (apps *mockClient) Applications() ([]*application, error) {
	return *apps, nil
}

var _ Client = (*mockClient)(nil)

func TestServiceDiscoveryServices(t *testing.T) {
	cl := &mockClient{
		{
			Name: appName("a.default.svc.local"),
			Instances: []*instance{
				makeInstance("a.default.svc.local", "10.0.0.1", 9090, 8080, nil),
				makeInstance("b.default.svc.local", "10.0.0.2", 7070, -1, nil),
			},
		},
	}
	sd := NewServiceDiscovery(cl)
	services := []*model.Service{
		makeService("a.default.svc.local", []int{9090, 8080}, nil),
		makeService("b.default.svc.local", []int{7070}, nil),
	}

	if err := compare(t, sd.Services(), services); err != nil {
		t.Error(err)
	}
}

func TestServiceDiscoveryGetService(t *testing.T) {
	hostA := "a.default.svc.local"
	hostB := "b.default.svc.local"
	hostC := "c.default.svc.local"

	cl := &mockClient{
		{
			Name: appName(hostA),
			Instances: []*instance{
				makeInstance(hostA, "10.0.0.1", 9090, 8080, nil),
				makeInstance(hostB, "10.0.0.2", 7070, -1, nil),
			},
		},
	}
	sd := NewServiceDiscovery(cl)

	_, exists := sd.GetService(hostC)
	if exists {
		t.Errorf("GetService() retrieved non-existent service %s", hostC)
	}

	expected := makeService(hostA, []int{9090, 8080}, nil)
	actual, exists := sd.GetService(hostA)
	if !exists {
		t.Errorf("GetService() could not retrieve %s", hostA)
	}

	if err := compare(t, actual, expected); err != nil {
		t.Error(err)
	}
}

func TestServiceDiscoveryHostInstances(t *testing.T) {
	cl := &mockClient{
		{
			Name: appName("a.default.svc.local"),
			Instances: []*instance{
				makeInstance("a.default.svc.local", "10.0.0.1", 9090, -1, nil),
				makeInstance("a.default.svc.local", "10.0.0.2", 8080, -1, nil),
				makeInstance("b.default.svc.local", "10.0.0.1", 7070, -1, nil),
			},
		},
	}
	sd := NewServiceDiscovery(cl)

	serviceA := makeService("a.default.svc.local", []int{9090, 8080}, nil)
	serviceB := makeService("b.default.svc.local", []int{7070}, nil)

	instanceTests := []struct {
		addrs     map[string]bool
		instances []*model.ServiceInstance
	}{
		{
			addrs: map[string]bool{
				"10.0.0.1": true,
			},
			instances: []*model.ServiceInstance{
				makeServiceInstance(serviceA, "10.0.0.1", 9090, nil),
				makeServiceInstance(serviceB, "10.0.0.1", 7070, nil),
			},
		},
	}

	for _, tt := range instanceTests {
		if err := compare(t, sd.HostInstances(tt.addrs), tt.instances); err != nil {
			t.Error(err)
		}
	}
}

func TestServiceDiscoveryInstances(t *testing.T) {
	cl := &mockClient{
		{
			Name: appName("a.default.svc.local"),
			Instances: []*instance{
				makeInstance("a.default.svc.local", "10.0.0.1", 9090, -1, metadata{"spam": "coolaid"}),
				makeInstance("a.default.svc.local", "10.0.0.2", 8080, -1, metadata{"kit": "kat"}),
				makeInstance("b.default.svc.local", "10.0.0.1", 7070, -1, nil),
			},
		},
	}
	sd := NewServiceDiscovery(cl)
	serviceA := makeService("a.default.svc.local", []int{9090, 8080}, nil)
	serviceB := makeService("b.default.svc.local", []int{7070}, nil)
	spamCoolaidTags := model.Tags{"spam": "coolaid"}
	kitKatTags := model.Tags{"kit": "kat"}

	serviceInstanceTests := []struct {
		hostname  string
		ports     []string
		tags      model.TagsList
		instances []*model.ServiceInstance
	}{
		{
			// filter by hostname
			hostname: "a.default.svc.local",
			instances: []*model.ServiceInstance{
				makeServiceInstance(serviceA, "10.0.0.1", 9090, spamCoolaidTags),
				makeServiceInstance(serviceA, "10.0.0.2", 8080, kitKatTags),
			},
		},
		{
			// filter by hostname and tags
			hostname: "a.default.svc.local",
			tags:     model.TagsList{{"spam": "coolaid"}},
			instances: []*model.ServiceInstance{
				makeServiceInstance(serviceA, "10.0.0.1", 9090, spamCoolaidTags),
			},
		},
		{
			// filter by hostname and port
			hostname: "b.default.svc.local",
			ports:    []string{"7070"},
			instances: []*model.ServiceInstance{
				makeServiceInstance(serviceB, "10.0.0.1", 7070, nil),
			},
		},
	}

	for _, c := range serviceInstanceTests {
		instances := sd.Instances(c.hostname, c.ports, c.tags)
		if err := compare(t, instances, c.instances); err != nil {
			t.Error(err)
		}
	}
}
