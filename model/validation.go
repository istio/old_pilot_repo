// Copyright 2017 Google Inc.
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

package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"

	multierror "github.com/hashicorp/go-multierror"

	proxyconfig "istio.io/manager/model/proxy/alphav1/config"
)

const (
	dns1123LabelMaxLength int    = 63
	dns1123LabelFmt       string = "[a-z0-9]([-a-z0-9]*[a-z0-9])?"
	// TODO: there is a stricter regex for the labels from validation.go in k8s
	qualifiedNameFmt string = "[-A-Za-z0-9_./]*"
)

var (
	dns1123LabelRex = regexp.MustCompile("^" + dns1123LabelFmt + "$")
	kindRegexp      = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9]*$")
	tagRegexp       = regexp.MustCompile("^" + qualifiedNameFmt + "$")
)

// IsDNS1123Label tests for a string that conforms to the definition of a label in
// DNS (RFC 1123).
func IsDNS1123Label(value string) bool {
	return len(value) <= dns1123LabelMaxLength && dns1123LabelRex.MatchString(value)
}

// Validate confirms that the names in the configuration key are appropriate
func (k *ConfigKey) Validate() error {
	var errs error
	if !kindRegexp.MatchString(k.Kind) {
		errs = multierror.Append(errs, fmt.Errorf("Invalid kind: %q", k.Kind))
	}
	if !IsDNS1123Label(k.Name) {
		errs = multierror.Append(errs, fmt.Errorf("Invalid name: %q", k.Name))
	}
	if !IsDNS1123Label(k.Namespace) {
		errs = multierror.Append(errs, fmt.Errorf("Invalid namespace: %q", k.Namespace))
	}
	return errs
}

// Validate checks that each name conforms to the spec and has a ProtoMessage
func (km KindMap) Validate() error {
	var errs error
	for k, v := range km {
		if !kindRegexp.MatchString(k) {
			errs = multierror.Append(errs, fmt.Errorf("Invalid kind: %q", k))
		}
		if proto.MessageType(v.MessageName) == nil {
			errs = multierror.Append(errs, fmt.Errorf("Cannot find proto message type: %q", v.MessageName))
		}
	}
	return errs
}

// ValidateKey ensures that the key is well-defined and kind is well-defined
func (km KindMap) ValidateKey(k *ConfigKey) error {
	if err := k.Validate(); err != nil {
		return err
	}
	if _, ok := km[k.Kind]; !ok {
		return fmt.Errorf("Kind %q is not defined", k.Kind)
	}
	return nil
}

// ValidateConfig ensures that the config object is well-defined
func (km KindMap) ValidateConfig(obj *Config) error {
	if obj == nil {
		return fmt.Errorf("Invalid nil configuration object")
	}

	if err := obj.ConfigKey.Validate(); err != nil {
		return err
	}
	t, ok := km[obj.Kind]
	if !ok {
		return fmt.Errorf("Undeclared kind: %q", obj.Kind)
	}

	// Validate spec field
	if obj.Spec == nil {
		return fmt.Errorf("Want a proto message, received empty content")
	}
	v, ok := obj.Spec.(proto.Message)
	if !ok {
		return fmt.Errorf("Cannot cast spec to a proto message")
	}
	if proto.MessageName(v) != t.MessageName {
		return fmt.Errorf("Mismatched spec message type %q and kind %q",
			proto.MessageName(v), t.MessageName)
	}
	if err := t.Validate(v); err != nil {
		return err
	}

	// Validate status field
	if obj.Status != nil {
		v, ok := obj.Status.(proto.Message)
		if !ok {
			return fmt.Errorf("Cannot cast status to a proto message")
		}
		if proto.MessageName(v) != t.StatusMessageName {
			return fmt.Errorf("Mismatched status message type %q and kind %q",
				proto.MessageName(v), t.StatusMessageName)
		}
	}

	return nil
}

// Validate ensures that the service object is well-defined
func (s *Service) Validate() error {
	var errs error
	if len(s.Hostname) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("Invalid empty hostname"))
	}
	parts := strings.Split(s.Hostname, ".")
	for _, part := range parts {
		if !IsDNS1123Label(part) {
			errs = multierror.Append(errs, fmt.Errorf("Invalid hostname part: %q", part))
		}
	}

	for _, tag := range s.Tags {
		if err := tag.Validate(); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	// Require at least one port
	if len(s.Ports) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("Service must have at least one declared port"))
	}

	// Port names can be empty if there exists only one port
	for _, port := range s.Ports {
		if port.Name == "" {
			if len(s.Ports) > 1 {
				errs = multierror.Append(errs,
					fmt.Errorf("Empty port names are not allowed for services with multiple ports"))
			}
		} else if !IsDNS1123Label(port.Name) {
			errs = multierror.Append(errs, fmt.Errorf("Invalid name: %q", port.Name))
		}
		if port.Port < 0 {
			errs = multierror.Append(errs, fmt.Errorf("Invalid service port value %d for %q", port.Port, port.Name))
		}
	}
	return errs
}

// Validate ensures tag is well-formed
func (t Tag) Validate() error {
	var errs error
	if len(t) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("Tag must have at least one key-value pair"))
	}
	for k, v := range t {
		if !tagRegexp.MatchString(k) {
			errs = multierror.Append(errs, fmt.Errorf("Invalid tag key: %q", k))
		}
		if !tagRegexp.MatchString(v) {
			errs = multierror.Append(errs, fmt.Errorf("Invalid tag value: %q", v))
		}
	}
	return errs
}

// ValidateProxyConfig ensures that the ProxyConfig posted by the user is well-formed
func ValidateProxyConfig(o proto.Message) error {
	newConfig := &proxyconfig.ProxyConfig{}

	if data, err := proto.Marshal(o); err != nil {
		return err
	}

	if err = proto.Unmarshal(data, newConfig); err != nil {
		return err
	}

	var errs error

	if len(newConfig.RouteRules) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("ProxyConfig must have atleast one route rule"))
	} else {
		for _, rule := range newConfig.RouteRules {
			if rule.GetLayer4() != nil {
				errs = multierror.Append(errs, fmt.Errorf("Layer4 route rules are not supported yet"))
			}
		}
	}

	if len(newConfig.UpstreamClusters) == 0 {
		errs = multierror.Append(errs, fmt.Errorf("ProxyConfig must have atleast one upstream cluster"))
	}

	//TODO: validate rest of proxy config
	return errs
}
