package mock

import (
	"testing"

	"istio.io/manager/model"
)

func TestSecret(t *testing.T) {
	tls := &model.TLSSecret{
		Certificate: []byte("abcdef"),
		PrivateKey:  []byte("ghijkl"),
	}
	ns := "default"
	host := "host1"

	s := SecretRegistry{ns: {host: tls}}
	if secret, err := s.GetTLSSecret(ns, host); err != nil {
		t.Fatalf("GetTLSSecret(%q, %q) -> %q", ns, host, err)
	} else if secret == nil {
		t.Fatalf("GetTLSSecret(%q, %q) -> not found", ns, host)
	}
}
