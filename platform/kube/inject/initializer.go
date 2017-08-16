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
	"encoding/json"
	"time"

	"github.com/golang/glog"

	appsv1beta1 "k8s.io/api/apps/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/tools/version"
)

const initializerName = "sidecar.initializer.istio.io"

var ignoredNamespaces = []string{
	"kube-system",
	"kube-public",
}

// InitializerOptions stores the configurable options for an initializer
type InitializerOptions struct {
	// TODO - pull from ConfigMap?

	// ResyncPeriod specifies how frequently to retrieve the full list
	// of watched resources for initialization.
	ResyncPeriod time.Duration

	// Hub is the image registry used for the injected sidecar proxy
	// and init images, e.g. docker.io/istio.
	Hub string

	// Tag is the docker image tag used for the injected sidecar proxy
	// and init images.
	Tag string

	// Namespace is the Kubernetes namespace the initializer is
	// responsible for managing. The initializer will manage injection
	// for all namespaces if the value of v1.NamespaceAll is
	// specified.
	Namespace string

	// InjectionPolicy determines the default injection policy for
	// resources in the managed namespace.
	InjectionPolicy InjectionPolicy
}

// Initializer implements a k8s initializer for transparently
// injecting the sidecar into user resources. For each resource in the
// managed namespace, the initializer will remove itself from the
// pending list of initializers can optionally inject the sidecar
// based on the InjectionPolicy and per-resource policy (see
// istioSidecarAnnotationPolicyKey).
type Initializer struct {
	clientset   kubernetes.Interface
	mesh        *proxyconfig.ProxyMeshConfig
	controllers []cache.Controller
	options     InitializerOptions
	params      Params
}

// NewInitializer creates a new instance of the Istio sidecar initializer.
func NewInitializer(cl kubernetes.Interface, mesh *proxyconfig.ProxyMeshConfig, o InitializerOptions) *Initializer {
	i := &Initializer{
		clientset: cl,
		mesh:      mesh,
		options:   o,
		params: Params{
			InitImage:         InitImageName(o.Hub, o.Tag),
			ProxyImage:        ProxyImageName(o.Hub, o.Tag),
			Verbosity:         DefaultVerbosity,
			SidecarProxyUID:   DefaultSidecarProxyUID,
			EnableCoreDump:    true,
			Version:           version.Line(),
			Mesh:              mesh,
			MeshConfigMapName: "istio",
		},
	}

	kinds := []struct {
		resource   string
		getter     cache.Getter
		objType    runtime.Object
		initialize func(in interface{}) error
	}{
		{
			"deployments",
			i.clientset.AppsV1beta1().RESTClient(),
			&appsv1beta1.Deployment{},
			i.initializeDeployment,
		},
		{
			"statefulsets",
			i.clientset.AppsV1beta1().RESTClient(),
			&appsv1beta1.StatefulSet{},
			i.initializeStatefulSet,
		},
		{
			"jobs",
			i.clientset.BatchV1().RESTClient(),
			&batchv1.Job{},
			i.initializeJob,
		},
		{
			"daemonsets",
			i.clientset.ExtensionsV1beta1().RESTClient(),
			&v1beta1.DaemonSet{},
			i.initializeDaemonSet,
		},
		{
			"replicasets",
			i.clientset.ExtensionsV1beta1().RESTClient(),
			&v1beta1.ReplicaSet{},
			i.initializeReplicaSet,
		},
		{
			"replicationcontrollers",
			i.clientset.CoreV1().RESTClient(),
			&v1.ReplicationController{},
			i.initializeReplicationController,
		},
		{
			"pods",
			i.clientset.CoreV1().RESTClient(),
			&v1.Pod{},
			i.initializePod,
		},
	}

	for _, kind := range kinds {
		kind := kind // capture the current value of `kind`

		watchList := cache.NewListWatchFromClient(kind.getter, kind.resource,
			i.options.Namespace, fields.Everything())

		// Wrap the returned watchlist to workaround the inability to include
		// the `IncludeUninitialized` list option when setting up watch clients.
		includeUninitializedWatchList := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.IncludeUninitialized = true
				return watchList.List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.IncludeUninitialized = true
				return watchList.Watch(options)
			},
		}

		_, controller := cache.NewInformer(includeUninitializedWatchList, kind.objType, i.options.ResyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					if err := kind.initialize(obj); err != nil {
						glog.Errorf("Could not initialize %s: %v", kind.resource, err)
					}
				},
			},
		)
		i.controllers = append(i.controllers, controller)
	}
	return i
}

func (i *Initializer) hasIstioInitializerNext(objectMeta *metav1.ObjectMeta) bool {
	glog.V(2).Infof("ObjectMeta initializer info %v/%v policy:%q status:%q %v",
		objectMeta.Namespace, objectMeta.Name,
		objectMeta.Annotations[istioSidecarAnnotationPolicyKey],
		objectMeta.Annotations[istioSidecarAnnotationStatusKey],
		objectMeta.Initializers)

	if objectMeta.GetInitializers() == nil {
		return false
	}
	pendingInitializers := objectMeta.GetInitializers().Pending
	if len(pendingInitializers) == 0 {
		return false
	}
	if initializerName != pendingInitializers[0].Name {
		return false
	}

	return true
}

func (i *Initializer) modifyResource(objectMeta *metav1.ObjectMeta, templateObjectMeta *metav1.ObjectMeta, spec *v1.PodSpec) error { // nolint: lll
	switch i.options.Namespace {
	case v1.NamespaceAll:
		// skip special kubernetes system namespaces
		for _, namespace := range ignoredNamespaces {
			if objectMeta.Namespace == namespace {
				return nil
			}
		}
	case objectMeta.Namespace:
		// Don't skip. The initializer should initialize this resource.
	default:
		// Skip namespace(s) that we're not responsible for
		// initializing.
		return nil
	}

	// Remove self from the list of pending Initializers while
	// preserving ordering.
	if pending := objectMeta.GetInitializers().Pending; len(pending) == 1 {
		objectMeta.Initializers = nil
	} else {
		objectMeta.Initializers.Pending = append(pending[:0], pending[1:]...)
	}

	if !injectRequired(i.options.InjectionPolicy, objectMeta) {
		glog.V(2).Infof("Skipping %s/%s", objectMeta.Namespace, objectMeta.Name)
		return nil
	}

	glog.Infof("Initializing %s/%s", objectMeta.Namespace, objectMeta.Name)

	if err := injectIntoSpec(&i.params, spec); err != nil {
		return err
	}

	addAnnotation(objectMeta, i.params.Version)

	// templated annotation to avoid double-injection
	if templateObjectMeta != nil {
		addAnnotation(templateObjectMeta, i.params.Version)
	}

	return nil
}

func (i *Initializer) createTwoWayMergePatch(prev, curr interface{}, dataStruct interface{}) ([]byte, error) {
	prevData, err := json.Marshal(prev)
	if err != nil {
		return nil, err
	}
	currData, err := json.Marshal(curr)
	if err != nil {
		return nil, err
	}
	return strategicpatch.CreateTwoWayMergePatch(prevData, currData, dataStruct)
}

func (i *Initializer) initializeDeployment(obj interface{}) error {
	in := obj.(*appsv1beta1.Deployment)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing deployment: %s", in.Name)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*appsv1beta1.Deployment)

	if err = i.modifyResource(&out.ObjectMeta, &out.Spec.Template.ObjectMeta, &out.Spec.Template.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, appsv1beta1.Deployment{})
	if err != nil {
		return err
	}
	_, err = i.clientset.AppsV1beta1().Deployments(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

func (i *Initializer) initializeStatefulSet(obj interface{}) error {
	in := obj.(*appsv1beta1.StatefulSet)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing statefulset: %s", in.Name)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*appsv1beta1.StatefulSet)

	if err = i.modifyResource(&out.ObjectMeta, &out.Spec.Template.ObjectMeta, &out.Spec.Template.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, appsv1beta1.StatefulSet{})
	if err != nil {
		return err
	}
	_, err = i.clientset.AppsV1beta1().StatefulSets(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

func (i *Initializer) initializeJob(obj interface{}) error {
	in := obj.(*batchv1.Job)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing job: %s", in.Name)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*batchv1.Job)

	if err = i.modifyResource(&out.ObjectMeta, &out.Spec.Template.ObjectMeta, &out.Spec.Template.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, batchv1.Job{})
	if err != nil {
		return err
	}
	_, err = i.clientset.BatchV1().Jobs(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

func (i *Initializer) initializeDaemonSet(obj interface{}) error {
	in := obj.(*v1beta1.DaemonSet)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing daemonset: %s", in.Name)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*v1beta1.DaemonSet)

	if err = i.modifyResource(&out.ObjectMeta, &out.Spec.Template.ObjectMeta, &out.Spec.Template.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, v1beta1.DaemonSet{})
	if err != nil {
		return err
	}
	_, err = i.clientset.ExtensionsV1beta1().DaemonSets(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

func (i *Initializer) initializeReplicaSet(obj interface{}) error {
	in := obj.(*v1beta1.ReplicaSet)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing replicaset: %s", in.Name)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*v1beta1.ReplicaSet)

	if err = i.modifyResource(&out.ObjectMeta, &out.Spec.Template.ObjectMeta, &out.Spec.Template.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, v1beta1.ReplicaSet{})
	if err != nil {
		return err
	}
	_, err = i.clientset.ExtensionsV1beta1().ReplicaSets(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

func (i *Initializer) initializeReplicationController(obj interface{}) error {
	in := obj.(*v1.ReplicationController)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing replicationcontroller: %s", in.Name)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*v1.ReplicationController)

	if err = i.modifyResource(&out.ObjectMeta, &out.Spec.Template.ObjectMeta, &out.Spec.Template.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, v1.ReplicationController{})
	if err != nil {
		return err
	}
	_, err = i.clientset.CoreV1().ReplicationControllers(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

func (i *Initializer) initializePod(obj interface{}) error {
	in := obj.(*v1.Pod)
	if !i.hasIstioInitializerNext(&in.ObjectMeta) {
		return nil
	}

	glog.Infof("Initializing pod: %v", in)

	o, err := runtime.NewScheme().DeepCopy(in)
	if err != nil {
		return err
	}
	out := o.(*v1.Pod)

	if err = i.modifyResource(&out.ObjectMeta, nil, &out.Spec); err != nil {
		return err
	}
	patchBytes, err := i.createTwoWayMergePatch(in, out, v1.Pod{})
	if err != nil {
		return err
	}
	_, err = i.clientset.CoreV1().Pods(in.Namespace).
		Patch(in.Name, types.StrategicMergePatchType, patchBytes)
	return err
}

// Run runs the Initializer controller.
func (i *Initializer) Run(stopCh <-chan struct{}) {
	glog.Info("Starting Istio sidecar initializer...")
	glog.Infof("Initializer name set to: %s", initializerName)
	glog.Infof("Options: %v", i.options)

	for _, controller := range i.controllers {
		go controller.Run(stopCh)
	}
}
