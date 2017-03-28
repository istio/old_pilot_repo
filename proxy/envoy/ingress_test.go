package envoy

import (
	"testing"

	"istio.io/manager/model"
	"istio.io/manager/test/mock"
	"io/ioutil"
	"github.com/pmezard/go-difflib/difflib"
	"fmt"
	"reflect"
)

const (
	ingressEnvoyV0Config = "testdata/ingress-envoy-v0.json"
	ingressRouteRule     = "testdata/ingress-route.yaml.golden"
)

func compareFile(filename string, expect []byte, t *testing.T) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Error loading %s", filename)
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
		t.Fatal("Failed validating file %s", filename)
	}
}

func testIngressConfig(context *IngressContext, envoyConfig string, t *testing.T) {
	config := generateIngress(context)
	if config == nil {
		t.Fatal("Failed to generate config")
	}

	if err := config.WriteFile(envoyConfig); err != nil {
		t.Fatalf(err.Error())
	}

	compareJSON(envoyConfig, t)
}

func addIngressRoute(r *model.IstioRegistry, t *testing.T) {
	msg, err := configObjectFromYAML(model.IngressRule, ingressRouteRule)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Post(model.Key{Kind: model.IngressRule, Name: "route"}, msg); err != nil {
		t.Fatal(err)
	}
}

func TestIngressRoutes(t *testing.T) {
	r := mock.MakeRegistry()
	addIngressRoute(r, t)
	testIngressConfig(&IngressContext{
		Registry: r,
		Mesh: DefaultMeshConfig,
	}, ingressEnvoyV0Config, t)
}

func TestIngressRoutesSSL(t *testing.T) {
	crt := []byte("abcdefghijklmnop")
	key := []byte("qrstuvwxyz123456")

	r := mock.MakeRegistry()
	s := mock.SecretRegistry{
		Secrets: map[string]map[string][]byte {
			"secret": {
				"tls.crt": crt,
				"tls.key": key,
			},
		},
	}
	addIngressRoute(r, t)
	testIngressConfig(&IngressContext{
		CertFilename: "testdata/tls.crt",
		KeyFilename: "testdata/tls.key",
		Namespace: "",
		Secret: "secret",
		Secrets: &s,
		Registry: r,
		Mesh: DefaultMeshConfig,
	}, ingressEnvoyV0Config, t)
	compareFile("testdata/tls.crt", crt, t)
	compareFile("testdata/tls.key", key, t)
}
