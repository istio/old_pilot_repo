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

package inject

// NOTE: This tool only exists because kubernetes does not support
// dynamic/out-of-tree admission controller for transparent proxy
// injection. This file should be removed as soon as a proper kubernetes
// admission controller is written for istio.

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	yamlDecoder "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/model"
	"istio.io/pilot/proxy"
)

// TODO - Temporally retain the deprecated alpha annotations to ease
// migration to k8s sidecar initializer.
var (
	insertDeprecatedAlphaAnnotation = true
	checkDeprecatedAlphaAnnotation  = true
)

const istioSidecarAnnotationStatusKey = "status.sidecar.istio.io"

// per-sidecar policy (deployment, job, statefulset, pod, etc)
const (
	istioSidecarAnnotationPolicyKey           = "policy.sidecar.istio.io"
	istioSidecarAnnotationPolicyValueDefault  = "policy.sidecar.istio.io/default"
	istioSidecarAnnotationPolicyValueForceOn  = "policy.sidecar.istio.io/force-on"
	istioSidecarAnnotationPolicyValueForceOff = "policy.sidecar.istio.io/force-off"
)

type InjectionPolicy string

const (
	InjectionPolicyOff    InjectionPolicy = "off"
	InjectionPolicyOptIn  InjectionPolicy = "opt-in"
	InjectionPolicyOptOut InjectionPolicy = "opt-out"
)

var defaultNamespacePolicy = map[string]InjectionPolicy{
	"":            InjectionPolicyOptOut,
	"kube-system": InjectionPolicyOff,
}

// Defaults values for injecting istio proxy into kubernetes
// resources.
const (
	DefaultSidecarProxyUID = int64(1337)
	DefaultVerbosity       = 2
)

const (
	// deprecated - remove after istio/istio is updated to use new templates
	deprecatedIstioSidecarAnnotationSidecarKey   = "alpha.istio.io/sidecar"
	deprecatedIstioSidecarAnnotationSidecarValue = "injected(deprecated)"

	// InitContainerName is the name for init container
	InitContainerName = "istio-init"

	// ProxyContainerName is the name for sidecar proxy container
	ProxyContainerName = "istio-proxy"

	enableCoreDumpContainerName = "enable-core-dump"
	enableCoreDumpImage         = "alpine"

	istioCertSecretPrefix = "istio."

	istioCertVolumeName        = "istio-certs"
	istioConfigVolumeName      = "istio-config"
	istioEnvoyConfigVolumeName = "istio-envoy"

	// ConfigMapKey should match the expected MeshConfig file name
	ConfigMapKey = "mesh"
)

// InitImageName returns the fully qualified image name for the istio
// init image given a docker hub and tag
func InitImageName(hub, tag string) string { return hub + "/proxy_init:" + tag }

// ProxyImageName returns the fully qualified image name for the istio
// proxy image given a docker hub and tag.
func ProxyImageName(hub, tag string) string { return hub + "/proxy_debug:" + tag }

// Params describes configurable parameters for injecting istio proxy
// into kubernetes resource.
type Params struct {
	InitImage         string
	ProxyImage        string
	Verbosity         int
	SidecarProxyUID   int64
	Version           string
	EnableCoreDump    bool
	Mesh              *proxyconfig.ProxyMeshConfig
	MeshConfigMapName string
	ImagePullPolicy   string
	// Comma separated list of IP ranges in CIDR form. If set, only
	// redirect outbound traffic to Envoy for these IP
	// ranges. Otherwise all outbound traffic is redirected to Envoy.
	IncludeIPRanges string
}

// GetMeshConfig fetches configuration from a config map
func GetMeshConfig(kube kubernetes.Interface, namespace, name string) (*proxyconfig.ProxyMeshConfig, error) {
	config, err := kube.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// values in the data are strings, while proto might use a different data type.
	// therefore, we have to get a value by a key
	yaml, exists := config.Data[ConfigMapKey]
	if !exists {
		return nil, fmt.Errorf("missing configuration map key %q", ConfigMapKey)
	}

	mesh := proxy.DefaultMeshConfig()
	if err = model.ApplyYAML(yaml, &mesh); err != nil {
		return nil, multierror.Prefix(err, "failed to convert to proto.")
	}

	if err = model.ValidateProxyMeshConfig(&mesh); err != nil {
		return nil, err
	}

	return &mesh, nil
}

func injectRequired(injectionPolicy map[string]InjectionPolicy, objectMeta *metav1.ObjectMeta) bool {
	namespacePolicy, ok := injectionPolicy[objectMeta.Namespace]
	if !ok {
		glog.V(2).Infof("No namespace policy for %v/%v: trying global policy",
			objectMeta.Namespace, objectMeta.Name)
		// inherit from global policy
		namespacePolicy, ok = injectionPolicy[v1.NamespaceAll]
		if !ok {
			return false
		}
	}

	var resourcePolicy string
	if objectMeta.Annotations == nil {
		resourcePolicy = istioSidecarAnnotationPolicyValueDefault
	} else {
		if resourcePolicy, ok = objectMeta.Annotations[istioSidecarAnnotationPolicyKey]; !ok {
			resourcePolicy = istioSidecarAnnotationPolicyValueDefault
			glog.V(2).Infof("No policy for %v/%v: defaulting to %v",
				objectMeta.Namespace, objectMeta.Name, resourcePolicy)
		}
	}

	var required bool
	switch namespacePolicy {
	case InjectionPolicyOptIn:
		if resourcePolicy == istioSidecarAnnotationPolicyValueForceOn {
			required = true
		}
	case InjectionPolicyOptOut:
		if resourcePolicy != istioSidecarAnnotationPolicyValueForceOff {
			required = true
		}
	}

	status, ok := objectMeta.Annotations[istioSidecarAnnotationStatusKey]

	glog.V(2).Info("Sidecar injection policy for %v/%v: namespacePolicy:%v resourcePolicy:%v inject?:%v status:%v",
		objectMeta.Namespace, objectMeta.Name, namespacePolicy, resourcePolicy, required, status)

	if checkDeprecatedAlphaAnnotation {
		if _, ok := objectMeta.Annotations[deprecatedIstioSidecarAnnotationSidecarKey]; ok {
			return false
		}
	}

	if !required {
		return false
	}

	// TODO - add version check for sidecar upgrade

	return !ok
}

func addAnnotation(objectMeta *metav1.ObjectMeta, version string) {
	if objectMeta.Annotations == nil {
		objectMeta.Annotations = make(map[string]string)
	}
	objectMeta.Annotations[istioSidecarAnnotationStatusKey] = "injected-version-" + version

	if insertDeprecatedAlphaAnnotation {
		objectMeta.Annotations[deprecatedIstioSidecarAnnotationSidecarKey] =
			deprecatedIstioSidecarAnnotationSidecarValue
	}
}

func injectIntoPodTemplateSpec(policy map[string]InjectionPolicy, p *Params, objectMeta *metav1.ObjectMeta, spec *v1.PodSpec) error {
	if !injectRequired(policy, objectMeta) {
		return nil
	}
	addAnnotation(objectMeta, p.Version)

	// proxy initContainer 1.6 spec
	initArgs := []string{
		"-p", fmt.Sprintf("%d", p.Mesh.ProxyListenPort),
		"-u", strconv.FormatInt(p.SidecarProxyUID, 10),
	}
	if p.IncludeIPRanges != "" {
		initArgs = append(initArgs, "-i", p.IncludeIPRanges)
	}

	var pullPolicy v1.PullPolicy
	switch p.ImagePullPolicy {
	case "Always":
		pullPolicy = v1.PullAlways
	case "IfNotPresent":
		pullPolicy = v1.PullIfNotPresent
	case "Never":
		pullPolicy = v1.PullNever
	default:
		pullPolicy = v1.PullIfNotPresent
	}

	privTrue := true

	initContainer := v1.Container{
		Name:            InitContainerName,
		Image:           p.InitImage,
		Args:            initArgs,
		ImagePullPolicy: pullPolicy,
		SecurityContext: &v1.SecurityContext{
			Capabilities: &v1.Capabilities{
				Add: []v1.Capability{"CAP_NET_ADMIN"},
			},
			// TODO: Determine SELINUX options needed to remove privileged
			Privileged: &privTrue,
		},
	}

	enableCoreDumpContainer := v1.Container{
		Name:    enableCoreDumpContainerName,
		Image:   enableCoreDumpImage,
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			"sysctl -w kernel.core_pattern=/tmp/core.%e.%p.%t && ulimit -c unlimited",
		},
		ImagePullPolicy: pullPolicy,
		SecurityContext: &v1.SecurityContext{
			// TODO: Determine SELINUX options needed to remove privileged
			Privileged: &privTrue,
		},
	}

	spec.InitContainers = append(spec.InitContainers, initContainer)

	if p.EnableCoreDump {
		spec.InitContainers = append(spec.InitContainers, enableCoreDumpContainer)
	}

	// sidecar proxy container
	args := []string{"proxy"}

	if p.Verbosity > 0 {
		args = append(args, "-v", strconv.Itoa(p.Verbosity))
	}

	_, _ = healthPorts(spec)
	/* See issue https://github.com/istio/pilot/issues/953
	if err != nil {
		return err
	}
		for _, port := range ports {
			args = append(args, "--passthrough", strconv.Itoa(port))
		}
	*/

	volumeMounts := []v1.VolumeMount{
		{
			Name:      istioConfigVolumeName,
			ReadOnly:  true,
			MountPath: "/etc/istio/config",
		},
		{
			Name:      istioEnvoyConfigVolumeName,
			MountPath: "/etc/istio/proxy",
		},
	}

	spec.Volumes = append(spec.Volumes,
		v1.Volume{
			Name: istioConfigVolumeName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: p.MeshConfigMapName,
					},
				},
			},
		},
		v1.Volume{
			Name: istioEnvoyConfigVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{
					Medium: v1.StorageMediumMemory,
				},
			},
		})

	if p.Mesh.AuthPolicy == proxyconfig.ProxyMeshConfig_MUTUAL_TLS {
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      istioCertVolumeName,
			ReadOnly:  true,
			MountPath: p.Mesh.AuthCertsPath,
		})

		sa := spec.ServiceAccountName
		if sa == "" {
			sa = "default"
		}
		spec.Volumes = append(spec.Volumes, v1.Volume{
			Name: istioCertVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: istioCertSecretPrefix + sa,
				},
			},
		})
	}

	readOnly := true
	sidecar := v1.Container{
		Name:  ProxyContainerName,
		Image: p.ProxyImage,
		Args:  args,
		Env: []v1.EnvVar{{
			Name: "POD_NAME",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		}, {
			Name: "POD_NAMESPACE",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		}, {
			Name: "INSTANCE_IP",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		}},
		ImagePullPolicy: pullPolicy,
		SecurityContext: &v1.SecurityContext{
			RunAsUser:              &p.SidecarProxyUID,
			ReadOnlyRootFilesystem: &readOnly,
		},
		VolumeMounts: volumeMounts,
	}

	spec.Containers = append(spec.Containers, sidecar)

	return nil
}

func resolvePort(c v1.Container, port intstr.IntOrString) (int, error) {
	switch port.Type {
	case intstr.Int:
		return port.IntValue(), nil
	case intstr.String:
		for _, named := range c.Ports {
			if named.Name == port.String() {
				return int(named.ContainerPort), nil
			}
		}
		return 0, fmt.Errorf("missing named port %q", port)
	default:
		return 0, fmt.Errorf("incorrect port type %q", port)
	}
}

func healthPorts(spec *v1.PodSpec) ([]int, error) {
	set := make(map[int]bool)
	var errs error
	for _, container := range spec.Containers {
		if container.LivenessProbe != nil && container.LivenessProbe.HTTPGet != nil {
			port, err := resolvePort(container, container.LivenessProbe.HTTPGet.Port)
			if err != nil {
				errs = multierror.Append(errs, err)
			} else {
				set[port] = true
			}
		}
		if container.ReadinessProbe != nil && container.ReadinessProbe.HTTPGet != nil {
			port, err := resolvePort(container, container.ReadinessProbe.HTTPGet.Port)
			if err != nil {
				errs = multierror.Append(errs, err)
			} else {
				set[port] = true
			}
		}
	}

	out := make([]int, 0, len(set))
	for port := range set {
		out = append(out, port)
	}
	sort.Ints(out)
	return out, errs

}

// IntoResourceFile injects the istio proxy into the specified
// kubernetes YAML file.
func IntoResourceFile(p *Params, in io.Reader, out io.Writer) error {
	reader := yamlDecoder.NewYAMLReader(bufio.NewReaderSize(in, 4096))
	for {
		raw, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		kinds := map[string]struct {
			typ    interface{}
			inject func(typ interface{}) error
		}{
			"Job": {
				typ: &batchv1.Job{},
				inject: func(typ interface{}) error {
					o := typ.(*batchv1.Job)
					meta := &o.ObjectMeta
					if insertDeprecatedAlphaAnnotation {
						meta = &o.Spec.Template.ObjectMeta
					}
					return injectIntoPodTemplateSpec(defaultNamespacePolicy, p, meta, &o.Spec.Template.Spec)
				},
			},
			"DaemonSet": {
				typ: &v1beta1.DaemonSet{},
				inject: func(typ interface{}) error {
					o := typ.(*v1beta1.DaemonSet)
					meta := &o.ObjectMeta
					if insertDeprecatedAlphaAnnotation {
						meta = &o.Spec.Template.ObjectMeta
					}
					return injectIntoPodTemplateSpec(defaultNamespacePolicy, p, meta, &o.Spec.Template.Spec)
				},
			},
			"ReplicaSet": {
				typ: &v1beta1.ReplicaSet{},
				inject: func(typ interface{}) error {
					o := typ.(*v1beta1.ReplicaSet)
					meta := &o.ObjectMeta
					if insertDeprecatedAlphaAnnotation {
						meta = &o.Spec.Template.ObjectMeta
					}
					return injectIntoPodTemplateSpec(defaultNamespacePolicy, p, meta, &o.Spec.Template.Spec)
				},
			},
			"Deployment": {
				typ: &v1beta1.Deployment{},
				inject: func(typ interface{}) error {
					o := typ.(*v1beta1.Deployment)
					meta := &o.ObjectMeta
					if insertDeprecatedAlphaAnnotation {
						meta = &o.Spec.Template.ObjectMeta
					}
					return injectIntoPodTemplateSpec(defaultNamespacePolicy, p, meta, &o.Spec.Template.Spec)
				},
			},
			"ReplicationController": {
				typ: &v1.ReplicationController{},
				inject: func(typ interface{}) error {
					o := typ.(*v1.ReplicationController)
					meta := &o.ObjectMeta
					if insertDeprecatedAlphaAnnotation {
						meta = &o.Spec.Template.ObjectMeta
					}
					return injectIntoPodTemplateSpec(defaultNamespacePolicy, p, meta, &o.Spec.Template.Spec)
				},
			},
			"StatefulSet": {
				typ: &appsv1beta1.StatefulSet{},
				inject: func(typ interface{}) error {
					o := typ.(*appsv1beta1.StatefulSet)
					meta := &o.ObjectMeta
					if insertDeprecatedAlphaAnnotation {
						meta = &o.Spec.Template.ObjectMeta
					}
					return injectIntoPodTemplateSpec(defaultNamespacePolicy, p, meta, &o.Spec.Template.Spec)
				},
			},
		}
		var meta metav1.TypeMeta
		var updated []byte
		if err = yaml.Unmarshal(raw, &meta); err != nil {
			return err
		}
		if kind, ok := kinds[meta.Kind]; ok {
			if err = yaml.Unmarshal(raw, kind.typ); err != nil {
				return err
			}
			if err = kind.inject(kind.typ); err != nil {
				return err
			}
			if updated, err = yaml.Marshal(kind.typ); err != nil {
				return err
			}
		} else {
			updated = raw // unchanged
		}

		if _, err = out.Write(updated); err != nil {
			return err
		}
		if _, err = fmt.Fprint(out, "---\n"); err != nil {
			return err
		}
	}
	return nil
}
