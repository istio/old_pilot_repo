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

package inject

import (
	"testing"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInjectRequired(t *testing.T) {
	cases := []struct {
		policy map[string]InjectionPolicy
		meta   *metav1.ObjectMeta
		want   bool
	}{
		{
			policy: map[string]InjectionPolicy{v1.NamespaceAll: InjectionPolicyOptOut},
			meta: &metav1.ObjectMeta{
				Name:        "no-policy",
				Namespace:   "test-namespace",
				Annotations: map[string]string{},
			},
			want: true,
		},
		{
			policy: map[string]InjectionPolicy{v1.NamespaceAll: InjectionPolicyOptOut},
			meta: &metav1.ObjectMeta{
				Name:        "default-policy",
				Namespace:   "test-namespace",
				Annotations: map[string]string{istioSidecarAnnotationPolicyKey: istioSidecarAnnotationPolicyValueDefault},
			},
			want: true,
		},
		{
			policy: map[string]InjectionPolicy{v1.NamespaceAll: InjectionPolicyOptOut},
			meta: &metav1.ObjectMeta{
				Name:        "force-on-policy",
				Namespace:   "test-namespace",
				Annotations: map[string]string{istioSidecarAnnotationPolicyKey: istioSidecarAnnotationPolicyValueForceOn},
			},
			want: true,
		},
		{
			policy: map[string]InjectionPolicy{v1.NamespaceAll: InjectionPolicyOptOut},
			meta: &metav1.ObjectMeta{
				Name:        "force-off-policy",
				Namespace:   "test-namespace",
				Annotations: map[string]string{istioSidecarAnnotationPolicyKey: istioSidecarAnnotationPolicyValueForceOff},
			},
			want: false,
		},
	}

	for _, c := range cases {
		if got := injectRequired(c.policy, c.meta); got != c.want {
			t.Errorf("injectRequired(%v, %v) got %v want %v", c.policy, c.meta, got, c.want)
		}
	}
}
