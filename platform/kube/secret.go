package kube

import (
	"sync"

	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"istio.io/manager/model"
)

// keys
const (
	secretCertificate = "tls.crt"
	secretPrivateKey = "tls.key"
)

// New
func NewSecret(namespace string, client kubernetes.Interface) *secret {
	return &secret{
		namespace: namespace,
		client:    client,
		secrets:   make(map[string]string),
	}
}

type secret struct {
	mutex     sync.Mutex
	namespace string
	secrets   map[string]string
	client    kubernetes.Interface
}

func (s *secret) put(host, secretName string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.secrets[host] = secretName
}

func (s *secret) GetSecret(host string) (*model.TLSContext, error) {
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
		Certificate: []byte(secret.Data[secretCertificate]),
		PrivateKey: []byte(secret.Data[secretPrivateKey]),
	}, nil
}
