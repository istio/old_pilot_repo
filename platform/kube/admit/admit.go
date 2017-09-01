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

package admit

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"k8s.io/api/admission/v1alpha1"
	admissionregistrationv1alpha1 "k8s.io/api/admissionregistration/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/client-go/kubernetes"
	admissionClient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1alpha1"

	"istio.io/pilot/adapter/config/crd"
	"istio.io/pilot/model"
)

// A cluster-unique (i.e. random) suffix should be added to each
// default below when testing in a shared cluster to avoid collisions.

const (
	// DefaultAdmissionHookConfigName is the default name for the
	// ExternalAdmissionHookConfiguration.
	DefaultAdmissionHookConfigName = "pilot-config"

	// DefaultAdmissionHookName is the default name for the
	// ExternalAdmissionHooks.
	DefaultAdmissionHookName = "pilot.config.istio.io"

	// DefaultAdmissionServiceName is the default service of the
	// validation webhook.
	//
	// This is a reference to the service for the webhook. If there is
	// only one port open for the service, that port will be used. If
	// there are multiple ports open, port 443 will be used if it is
	// open, otherwise it is an error.
	//
	// ref - k8s.io/pkg/apis/admissionregistration/types.go#L197
	//
	// This needs to point to an external service when running on
	// Kubernetes 1.7 to work around some cloud provider bug (see
	// https://github.com/kubernetes/kubernetes/issues/49987#issuecomment-319739227)
	DefaultAdmissionServiceName = "istio-pilot"
)

// AdmissionController implements the external admission webhook for validation of
// pilot configuration.
type AdmissionController struct {
	descriptor model.ConfigDescriptor

	// externalAdmissionHookConfigurationName is the name of the
	// ExternalAdmissionHookConfiguration which configures the
	// initializer.
	externalAdmissionHookConfigurationName string

	// Name of the ExternalAdmissionHook which describes he external
	// admission webhook and resources and operations it applies to.
	externalAdmissionHookName string

	// serviceName is the service name of the webhook.
	serviceName string

	// serviceNamespace is the namespace of the webhook service.
	serviceNamespace string

	// cabundle is the PEM encoded CA bundle which will be used to
	// validate webhook's service certificate.
	caBundle []byte

	// unconditionally validate all config that is not in this list of
	// configuration.
	validateNamespaces []string
}

// GetAPIServerExtensionCACert gets the Kubernetes aggregate apiserver
// client CA cert used by the "GenericAdmissionWebhook" plugin
// admission controller.
//
// NOTE: this certificate is provided kubernetes. We do not control
// its name or location.
func GetAPIServerExtensionCACert(cl kubernetes.Interface) ([]byte, error) {
	const name = "extension-apiserver-authentication"
	c, err := cl.CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pem, ok := c.Data["requestheader-client-ca-file"]
	if !ok {
		return nil, fmt.Errorf("cannot find ca.crt in %v: ConfigMap.Data is %#v", name, c.Data)
	}
	return []byte(pem), nil
}

// MakeTLSConfig makes a TLS configuration suitable for use with the
// GenericAdmissionWebhook.
func MakeTLSConfig(serverCert, serverKey, caCert []byte) (*tls.Config, error) {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	cert, err := tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

// NewController creates a new instance of the admission webhook controller.
func NewController(descriptor model.ConfigDescriptor, externalAdmissionHookConfigurationName, externalAdmissionHookName,
	serviceName, serviceNamespace string, validateNamespaces []string, caBundle []byte) *AdmissionController {
	return &AdmissionController{
		descriptor:                             descriptor,
		externalAdmissionHookConfigurationName: externalAdmissionHookConfigurationName,
		externalAdmissionHookName:              externalAdmissionHookName,
		serviceName:                            serviceName,
		serviceNamespace:                       serviceNamespace,
		caBundle:                               caBundle,
		validateNamespaces:                     validateNamespaces,
	}
}

// Unregister unregisters the external admission webhook
func (ac *AdmissionController) Unregister(client admissionClient.ExternalAdmissionHookConfigurationInterface) error {
	return client.Delete(ac.externalAdmissionHookConfigurationName, nil)
}

// Register registers the external admission webhook for pilot
// configuration types.
func (ac *AdmissionController) Register(client admissionClient.ExternalAdmissionHookConfigurationInterface) error {
	var resources []string
	for _, schema := range ac.descriptor {
		resources = append(resources, schema.Plural)
	}

	webhook := &admissionregistrationv1alpha1.ExternalAdmissionHookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: ac.externalAdmissionHookConfigurationName,
		},
		ExternalAdmissionHooks: []admissionregistrationv1alpha1.ExternalAdmissionHook{
			{
				Name: ac.externalAdmissionHookName,
				Rules: []admissionregistrationv1alpha1.RuleWithOperations{{
					Operations: []admissionregistrationv1alpha1.OperationType{
						admissionregistrationv1alpha1.Create,
						admissionregistrationv1alpha1.Update,
					},
					Rule: admissionregistrationv1alpha1.Rule{
						APIGroups:   []string{model.IstioAPIGroup},
						APIVersions: []string{model.IstioAPIVersion},
						Resources:   resources,
					},
				}},
				ClientConfig: admissionregistrationv1alpha1.AdmissionHookClientConfig{
					Service: admissionregistrationv1alpha1.ServiceReference{
						Namespace: ac.serviceNamespace,
						Name:      ac.serviceName,
					},
					CABundle: ac.caBundle,
				},
			},
		},
	}
	if err := client.Delete(webhook.Name, nil); err != nil {
		glog.V(2).Info("Delete %v failed before trying to (re)register (innocuous):%v",
			webhook.Name, err)
	}
	_, err := client.Create(webhook) // Update?
	return err
}

// ServeHTTP implements the external admission webhook for validating
// pilot configuration.
func (ac *AdmissionController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "invalid Content-Type, want `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var review v1alpha1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		http.Error(w, fmt.Sprintf("could not decode body: %v", err), http.StatusBadRequest)
		return
	}
	status := ac.admit(&review)

	resp, err := json.Marshal(status)
	if err != nil {
		http.Error(w, fmt.Sprintf("could encode response: %v", err), http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(resp); err != nil {
		http.Error(w, fmt.Sprintf("could write response: %v", err), http.StatusInternalServerError)
		return
	}
}

func watched(watchedNamespaces []string, namespace string) bool {
	for _, watched := range watchedNamespaces {
		if watched == metav1.NamespaceAll {
			return true
		} else if watched == namespace {
			return true
		}
		// else, keep searching
	}
	return false
}

func (ac *AdmissionController) admit(review *v1alpha1.AdmissionReview) *v1alpha1.AdmissionReviewStatus {
	makeErrorStatus := func(reason string, args ...interface{}) *v1alpha1.AdmissionReviewStatus {
		result := apierrors.NewBadRequest(fmt.Sprintf(reason, args...)).Status()
		return &v1alpha1.AdmissionReviewStatus{
			Result: &result,
		}
	}

	switch review.Spec.Operation {
	case admission.Create, admission.Update:
	default:
		glog.Warningf("Unsupported webhook operation %v", review.Spec.Operation)
		return &v1alpha1.AdmissionReviewStatus{Allowed: true}
	}

	var obj crd.IstioKind
	if err := yaml.Unmarshal(review.Spec.Object.Raw, &obj); err != nil {
		return makeErrorStatus("cannot decode configuration: %v", err)
	}

	if !watched(ac.validateNamespaces, obj.Namespace) {
		return &v1alpha1.AdmissionReviewStatus{Allowed: true}
	}

	schema, exists := ac.descriptor.GetByType(crd.CamelCaseToKabobCase(obj.Kind))
	if !exists {
		return makeErrorStatus("unrecognized type %v", obj.Kind)
	}

	out, err := crd.ConvertObject(schema, &obj)
	if err != nil {
		return makeErrorStatus("error decoding configuration: %v", err)
	}

	if err := schema.Validate(out.Spec); err != nil {
		return makeErrorStatus("configuration is invalid: %v", err)
	}

	return &v1alpha1.AdmissionReviewStatus{Allowed: true}
}
