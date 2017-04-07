package envoy

import (
	"testing"

	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/pmezard/go-difflib/difflib"
	"istio.io/manager/model"
	"istio.io/manager/test/mock"
	"encoding/json"
)

const (
	ingressEnvoyConfig    = "testdata/ingress-envoy.json"
	ingressEnvoySSLConfig = "testdata/ingress-envoy-ssl.json"
	ingressEnvoyPartialSSLConfig = "testdata/ingress-envoy-partial-ssl.json"
	ingressRouteRule1      = "testdata/ingress-route-1.yaml.golden"
	ingressRouteRule2      = "testdata/ingress-route-2.yaml.golden"
	ingressCertFile       = "testdata/tls.crt"
	ingressKeyFile        = "testdata/tls.key"
)

var (
	ingressCert       = []byte("abcdefghijklmnop")
	ingressKey        = []byte("qrstuvwxyz123456")
	ingressTLSContext = &model.TLSContext{ingressCert, ingressKey}
)

func compareFile(filename string, expect []byte, t *testing.T) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Error loading %s: %s", filename, err.Error())
	}
	if !reflect.DeepEqual(data, expect) {
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(expect)),
			B:        difflib.SplitLines(string(data)),
			FromFile: filename,
			ToFile:   "",
			Context:  2,
		}
		text, _ := difflib.GetUnifiedDiffString(diff)
		fmt.Println(text)
		t.Fatalf("Failed validating file %s", filename)
	}
}

func testIngressConfig(c *IngressConfig, envoyConfig string, t *testing.T) {
	config := generateIngress(c)
	if config == nil {
		t.Fatal("Failed to generate config")
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	fmt.Println(string(data))

	if err := config.WriteFile(envoyConfig); err != nil {
		t.Fatal(err)
	}

	compareJSON(envoyConfig, t)
}

func addIngressRoutes(r *model.IstioRegistry, t *testing.T) {
	for i, file := range []string{ ingressRouteRule1, ingressRouteRule2 } {
		msg, err := configObjectFromYAML(model.IngressRule, file)
		if err != nil {
			t.Fatal(err)
		}
		if err = r.Post(model.Key{Kind: model.IngressRule, Name: fmt.Sprintf("route_%d", i)}, msg); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIngressRoutes(t *testing.T) {
	r := mock.MakeRegistry()
	s := &mock.SecretRegistry{}
	addIngressRoutes(r, t)
	testIngressConfig(&IngressConfig{
		Registry: r,
		Secrets:  s,
		Mesh:     DefaultMeshConfig,
	}, ingressEnvoyConfig, t)
}

func TestIngressRoutesSSL(t *testing.T) {
	r := mock.MakeRegistry()
	s := &mock.SecretRegistry{"*": ingressTLSContext}
	addIngressRoutes(r, t)
	testIngressConfig(&IngressConfig{
		CertFile:  ingressCertFile,
		KeyFile:   ingressKeyFile,
		Namespace: "",
		Secrets:   s,
		Registry:  r,
		Mesh:      DefaultMeshConfig,
	}, ingressEnvoySSLConfig, t)
	compareFile(ingressCertFile, ingressCert, t)
	compareFile(ingressKeyFile, ingressKey, t)
}

func TestIngressRoutesPartialSSL(t *testing.T) {
	r := mock.MakeRegistry()
	s := &mock.SecretRegistry{"world.default.svc.cluster.local": ingressTLSContext}
	addIngressRoutes(r, t)
	testIngressConfig(&IngressConfig{
		CertFile:  ingressCertFile,
		KeyFile:   ingressKeyFile,
		Namespace: "",
		Secrets:   s,
		Registry:  r,
		Mesh:      DefaultMeshConfig,
	}, ingressEnvoyPartialSSLConfig, t)
	compareFile(ingressCertFile, ingressCert, t)
	compareFile(ingressKeyFile, ingressKey, t)
}
