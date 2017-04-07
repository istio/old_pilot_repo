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

package kube

import (
	"sync"

	"istio.io/manager/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NewSecretStore creates a new ingress secret store
func NewSecretStore(namespace string, client kubernetes.Interface) *secretStore {
	return &secretStore{
		namespace: namespace,
		client:    client,
		secrets:   make(map[string]string),
	}
}

type secretStore struct {
	mutex     sync.Mutex
	namespace string
	secrets   map[string]string
	client    kubernetes.Interface
}

func (s *secretStore) put(host, secretName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.secrets[host] = secretName
}

// GetSecret retrieves the secret for a host
func (s *secretStore) GetSecret(host string) (*model.TLSContext, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// get the secret name
	name := s.secrets[host]
	if name == "" {
		name = s.secrets["*"] // try falling back to wildcard
	}
	if name == "" {
		return nil, nil // no secret name for this host
	}

	// retrieve the secret
	secret, err := s.client.Core().Secrets(s.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &model.TLSContext{
		Certificate: []byte(secret.Data["tls.crt"]),
		PrivateKey:  []byte(secret.Data["tls.key"]),
	}, nil
}
