package eureka

import (
	"testing"

	"fmt"
	"strings"

	"istio.io/pilot/model"
	"encoding/json"
	"istio.io/pilot/test/util"
)

func TestConvertService(t *testing.T) {
	serviceTests := []struct {
		apps     []*application
		services map[string]*model.Service
	}{
		{
			// single instance with multiple ports
			apps: []*application{
				{
					Name:      "foo_bar_local",
					Instances: []*instance{makeInstance("foo.bar.local", "10.0.0.1", 5000, 5443)},
				},
			},
			services: map[string]*model.Service{
				"foo.bar.local": makeService("foo.bar.local", []int{5000, 5443}, nil),
			},
		},
		{
			// multi-instance with different IPs
			apps: []*application{
				{
					Name: "foo_bar_local",
					Instances: []*instance{
						makeInstance("foo.bar.local", "10.0.0.1", 5000, -1),
						makeInstance("foo.bar.local", "10.0.0.2", 5000, -1),
					},
				},
			},
			services: map[string]*model.Service{
				"foo.bar.local": makeService("foo.bar.local", []int{5000}, nil),
			},
		},
		{
			// multi-instance with different IPs, ports
			apps: []*application{
				{
					Name: "foo_bar_local",
					Instances: []*instance{
						makeInstance("foo.bar.local", "10.0.0.1", 5000, -1),
						makeInstance("foo.bar.local", "10.0.0.1", 6000, -1),
					},
				},
			},
			services: map[string]*model.Service{
				"foo.bar.local": makeService("foo.bar.local", []int{5000, 6000}, nil),
			},
		},
		{
			// multi-application with the same hostname
			apps: []*application{
				{
					Name: "foo_bar_local",
					Instances: []*instance{
						makeInstance("foo.bar.local", "10.0.0.1", 5000, -1),
					},
				},
				{
					Name: "foo_bar_local2",
					Instances: []*instance{
						makeInstance("foo.bar.local", "10.0.0.2", 5000, -1),
					},
				},
			},
			services: map[string]*model.Service{
				"foo.bar.local": makeService("foo.bar.local", []int{5000}, nil),
			},
		},
		{
			// multi-application with different hostnames
			apps: []*application{
				{
					Name: "foo_bar_local",
					Instances: []*instance{
						makeInstance("foo.bar.local", "10.0.0.1", 5000, -1),
					},
				},
				{
					Name: "foo_biz_local",
					Instances: []*instance{
						makeInstance("foo.biz.local", "10.0.0.2", 5000, -1),
					},
				},
			},
			services: map[string]*model.Service{
				"foo.bar.local": makeService("foo.bar.local", []int{5000}, nil),
				"foo.biz.local": makeService("foo.biz.local", []int{5000}, nil),
			},
		},
	}

	for _, tt := range serviceTests {
		services := convertServices(tt.apps, nil)
		if err := compare(services, tt.services, t); err != nil {
			t.Error(err)
		}
	}

	hostnameTests := []map[string]bool{
		map[string]bool{"foo.bar.local": true},
		map[string]bool{"foo.biz.local": true},
		map[string]bool{"foo.bar.local": true, "foo.biz.local": true},
	}
	for _, tt := range serviceTests {
		for _, hostnames := range hostnameTests {
			services := convertServices(tt.apps, hostnames)
			for _, service := range services {
				if !hostnames[service.Hostname] {
					t.Errorf("convert services did not filter hostname %q", service.Hostname)
				}
			}
		}
	}
}

func TestConvertServiceInstances(t *testing.T) {
	foobarService := makeService("foo.bar.local", []int{5000, 5443}, nil)

	serviceInstanceTests := []struct {
		services map[string]*model.Service
		apps     []*application
		out      []*model.ServiceInstance
	}{
		{
			services: map[string]*model.Service{
				"foo.bar.local": foobarService,
			},
			apps: []*application{
				{
					Name: "foo_bar_local",
					Instances: []*instance{
						makeInstance("foo.bar.local", "10.0.0.1", 5000, 5443),
						makeInstance("foo.bar.local", "10.0.0.2", 5000, -1),
					},
				},
			},
			out: []*model.ServiceInstance{
				makeServiceInstance(foobarService, "10.0.0.1", 5000, nil),
				makeServiceInstance(foobarService, "10.0.0.1", 5443, nil),
				makeServiceInstance(foobarService, "10.0.0.2", 5000, nil),
			},
		},
	}

	for _, tt := range serviceInstanceTests {
		instances := convertServiceInstances(tt.services, tt.apps)
		if err := compare(instances, tt.out, t); err != nil {
			t.Error(err)
		}
	}
}

func TestConvertProtocol(t *testing.T) {
	protocolTests := []struct {
		in  metadata
		out model.Protocol
	}{
		{in: nil, out: model.ProtocolTCP},
		{in: metadata{protocolMetadata: "", "kit": "kat"}, out: model.ProtocolTCP},
		{in: metadata{protocolMetadata: "HTCPCP", "kit": "kat", "roast": "dark"}, out: model.ProtocolTCP},
		{in: metadata{protocolMetadata: metadataHTTP, "kit": "kat"}, out: model.ProtocolHTTP},
		{in: metadata{protocolMetadata: metadataHTTP2, "kit": "kat"}, out: model.ProtocolHTTP2},
	}

	for _, tt := range protocolTests {
		if protocol := convertProtocol(tt.in); protocol != tt.out {
			t.Errorf("convertProtocol(%q) => %q, want %q", tt.in, protocol, tt.out)
		}
	}
}

func TestConvertTags(t *testing.T) {
	md := metadata{
		"@class":         "java.util.Collections$EmptyMap",
		protocolMetadata: metadataHTTP2,
		"kit":            "kat",
		"spam":           "coolaid",
	}
	tags := convertTags(md)

	for _, special := range []string{protocolMetadata, "@class"} {
		if _, exists := tags[special]; exists {
			t.Errorf("convertTags did not filter out special tag %q", special)
		}
	}

	for _, tag := range []string{"kit", "spam"} {
		_, exists := tags[tag]
		if !exists {
			t.Errorf("converted tags has missing key %q", tag)
		} else if tags[tag] != md[tag] {
			t.Errorf("converted tags has mismatch for key %q, &q want %q", tag, tags[tag], md[tag])
		}
	}

	if len(tags) != 2 {
		t.Errorf("converted tags has length %d, want %d", len(tags), 2)
	}
}

func appName(hostname string) string {
	return strings.ToUpper(strings.Replace(hostname, ".", "_", -1))
}

func makeInstance(hostname, ip string, portNum, securePort int) *instance {
	inst := &instance{
		App:       appName(hostname),
		Hostname:  hostname,
		IPAddress: ip,
	}
	if portNum > 0 {
		inst.Port = &port{
			Port:    portNum,
			Enabled: true,
		}
	}
	if securePort > 0 {
		inst.SecurePort = &port{
			Port:    securePort,
			Enabled: true,
		}
	}
	return inst
}

func makeService(hostname string, ports []int, protocols []model.Protocol) *model.Service {
	portList := make(model.PortList, 0, len(ports))
	for i, port := range ports {
		protocol := model.ProtocolTCP
		if i < len(protocols) {
			protocol = protocols[i]
		}

		portList = append(portList, &model.Port{
			Name:     fmt.Sprint(port),
			Port:     port,
			Protocol: protocol,
		})
	}

	return &model.Service{
		Hostname: hostname,
		Ports:    portList,
	}
}

func makeServiceInstance(service *model.Service, ip string, port int, tags model.Tags) *model.ServiceInstance {
	servicePort, _ := service.Ports.GetByPort(port)
	return &model.ServiceInstance{
		Endpoint: model.NetworkEndpoint{
			Address:     ip,
			Port:        port,
			ServicePort: servicePort,
		},
		Service:          service,
		Tags:             tags,
		AvailabilityZone: "",
	}
}

func compare(actual, expected interface{}, t *testing.T) error {
	return util.Compare(jsonBytes(actual, t), jsonBytes(expected, t))
}

func jsonBytes(v interface{}, t *testing.T) []byte {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(t)
	}
	return data
}
