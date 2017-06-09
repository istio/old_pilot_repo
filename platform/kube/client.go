// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kube

import (
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	// import GKE cluster authentication plugin
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// import OIDC cluster authentication plugin, e.g. for Tectonic
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"crypto/tls"

	"istio.io/pilot/model"
)

const (
	// IstioAPIGroup defines Kubernetes API group for TPR
	IstioAPIGroup = "istio.io"

	// IstioResourceVersion defines Kubernetes API group version
	IstioResourceVersion = "v1alpha1"

	// IstioKind defines the shared TPR kind to avoid boilerplate
	// code for each custom kind
	IstioKind = "IstioConfig"
)

// Client provides state-less Kubernetes bindings:
// - configuration objects are stored as third-party resources
// - dynamic REST client is configured to use third-party resources
// - static client exposes Kubernetes API
type Client struct {
	client kubernetes.Interface
}

// CreateRESTConfig for cluster API server, pass empty config file for in-cluster
func CreateRESTConfig(kubeconfig string) (config *rest.Config, err error) {
	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if err != nil {
		return
	}

	version := schema.GroupVersion{
		Group:   IstioAPIGroup,
		Version: IstioResourceVersion,
	}

	config.GroupVersion = &version
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			scheme.AddKnownTypes(
				version,
			)
			scheme.AddKnownTypeWithName(schema.GroupVersionKind{
				Group:   IstioAPIGroup,
				Version: IstioResourceVersion,
				Kind:    IstioKind,
			}, &Config{})
			scheme.AddKnownTypeWithName(schema.GroupVersionKind{
				Group:   IstioAPIGroup,
				Version: IstioResourceVersion,
				Kind:    IstioKind + "List",
			}, &ConfigList{})

			return nil
		})
	meta_v1.AddToGroupVersion(api.Scheme, version)
	err = schemeBuilder.AddToScheme(api.Scheme)

	return
}

// NewClient creates a client to Kubernetes API using a kubeconfig file.
// Use an empty value for `kubeconfig` to use the in-cluster config.
// If the kubeconfig file is empty, defaults to in-cluster config as well.
// namespace is used to store TPRs
func NewClient(kubeconfig string, km model.ConfigDescriptor, namespace string) (*Client, error) {
	if kubeconfig != "" {
		info, exists := os.Stat(kubeconfig)
		if exists != nil {
			return nil, fmt.Errorf("kubernetes configuration file %q does not exist", kubeconfig)
		}

		// if it's an empty file, switch to in-cluster config
		if info.Size() == 0 {
			glog.Info("Using in-cluster configuration")
			kubeconfig = ""
		}
	}

	config, err := CreateRESTConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	cl, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	out := &Client{
		client: cl,
	}

	return out, nil
}

// GetKubernetesClient retrieves core set kubernetes client
func (cl *Client) GetKubernetesClient() kubernetes.Interface {
	return cl.client
}

const (
	secretCert = "tls.crt"
	secretKey  = "tls.key"
)

// GetTLSSecret retrieves a TLS secret by implementation specific URI
// uri is "name"."namespace" for the secret
func (cl *Client) GetTLSSecret(uri string) (*model.TLSSecret, error) {
	parts := strings.Split(uri, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("URI %q does not match <name>.<namespace>", uri)
	}

	secret, err := cl.client.CoreV1().Secrets(parts[1]).Get(parts[0], meta_v1.GetOptions{})
	if err != nil {
		return nil, multierror.Prefix(err, "failed to retrieve secret "+uri)
	}

	cert := secret.Data[secretCert]
	key := secret.Data[secretKey]
	if len(cert) == 0 || len(key) == 0 {
		return nil, fmt.Errorf("Secret keys %q and/or %q are missing", secretCert, secretKey)
	}

	if _, err = tls.X509KeyPair(cert, key); err != nil {
		return nil, err
	}

	return &model.TLSSecret{
		Certificate: cert,
		PrivateKey:  key,
	}, nil
}
