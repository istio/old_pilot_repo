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
		Valid      bool
	}{
		{
			Mode:       proxyconfig.ProxyMeshConfig_DEFAULT,
			Annotation: "",
			Valid:      true,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_DEFAULT,
			Annotation: "istio",
			Valid:      true,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_DEFAULT,
			Annotation: "nginx",
			Valid:      false,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_STRICT,
			Annotation: "",
			Valid:      false,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_STRICT,
			Annotation: "istio",
			Valid:      true,
		},
		{
			Mode:       proxyconfig.ProxyMeshConfig_STRICT,
			Annotation: "nginx",
			Valid:      false,
		},
	}

	for _, c := range cases {
		ingressClass, defaultIngressClass := convertIngressControllerMode(c.Mode, "istio")

		ing := makeAnnotatedIngress(c.Annotation)
		if valid := class.IsValid(ing, ingressClass, defaultIngressClass); valid != c.Valid {
			t.Errorf("%v -> expected %v, got %v", c, c.Valid, valid)
		}
	}
}
