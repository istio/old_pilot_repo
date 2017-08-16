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

package crd

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/pilot/model"
)

var knownTypes = map[string]struct {
	obj        istioObject
	collection istioObjectList
}{
	model.MockConfig.Type: {
		obj:        &MockConfig{TypeMeta: meta_v1.TypeMeta{Kind: "MockConfig", APIVersion: model.IstioAPIVersion}},
		collection: &MockConfigList{},
	},
	model.RouteRule.Type: {
		obj:        &RouteRule{TypeMeta: meta_v1.TypeMeta{Kind: "RouteRule", APIVersion: model.IstioAPIVersion}},
		collection: &RouteRuleList{},
	},
	model.IngressRule.Type: {
		obj:        &IngressRule{TypeMeta: meta_v1.TypeMeta{Kind: "IngressRule", APIVersion: model.IstioAPIVersion}},
		collection: &IngressRuleList{},
	},
	model.DestinationPolicy.Type: {
		obj:        &DestinationPolicy{TypeMeta: meta_v1.TypeMeta{Kind: "DestinationPolicy", APIVersion: model.IstioAPIVersion}},
		collection: &DestinationPolicyList{},
	},
}
