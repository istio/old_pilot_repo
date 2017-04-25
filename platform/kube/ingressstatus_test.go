package kube

import (
	"testing"

	proxyconfig "istio.io/api/proxy/v1/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/ingress/core/pkg/ingress/annotations/class"
)

func makeAnnotatedIngress(annotation string) *extensions.Ingress {
	if annotation == "" {
		return &extensions.Ingress{}
	}

	return &extensions.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ingressClassAnnotation: annotation,
			},
		},
	}
}

// TestConvertIngressControllerMode ensures that ingress controller mode is converted to the k8s ingress status syncer's
// representation correctly.
func TestConvertIngressControllerMode(t *testing.T) {
	cases := []struct {
		Mode       proxyconfig.ProxyMeshConfig_IngressControllerMode
		Annotation string
		Ignore     bool
	}{
		{
			Mode:       proxyconfig.ProxyMeshConfig_DEFAULT,
			Annotation: "",
			Ignore:     true,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_DEFAULT,
			Annotation: "istio",
			Ignore:     true,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_DEFAULT,
			Annotation: "nginx",
			Ignore:     false,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_STRICT,
			Annotation: "",
			Ignore:     false,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_STRICT,
			Annotation: "istio",
			Ignore:     true,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_STRICT,
			Annotation: "nginx",
			Ignore:     false,
		},
	}

	for _, c := range cases {
		ingressClass, defaultIngressClass := convertIngressControllerMode(c.Mode, "istio")

		ing := makeAnnotatedIngress(c.Annotation)
		if ignore := class.IsValid(ing, ingressClass, defaultIngressClass); ignore != c.Ignore {
			t.Errorf("convertIngressControllerMode(%q, %q) => Got ignore %v, want %v", c, c.Ignore, ignore)
		}
	}
}
