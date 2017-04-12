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

	"strings"

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
		secrets: make(map[string]string),
	}
}

type secretStore struct {
	mutex             sync.RWMutex
	secrets           map[string]string
	wildcardNamespace string
	wildcardSecret    string
	client            kubernetes.Interface
}

func (s *secretStore) setWildcard(namespace, secretName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.wildcardNamespace = namespace
	s.wildcardSecret = secretName
}

func (s *secretStore) put(uri, secretName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.secrets[uri] = secretName
}

func (s *secretStore) delete(uri string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.secrets, uri)
}

// GetTLSSecret retrieves the TLS secret for a host.
func (s *secretStore) GetTLSSecret(uri string) (*model.TLSSecret, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var namespace string
	if uri == "*" {
		namespace = s.wildcardNamespace
	} else {
		parts := strings.Split(uri, ".")
		if len(parts) < 2 {
			return nil, fmt.Errorf("%q is not in the expected URI format: name.namespace.svc.cluster.local", uri)
		}
		namespace = parts[1]
	}

	// get the secret name
	name := s.secrets[uri]
	if name == "" {
		name = s.wildcardSecret
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
