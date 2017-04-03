package kube

import (
	"k8s.io/client-go/kubernetes"
	meta_v1 "k8s.io/client-go/pkg/apis/meta/v1"
	"sync"
)

func NewSecret(namespace string, client kubernetes.Interface) *Secret {
	return &Secret{
		namespace: namespace,
		client: client,
		secrets: make(map[string]string),
	}
}

// TODO: intelligent synchronization
type Secret struct {
	mutex sync.Mutex
	namespace string
	secrets map[string]string
	client kubernetes.Interface
}

func (s *Secret) put(k, v string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.secrets[k] = v
}

func (s *Secret) GetSecret(host string) (map[string][]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	name := s.secrets[host]
	if name == "" {
		return nil, nil // No secret
	}
	secret, err := s.client.Core().Secrets(s.namespace).Get(name, meta_v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}