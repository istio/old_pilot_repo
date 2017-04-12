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

	"fmt"

	"istio.io/manager/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	secretCert = "tls.crt"
	secretKey  = "tls.key"
)

// newSecretStore creates a new ingress secret store
func newSecretStore(client kubernetes.Interface) *secretStore {
	return &secretStore{
		client:  client,
		secrets: make(map[string]map[string]string),
	}
}

type secretStore struct {
	mutex   sync.RWMutex
	secrets map[string]map[string]string
	client  kubernetes.Interface
}

func (s *secretStore) put(namespace, host, secretName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.secrets[namespace] == nil {
		s.secrets[namespace] = make(map[string]string)
	}
	s.secrets[namespace][host] = secretName
}

func (s *secretStore) delete(namespace, host, secretName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.secrets[namespace] == nil {
		return
	}
	delete(s.secrets[namespace], host)
	if len(s.secrets[namespace]) == 0 {
		delete(s.secrets, namespace)
	}
}

// GetTLSSecret retrieves the TLS secret for a host
func (s *secretStore) GetTLSSecret(namespace, host string) (*model.TLSSecret, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.secrets[namespace] == nil {
		return nil, nil
	}

	// get the secret name
	name := s.secrets[namespace][host]
	if name == "" {
		name = s.secrets[namespace]["*"] // try falling back to wildcard
	}
	if name == "" {
		return nil, nil // no secret name for this host
	}

	// retrieve the secret
	secret, err := s.client.Core().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	cert := secret.Data[secretCert]
	key := secret.Data[secretKey]
	if len(cert) == 0 || len(key) == 0 {
		return nil, fmt.Errorf("Secret keys %q and/or %q are missing", secretCert, secretKey)
	}

	return &model.TLSSecret{
		Certificate: cert,
		PrivateKey:  key,
	}, nil
}
