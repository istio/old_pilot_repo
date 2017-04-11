package mock

import (
	"istio.io/manager/model"
)

// SecretRegistry is a mock of the secret registry
type SecretRegistry map[string]map[string]*model.TLSSecret

// GetTLSSecret retrieves a secret for the given namespace and host.
func (s SecretRegistry) GetTLSSecret(namespace, host string) (*model.TLSSecret, error) {
	if s[namespace] == nil {
		return nil, nil
	}
	return s[namespace][host], nil
}
