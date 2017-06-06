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
	"sort"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"

	proxyconfig "istio.io/api/proxy/v1/config"
)

// ConfigRegistry describes a set of platform agnostic APIs that must be
// supported by the underlying platform to store and retrieve Istio configuration.
//
// The storage registry presented here assumes that the underlying storage
// layer supports GET (list), PUT (add), PATCH (edit) and DELETE semantics
// but does not guarantee any transactional semantics.
//
// The configuration objects can be listed by the configuration type.
//
// "Put", "Post", and "Delete" are mutator operations. These operations are
// asynchronous, and you might not see the effect immediately (e.g. "Get" might
// not return the object by key immediately after you mutate the store.)
// Intermittent errors might occur even though the operation succeeds, so you
// should always check if the object store has been modified even if the
// mutating operation returns an error.
//
// Objects should be created with "Post" operation and updated with "Put" operation.
//
// Object references supplied and returned from this interface should be
// treated as read-only. Modifying them violates thread-safety.
type ConfigRegistry interface {
	// Get retrieves a configuration element.
	Get(kind, key string) (config proto.Message, exists bool, revision Revision)

	// List returns objects by type indexed by the key
	List(kind string) (map[string]proto.Message, error)

	// Post creates a configuration object. If an object with the same
	// key already exists, the operation fails with no side effects.
	Post(kind string, v proto.Message) (Revision, error)

	// Put updates a configuration object in the distributed store.
	// Put requires that the object has been created.
	// Revision prevents overriding a value that has been changed
	// between prior _Get_ and _Put_ operation to achieve optimistic concurrency.
	Put(kind string, v proto.Message, revision Revision) (Revision, error)

	// Delete removes an object from the distributed store by key.
	Delete(kind, key string) error
}

// IstioRegistry is an interface to access config registry using Istio
// configuration types
type IstioRegistry interface {
	// RouteRules lists all routing rules indexed by keys
	RouteRules() map[string]*proxyconfig.RouteRule

	// RouteRulesBySource selects routing rules by source service instances.
	// A rule must match at least one of the input service instances since the proxy
	// does not distinguish between source instances in the request.
	// The rules are sorted by precedence (high first) in a stable manner.
	RouteRulesBySource(instances []*ServiceInstance) []*proxyconfig.RouteRule

	// IngressRules lists all ingress rules
	IngressRules() map[string]*proxyconfig.RouteRule

	// DestinationRules lists all destination rules
	DestinationRules() []*proxyconfig.DestinationPolicy

	// DestinationPolicy returns a policy for a service version.
	DestinationPolicy(destination string, tags Tags) *proxyconfig.DestinationVersionPolicy
}

// Revision is an opaque identifier for auditing updates to the config registry.
// The implementation may use a change index or a commit log for the revision.
// The config client should not make any assumptions about revisions and rely only on
// exact equality to implement optimistic concurrency of read-write operations.
type Revision string

// KindMap defines bijection between Kind name and proto message name
type KindMap map[string]ProtoSchema

// ProtoSchema provides custom validation checks
type ProtoSchema struct {
	// MessageName refers to the protobuf message type name
	MessageName string

	// Validate configuration as a protobuf message
	Validate func(o proto.Message) error

	// Key function derives the unique key from the configuration content
	Key func(o proto.Message) string

	// Internal flag indicates that the configuration type is derived
	// from other configuration sources. This prohibits direct updates
	// but allows listing and watching.
	Internal bool
}

// Kinds lists all kinds in the kind schemas
func (km KindMap) Kinds() []string {
	kinds := make([]string, 0, len(km))
	for kind := range km {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

const (
	// RouteRule defines the kind for the route rule configuration
	RouteRule = "route-rule"
	// RouteRuleProto message name
	RouteRuleProto = "istio.proxy.v1.config.RouteRule"

	// IngressRule kind
	IngressRule = "ingress-rule"
	// IngressRuleProto message name
	IngressRuleProto = RouteRuleProto

	// DestinationPolicy defines the kind for the destination policy configuration
	DestinationPolicy = "destination-policy"
	// DestinationPolicyProto message name
	DestinationPolicyProto = "istio.proxy.v1.config.DestinationPolicy"

	// HeaderURI is URI HTTP header
	HeaderURI = "uri"

	// HeaderAuthority is authority HTTP header
	HeaderAuthority = "authority"
)

var (
	// IstioConfig lists all Istio config kinds with schemas and validation
	IstioConfig = KindMap{
		RouteRule: ProtoSchema{
			MessageName: RouteRuleProto,
			Validate:    ValidateRouteRule,
		},
		IngressRule: ProtoSchema{
			MessageName: IngressRuleProto,
			Validate:    ValidateIngressRule,
			Internal:    true,
		},
		DestinationPolicy: ProtoSchema{
			MessageName: DestinationPolicyProto,
			Validate:    ValidateDestinationPolicy,
		},
	}
)

// IstioConfigRegistry provides a simple adapter for Istio configuration kinds
// from the generic config registry
type IstioConfigRegistry struct {
	ConfigRegistry
}

func (i IstioConfigRegistry) RouteRules() map[string]*proxyconfig.RouteRule {
	out := make(map[string]*proxyconfig.RouteRule)
	rs, err := i.List(RouteRule)
	if err != nil {
		glog.V(2).Infof("RouteRules => %v", err)
	}
	for key, r := range rs {
		if rule, ok := r.(*proxyconfig.RouteRule); ok {
			out[key] = rule
		}
	}
	return out
}

type routeRuleConfig struct {
	key  string
	rule *proxyconfig.RouteRule
}

func (i *IstioConfigRegistry) RouteRulesBySource(instances []*ServiceInstance) []*proxyconfig.RouteRule {
	rules := make([]*routeRuleConfig, 0)
	for key, rule := range i.RouteRules() {
		// validate that rule match predicate applies to source service instances
		if rule.Match != nil {
			found := false
			for _, instance := range instances {
				// must match the source field if it is set
				if rule.Match.Source != "" && rule.Match.Source != instance.Service.Hostname {
					continue
				}
				// must match the tags field - the rule tags are a subset of the instance tags
				var tags Tags = rule.Match.SourceTags
				if tags.SubsetOf(instance.Tags) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		rules = append(rules, &routeRuleConfig{key: key, rule: rule})
	}
	// sort by high precedence first, key string second (keys are unique)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].rule.Precedence > rules[j].rule.Precedence ||
			(rules[i].rule.Precedence == rules[j].rule.Precedence && rules[i].key < rules[j].key)
	})

	// project to rules
	out := make([]*proxyconfig.RouteRule, len(rules))
	for i, rule := range rules {
		out[i] = rule.rule
	}
	return out
}

// A temporary measure to communicate the destination service's port
// to the proxy configuration generator. This can be improved by using
// a dedicated model object for IngressRule (instead of reusing RouteRule),
// which exposes the necessary target port field within the "Route" field.
// This also carries TLS secret name.
const (
	IngressPortName  = "servicePortName"
	IngressPortNum   = "servicePortNum"
	IngressTLSSecret = "tlsSecret"
)

func (i *IstioConfigRegistry) IngressRules() map[string]*proxyconfig.RouteRule {
	out := make(map[string]*proxyconfig.RouteRule)
	rs, err := i.List(IngressRule)
	if err != nil {
		glog.V(2).Infof("IngressRules => %v", err)
	}
	for key, r := range rs {
		if rule, ok := r.(*proxyconfig.RouteRule); ok {
			out[key] = rule
		}
	}
	return out
}

func (i *IstioConfigRegistry) DestinationRules() []*proxyconfig.DestinationPolicy {
	out := make([]*proxyconfig.DestinationPolicy, 0)
	rs, err := i.List(DestinationPolicy)
	if err != nil {
		glog.V(2).Infof("DestinationPolicies => %v", err)
	}
	for _, r := range rs {
		if rule, ok := r.(*proxyconfig.DestinationPolicy); ok {
			out = append(out, rule)
		}
	}
	return out
}

func (i *IstioConfigRegistry) DestinationPolicy(destination string, tags Tags) *proxyconfig.DestinationVersionPolicy {
	// TODO: optimize destination policy retrieval
	for _, value := range i.DestinationRules() {
		if value.Destination == destination {
			for _, policy := range value.Policy {
				if tags.Equals(policy.Tags) {
					return policy
				}
			}
		}
	}
	return nil
}
