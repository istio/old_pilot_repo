package eureka

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

func loadJSON(t *testing.T, filename string, out interface{}) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
}

//type MockClient struct {
//	Apps []*Application
//}
//
//func (m *MockClient) Applications() ([]*Application, error) {
//	return m.Apps, nil
//}
//
//var _ Client = (*MockClient)(nil)
//
//func MakeRegistry(t *testing.T) *serviceDiscovery {
//	var apps GetApplications
//
//	loadJSON(t, "testdata/eureka-apps.json", &apps)
//
//	cl := &MockClient{Apps: apps.Applications.Applications}
//
//	return &serviceDiscovery{client: cl}
//}
//
//func TestControllerServices(t *testing.T) {
//	r := MakeRegistry(t)
//
//	svcs := r.Services()
//	expectedSvcs := []*model.Service{
//		{
//			Hostname: "a.default.svc.local",
//			Ports: model.PortList{
//				{
//					Name:     "9090",
//					Port:     9090,
//					Protocol: model.ProtocolTCP,
//				},
//				{
//					Name:     "8080",
//					Port:     8080,
//					Protocol: model.ProtocolTCP,
//				},
//			},
//		},
//		{
//			Hostname: "b.default.svc.local",
//			Ports: model.PortList{
//				{
//					Name:     "7070",
//					Port:     7070,
//					Protocol: model.ProtocolTCP,
//				},
//			},
//		},
//	}
//
//	if !reflect.DeepEqual(svcs, expectedSvcs) {
//		if err := compare(svcs, expectedSvcs, t); err != nil {
//			t.Error(err)
//		}
//	}
//}
//
//func TestControllerHostInstances(t *testing.T) {
//	r := MakeRegistry(t)
//
//	instanceTests := []struct {
//		Addrs            map[string]bool
//		ServiceInstances []*model.ServiceInstance
//	}{
//		{
//			Addrs: map[string]bool{
//				"172.17.0.12": true,
//			},
//			ServiceInstances: []*model.ServiceInstance{
//				{
//					Endpoint: model.NetworkEndpoint{
//						Address: "172.17.0.12",
//						Port:    7070,
//						ServicePort: &model.Port{
//							Name:     "7070",
//							Port:     7070,
//							Protocol: model.ProtocolTCP,
//						},
//					},
//					Service: &model.Service{
//						Hostname: "b.default.svc.local",
//						Ports: model.PortList{
//							{
//								Name:     "7070",
//								Port:     7070,
//								Protocol: model.ProtocolTCP,
//							},
//						},
//					},
//					Tags: model.Tags{"@class": "java.util.Collections$EmptyMap"}, // TODO: filter out?
//				},
//			},
//		},
//	}
//
//	for _, tt := range instanceTests {
//		if err := compare(r.HostInstances(tt.Addrs), tt.ServiceInstances, t); err != nil {
//			t.Error(err)
//		}
//	}
//}
//
//func TestControllerInstances(t *testing.T) {
//	r := MakeRegistry(t)
//
//	cases := []struct {
//		Hostname         string
//		Ports            []string
//		Tags             model.TagsList
//		ServiceInstances []*model.ServiceInstance
//	}{
//		{ // filter by hostname
//			Hostname: "a.default.svc.local",
//			ServiceInstances: []*model.ServiceInstance{
//				{
//					Endpoint: model.NetworkEndpoint{
//						Address: "172.17.0.10",
//						Port:    9090,
//						ServicePort: &model.Port{
//							Name:     "9090",
//							Port:     9090,
//							Protocol: model.ProtocolTCP,
//						},
//					},
//					Service: &model.Service{
//						Hostname: "a.default.svc.local",
//						Ports: model.PortList{
//							{
//								Name:     "9090",
//								Port:     9090,
//								Protocol: model.ProtocolTCP,
//							},
//							{
//								Name:     "8080",
//								Port:     8080,
//								Protocol: model.ProtocolTCP,
//							},
//						},
//					},
//					Tags: model.Tags{"@class": "java.util.Collections$EmptyMap"}, // TODO: filter out?
//				},
//				{
//					Endpoint: model.NetworkEndpoint{
//						Address: "172.17.0.10",
//						Port:    8080,
//						ServicePort: &model.Port{
//							Name:     "8080",
//							Port:     8080,
//							Protocol: model.ProtocolTCP,
//						},
//					},
//					Service: &model.Service{
//						Hostname: "a.default.svc.local",
//						Ports: model.PortList{
//							{
//								Name:     "9090",
//								Port:     9090,
//								Protocol: model.ProtocolTCP,
//							},
//							{
//								Name:     "8080",
//								Port:     8080,
//								Protocol: model.ProtocolTCP,
//							},
//						},
//					},
//					Tags: model.Tags{"@class": "java.util.Collections$EmptyMap", "spam": "coolaid"}, // TODO: filter out?
//				},
//			},
//		},
//		{ // filter by hostname and tags
//			Hostname: "a.default.svc.local",
//			Tags: model.TagsList{
//				{"spam": "coolaid"},
//			},
//			ServiceInstances: []*model.ServiceInstance{
//				{
//					Endpoint: model.NetworkEndpoint{
//						Address: "172.17.0.10",
//						Port:    8080,
//						ServicePort: &model.Port{
//							Name:     "8080",
//							Port:     8080,
//							Protocol: model.ProtocolTCP,
//						},
//					},
//					Service: &model.Service{
//						Hostname: "a.default.svc.local",
//						Ports: model.PortList{
//							{
//								Name:     "9090",
//								Port:     9090,
//								Protocol: model.ProtocolTCP,
//							},
//							{
//								Name:     "8080",
//								Port:     8080,
//								Protocol: model.ProtocolTCP,
//							},
//						},
//					},
//					Tags: model.Tags{"@class": "java.util.Collections$EmptyMap", "spam": "coolaid"}, // TODO: filter out?
//				},
//			},
//		},
//		{ // filter by hostname and port
//			Hostname: "b.default.svc.local",
//			Ports: []string{"7070"},
//			ServiceInstances: []*model.ServiceInstance{
//				{
//					Endpoint: model.NetworkEndpoint{
//						Address: "172.17.0.12",
//						Port:    7070,
//						ServicePort: &model.Port{
//							Name:     "7070",
//							Port:     7070,
//							Protocol: model.ProtocolTCP,
//						},
//					},
//					Service: &model.Service{
//						Hostname: "b.default.svc.local",
//						Ports: model.PortList{
//							{
//								Name:     "7070",
//								Port:     7070,
//								Protocol: model.ProtocolTCP,
//							},
//						},
//					},
//					Tags: model.Tags{"@class": "java.util.Collections$EmptyMap"}, // TODO: filter out?
//				},
//				{
//					Endpoint: model.NetworkEndpoint{
//						Address: "172.17.0.13",
//						Port:    7070,
//						ServicePort: &model.Port{
//							Name:     "7070",
//							Port:     7070,
//							Protocol: model.ProtocolTCP,
//						},
//					},
//					Service: &model.Service{
//						Hostname: "b.default.svc.local",
//						Ports: model.PortList{
//							{
//								Name:     "7070",
//								Port:     7070,
//								Protocol: model.ProtocolTCP,
//							},
//						},
//					},
//					Tags: model.Tags{"@class": "java.util.Collections$EmptyMap"}, // TODO: filter out?
//				},
//			},
//		},
//	}
//
//	for _, c := range cases {
//		instances := r.Instances(c.Hostname, c.Ports, c.Tags)
//		if err := compare(instances, c.ServiceInstances, t); err != nil {
//			t.Error(err)
//		}
//		//if !reflect.DeepEqual(c.ServiceInstances, instances) {
//		//	t.Errorf("Wanted -> \n%v\nGot -> \n%v", prettySprint(c.ServiceInstances), prettySprint(instances))
//		//}
//	}
//}

