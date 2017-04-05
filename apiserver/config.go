package apiserver

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"

	"istio.io/manager/model"
)

// Config is the complete configuration including a parsed spec
type Config struct {
	// Type SHOULD be one of the kinds in model.IstioConfig; a route-rule, ingress-rule, or destination-policy
	Type string      `json:"type,omitempty"`
	Name string      `json:"name,omitempty"`
	Spec interface{} `json:"spec,omitempty"`
	// ParsedSpec will be one of the messages in model.IstioConfig: for example an
	// istio.proxy.v1alpha.config.RouteRule or DestinationPolicy
	ParsedSpec proto.Message `json:"-"`
}

func (c *Config) parseSpec() error {

	byteSpec, err := json.Marshal(c.Spec)
	if err != nil {
		return fmt.Errorf("could not encode Spec: %v", err)
	}
	schema, ok := model.IstioConfig[c.Type]
	if !ok {
		return fmt.Errorf("unknown spec type %s", c.Type)
	}
	message, err := schema.FromJSON(string(byteSpec))
	if err != nil {
		return fmt.Errorf("cannot parse proto message: %v", err)
	}
	c.ParsedSpec = message
	glog.V(2).Info(fmt.Sprintf("Parsed %v %v into %v %v", c.Type, c.Name, schema.MessageName, message))
	return nil
}
