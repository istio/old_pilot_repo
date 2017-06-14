package vms

import(
	"strings"
	"github.com/amalgam8/amalgam8/registry/client/test/model"
	"github.com/amalgam8/amalgam8/pkg/api"
)

const()

func convertService(svc *api.Service) *model.Service {
	return &model.Service {
		Hostname: svc.ServiceName,
		Address: svc.Address,
		Ports: convertPortList(svc.Ports),
		ExternalName: svc.ExternalName,
	}
}

func convertPortList(pl api.PortList) model.PortList {
	if pl == nil {return nil}

	out := make(model.PortList, len(pl), len(pl))

	for idx, port :=range pl {
		out[idx] = convertPort(port)
	}

	return out
}

func convertPort(port *api.Port) *model.Port {
	out := &model.Port{
		Name: port.Name,
		Port: port.Port,
		Protocol: convertProtocol(port.Protocol),
	}
	return out
}

func convertProtocol(proto string) model.Protocol {
	var out model.Protocol
	switch strings.ToLower(proto) {
		case "grpc":
			out = model.ProtocolGRPC
		case "https":
			out = model.ProtocolHTTPS
		case "http2":
			out = model.ProtocolHTTP2
		case "http":
			out = model.ProtocolHTTP
		case "tcp":
			out = model.ProtocolTCP
		case "udp":
			out = model.ProtocolUDP
	}

	return out
}

func convertTags(tagList []string) model.Tags {
	tags := make(model.Tags)
	for _, tag := range tagList {
		keyVal := strings.Split(tag, ":")
		if len(keyVal) != 2 {continue}
		tags[keyVal[0]] = keyVal[1]
	}

	return tags
}
