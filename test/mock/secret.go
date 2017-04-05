package mock

import "istio.io/manager/model"

// SecretRegistry is a mock of the secret registry
type SecretRegistry map[string]*model.TLSContext

// GetSecret retrieves a secret for the given URI
func (s SecretRegistry) GetSecret(uri string) (*model.TLSContext, error) {
	return s[uri], nil
}
