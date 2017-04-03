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

package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"

	multierror "github.com/hashicorp/go-multierror"

	proxyconfig "istio.io/api/proxy/v1/config"
)

const (
	dns1123LabelMaxLength int    = 63
	dns1123LabelFmt       string = "[a-z0-9]([-a-z0-9]*[a-z0-9])?"
	// TODO: there is a stricter regex for the labels from validation.go in k8s
	qualifiedNameFmt string = "[-A-Za-z0-9_./]*"
)

var (
	dns1123LabelRex = regexp.MustCompile("^" + dns1123LabelFmt + "$")
	tagRegexp       = regexp.MustCompile("^" + qualifiedNameFmt + "$")
)

// IsDNS1123Label tests for a string that conforms to the definition of a label in
// DNS (RFC 1123).
func IsDNS1123Label(value string) bool {
	return len(value) <= dns1123LabelMaxLength && dns1123LabelRex.MatchString(value)
}

// Validate confirms that the names in the configuration key are appropriate
func (k *Key) Validate() error {
	var errs error
	if !IsDNS1123Label(k.Kind) {
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
		if !IsDNS1123Label(k) {
			errs = multierror.Append(errs, fmt.Errorf("Invalid kind: %q", k))
		}
		if proto.MessageType(v.MessageName) == nil {
			errs = multierror.Append(errs, fmt.Errorf("Cannot find proto message type: %q", v.MessageName))
		}
	}
	return errs
}

// ValidateKey ensures that the key is well-defined and kind is well-defined
func (km KindMap) ValidateKey(k *Key) error {
	if err := k.Validate(); err != nil {
		return err
	}
	if _, ok := km[k.Kind]; !ok {
		return fmt.Errorf("Kind %q is not defined", k.Kind)
	}
	return nil
}

// ValidateConfig ensures that the config object is well-defined
func (km KindMap) ValidateConfig(k *Key, obj interface{}) error {
	if k == nil || obj == nil {
		return fmt.Errorf("Invalid nil configuration object")
	}

	if err := k.Validate(); err != nil {
		return err
	}
	t, ok := km[k.Kind]
	if !ok {
		return fmt.Errorf("Undeclared kind: %q", k.Kind)
	}

	v, ok := obj.(proto.Message)
	if !ok {
		return fmt.Errorf("Cannot cast to a proto message")
	}
	if proto.MessageName(v) != t.MessageName {
		return fmt.Errorf("Mismatched message type %q and kind %q",
			proto.MessageName(v), t.MessageName)
	}
	if err := t.Validate(v); err != nil {
		return err
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

// Validate ensures that the service instance is well-defined
func (instance *ServiceInstance) Validate() error {
	var errs error
	if instance.Service == nil {
		errs = multierror.Append(errs, fmt.Errorf("Missing service in the instance"))
	} else if err := instance.Service.Validate(); err != nil {
		errs = multierror.Append(errs, err)
	}

	if err := instance.Tags.Validate(); err != nil {
		errs = multierror.Append(errs, err)
	}

	if instance.Endpoint.Port < 0 {
		errs = multierror.Append(errs, fmt.Errorf("Negative port value: %d", instance.Endpoint.Port))
	}

	port := instance.Endpoint.ServicePort
	if port == nil {
		errs = multierror.Append(errs, fmt.Errorf("Missing service port"))
	} else if instance.Service != nil {
		expected, ok := instance.Service.Ports.Get(port.Name)
		if !ok {
			errs = multierror.Append(errs, fmt.Errorf("Missing service port %q", port.Name))
		} else {
			if expected.Port != port.Port {
				errs = multierror.Append(errs,
					fmt.Errorf("Unexpected service port value %d, expected %d", port.Port, expected.Port))
			}
			if expected.Protocol != port.Protocol {
				errs = multierror.Append(errs,
					fmt.Errorf("Unexpected service protocol %s, expected %s", port.Protocol, expected.Protocol))
			}
		}
	}

	return errs
}

// Validate ensures tag is well-formed
func (t Tags) Validate() error {
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

func validateFQDN(fqdn string) error {
	if len(fqdn) > 255 {
		return fmt.Errorf("Domain name too long (max 255)")
	}

	for _, label := range strings.Split(fqdn, ".") {
		if !IsDNS1123Label(label) {
			return fmt.Errorf("Domain name %q invalid", fqdn)
		}
	}

	return nil
}

func ValidateMatchCondition(mc *proxyconfig.MatchCondition) error {
	var retVal error

	if mc.Source != "" {
		if err := validateFQDN(mc.Source); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	// We do not validate source_tags because they have no explicit rules

	if mc.GetTcp() != nil {
		if err := ValidateL4MatchAttributes(mc.GetTcp()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if mc.GetUdp() != nil {
		if err := ValidateL4MatchAttributes(mc.GetUdp()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	// We do not (yet) validate http_headers.

	return retVal
}

func ValidateL4MatchAttributes(ma *proxyconfig.L4MatchAttributes) error {
	var retVal error

	if ma.SourceSubnet != nil {
		for _, subnet := range ma.SourceSubnet{
			if err := validateSubnet(subnet); err != nil {
				retVal = multierror.Append(retVal, err)
			}
		}
	}

	if ma.DestinationSubnet != nil {
		for _, subnet := range ma.DestinationSubnet {
			if err := validateSubnet(subnet); err != nil {
				retVal = multierror.Append(retVal, err)
			}
		}
	}

	return retVal
}

func ValidateDestinationWeight(dw *proxyconfig.DestinationWeight) error {
	var retVal error

	if dw.Destination != "" {
		if err := validateFQDN(dw.Destination); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	// We do not validate tags because they have no explicit rules

	if dw.Weight > 100 {
		retVal = multierror.Append(retVal, fmt.Errorf("weight must not exceed 100"))
	}
	if dw.Weight < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("weight must be in range 0..100"))
	}

	return retVal
}

func ValidateHttpTimeout(timeout *proxyconfig.HTTPTimeout) error {
	var retVal error

	if simple := timeout.GetSimpleTimeout(); simple != nil {
		if simple.TimeoutSeconds < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("timeout_seconds must be in range 0.."))
		}

		// We ignore override_header_name
	}

	return retVal
}

func ValidateHttpRetries(retry *proxyconfig.HTTPRetry) error {
	var retVal error

	if simple := retry.GetSimpleRetry(); simple != nil {
		if simple.Attempts < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("attempts must be in range 0.."))
		}

		// We ignore override_header_name
	}

	return retVal
}

func ValidateHttpFault(fault *proxyconfig.HTTPFaultInjection) error {
	var retVal error

	if fault.GetDelay() != nil {
		if err := validateDelay(fault.GetDelay()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if fault.GetAbort() != nil {
		if err := validateAbort(fault.GetAbort()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	return retVal
}

func ValidateL4Fault(fault *proxyconfig.L4FaultInjection) error {
	var retVal error

	if fault.GetTerminate() != nil {
		if err := validateTerminate(fault.GetTerminate()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if fault.GetThrottle() != nil {
		if err := validateThrottle(fault.GetThrottle()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	return retVal
}

func validateSubnet(subnet string) error {
	var retVal error

	// TODO verify subnet is "IPv4 or IPv6 ip address with optional subnet. E.g., a.b.c.d/xx form or just a.b.c.d

	return retVal
}

func validateDelay(delay *proxyconfig.HTTPFaultInjection_Delay) error {
	var retVal error

	if delay.Percent > 100 {
		retVal = multierror.Append(retVal, fmt.Errorf("delay percent must not exceed 100"))
	}
	if delay.Percent < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("delay percent must be in range 0..100"))
	}

	if delay.GetFixedDelaySeconds() < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("delay fixed_seconds invalid"))
	}

	if delay.GetExponentialDelaySeconds() < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("delay exponential_seconds invalid"))
	}

	return retVal
}

func validateAbort(abort *proxyconfig.HTTPFaultInjection_Abort) error {
	var retVal error

	if abort.Percent > 100 {
		retVal = multierror.Append(retVal, fmt.Errorf("abort percent must not exceed 100"))
	}
	if abort.Percent < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("abort percent must be in range 0..100"))
	}

	// No validation yet for grpc_status / http2_error / http_status
	// No validation yet for override_header_name

	return retVal
}

func validateTerminate(terminate *proxyconfig.L4FaultInjection_Terminate) error {
	var retVal error

	if terminate.Percent > 100 {
		retVal = multierror.Append(retVal, fmt.Errorf("terminate percent must not exceed 100"))
	}
	if terminate.Percent < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("terminate percent must be in range 0..100"))
	}

	return retVal
}

func validateThrottle(throttle *proxyconfig.L4FaultInjection_Throttle) error {
	var retVal error

	if throttle.Percent > 100 {
		retVal = multierror.Append(retVal, fmt.Errorf("throttle percent must not exceed 100"))
	}
	if throttle.Percent < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("throttle percent must be in range 0..100"))
	}

	if throttle.DownstreamLimitBps < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("downstream_limit_bps invalid"))
	}

	if throttle.UpstreamLimitBps < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("upstream_limit_bps invalid"))
	}

	if throttle.GetThrottleAfterSeconds() < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("throttle_after_seconds invalid"))
	}

	if throttle.GetThrottleAfterBytes() < 0 {
		retVal = multierror.Append(retVal, fmt.Errorf("throttle_after_bytes invalid"))
	}

	// TODO Check DoubleValue throttle.GetThrottleForSeconds()

	return retVal
}

func ValidateLoadBalancing(lb *proxyconfig.LoadBalancing) error {
	var retVal error

	// Currently the policy is just a name, and we don't validate it

	return retVal
}

func ValidateCircuitBreaker(cb *proxyconfig.CircuitBreaker) error {
	var retVal error

	if simple := cb.GetSimpleCb(); simple != nil {
		if simple.MaxConnections < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker max_connections must be in range 0.."))
		}
		if simple.HttpMaxPendingRequests < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker max_pending_requests must be in range 0.."))
		}
		if simple.HttpMaxRequests < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker max_requests must be in range 0.."))
		}
		if simple.SleepWindowSeconds < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker sleep_window_seconds must be in range 0.."))
		}
		if simple.HttpConsecutiveErrors < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker http_consecutive_errors must be in range 0.."))
		}
		if simple.HttpDetectionIntervalSeconds < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker http_detection_interval_seconds must be in range 0.."))
		}
		if simple.HttpMaxRequestsPerConnection < 0 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker http_max_requests_per_connection must be in range 0.."))
		}
		if simple.HttpMaxEjectionPercent < 0 || simple.HttpMaxEjectionPercent > 100 {
			retVal = multierror.Append(retVal, fmt.Errorf("circuit_breaker http_max_ejection_percent must be in range 0..100"))
		}
	}

	return retVal
}

// ValidateRouteRule checks routing rules
func ValidateRouteRule(msg proto.Message) error {

	value, ok := msg.(*proxyconfig.RouteRule)
	if !ok {
		return fmt.Errorf("cannot cast to routing rule")
	}
	var retVal error
	if value.Destination == "" {
		retVal = multierror.Append(retVal, fmt.Errorf("routeRule must have a destination service"))
	}
	if err := validateFQDN(value.Destination); err != nil {
		retVal = multierror.Append(retVal, err)
	}

	// We don't validate precedence because any int32 is legal

	if value.GetMatch() != nil {
		if err := ValidateMatchCondition(value.GetMatch()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if value.GetRoute() != nil {
		for _, destWeight := range value.GetRoute() {
			if err := ValidateDestinationWeight(destWeight); err != nil {
				retVal = multierror.Append(retVal, err)
			}
		}
	}

	if value.GetHttpReqTimeout() != nil {
		if err := ValidateHttpTimeout(value.GetHttpReqTimeout()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if value.GetHttpReqRetries() != nil {
		if err := ValidateHttpRetries(value.GetHttpReqRetries()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if value.GetHttpFault() != nil {
		if err := ValidateHttpFault(value.GetHttpFault()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if value.GetL4Fault() != nil {
		if err := ValidateL4Fault(value.GetL4Fault()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	return retVal
}

// ValidateIngressRule checks ingress rules
func ValidateIngressRule(msg proto.Message) error {
	// TODO: Add ingress-only validation checks, if any?
	return ValidateRouteRule(msg)
}

// ValidateDestinationPolicy checks proxy policies
func ValidateDestinationPolicy(msg proto.Message) error {
	value, ok := msg.(*proxyconfig.DestinationPolicy)
	if !ok {
		return fmt.Errorf("Cannot cast to destination policy")
	}

	var retVal error

	if value.Destination == "" {
		retVal = multierror.Append(retVal, fmt.Errorf("destinationPolicy should have a valid service name in its destination field"))
	} else {
		if err := validateFQDN(value.Destination); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	// We do not validate tags because they have no explicit rules

	if value.GetLoadBalancing() != nil {
		if err := ValidateLoadBalancing(value.GetLoadBalancing()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	if value.GetCircuitBreaker() != nil {
		if err := ValidateCircuitBreaker(value.GetCircuitBreaker()); err != nil {
			retVal = multierror.Append(retVal, err)
		}
	}

	return retVal
}
