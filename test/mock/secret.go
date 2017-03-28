package mock

type SecretRegistry struct {
	Secrets map[string]map[string][]byte
}

func (s *SecretRegistry) GetSecret(uri string) (map[string][]byte, error) {
	return s.Secrets[uri], nil
}
