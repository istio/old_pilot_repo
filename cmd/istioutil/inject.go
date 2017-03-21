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

package main

// NOTE: This tool only exists because kubernetes does not support
// dynamic/out-of-tree admission controller for transparent proxy
// injection. This file should be removed as soon as a proper kubernetes
// admission controller is written for istio.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	yamlDecoder "k8s.io/client-go/pkg/util/yaml"
)

const (
	defaultInitImage    = "gcr.io/istio-testing/init:latest"
	defaultRuntimeImage = "gcr.io/istio-testing/runtime:latest"

	istioSidecarAnnotationKey   = "alpha.istio.io/sidecar"
	istioSidecarAnnotationValue = "injected"

	initContainerName           = "istio-init"
	runtimeContainerName        = "istio-proxy"
	defaultManagerDiscoveryPort = 8080
	defaultMixerPort            = 9091
	defaultSidecarProxyUID      = int64(1337)
	defaultRuntimeVerbosity     = 2
)

var (
	initImage        string
	runtimeImage     string
	inFilename       string
	outFilename      string
	runtimeVerbosity int
	discoveryPort    int
	mixerPort        int
	sidecarProxyUID  int64

	injectCmd = &cobra.Command{
		Use:   "inject",
		Short: "Inject istio runtime into existing kubernete resources",
		RunE: func(_ *cobra.Command, _ []string) error {
			if inFilename == "" {
				return errors.New("filename not specified (see --filename or -f)")
			}
			var reader io.Reader
			if inFilename == "-" {
				reader = os.Stdin
			} else {
				var err error
				reader, err = os.Open(inFilename)
				if err != nil {
					return err
				}
			}

			var writer io.Writer
			if outFilename == "" {
				writer = os.Stdout
			} else {
				file, err := os.Create(outFilename)
				if err != nil {
					return err
				}
				writer = file
				defer file.Close()
			}

			return injectInfoFile(reader, writer)
		},
	}
)

func init() {
	injectCmd.PersistentFlags().StringVar(&initImage, "initImage", defaultInitImage, "Istio init image")
	injectCmd.PersistentFlags().StringVar(&runtimeImage, "runtimeImage", defaultRuntimeImage, "Istio runtime image")
	injectCmd.PersistentFlags().StringVarP(&inFilename, "filename", "f", "", "Input kubernetes resource filename")
	injectCmd.PersistentFlags().StringVarP(&outFilename, "output", "o", "", "Modified output kubernetes resource filename")
	injectCmd.PersistentFlags().IntVar(&discoveryPort, "discoveryPort", defaultManagerDiscoveryPort, "Manager discovery port")
	injectCmd.PersistentFlags().IntVar(&mixerPort, "mixerPort", defaultMixerPort, "Mixer port")
	injectCmd.PersistentFlags().IntVar(&runtimeVerbosity, "verbosity", defaultRuntimeVerbosity, "Runtime verbosity")
	injectCmd.PersistentFlags().Int64Var(&sidecarProxyUID, "sidecarProxyUID", defaultSidecarProxyUID, "Sidecar proxy UID")

	rootCmd.AddCommand(injectCmd)
}

func injectIntoPodTemplateSpec(t *v1.PodTemplateSpec) error {
	if t.Annotations == nil {
		t.Annotations = make(map[string]string)
	} else if _, ok := t.Annotations[istioSidecarAnnotationKey]; ok {
		// Return unmodified resource if sidecar is already present.
		return nil
	}
	t.Annotations[istioSidecarAnnotationKey] = istioSidecarAnnotationValue

	// init-container
	var annotations []interface{}
	if initContainer, ok := t.Annotations["pod.beta.kubernetes.io/init-containers"]; ok {
		if err := json.Unmarshal([]byte(initContainer), &annotations); err != nil {
			return err
		}
	}
	annotations = append(annotations,
		map[string]interface{}{
			"name":            initContainerName,
			"image":           initImage,
			"imagePullPolicy": "Always",
			"securityContext": map[string]interface{}{
				"capabilities": map[string]interface{}{
					"add": []string{"NET_ADMIN"},
				},
			},
		},
	)
	initAnnotationValue, err := json.Marshal(&annotations)
	if err != nil {
		return err
	}
	t.Annotations["pod.beta.kubernetes.io/init-containers"] = string(initAnnotationValue)

	// sidecar proxy container
	t.Spec.Containers = append(t.Spec.Containers,
		v1.Container{
			Name:  runtimeContainerName,
			Image: runtimeImage,
			Args: []string{
				"proxy",
				"sidecar",
				"-s", "manager:" + strconv.Itoa(discoveryPort),
				"-m", "mixer:" + strconv.Itoa(mixerPort),
				"-n", "$(POD_NAMESPACE)",
				"-v", strconv.Itoa(runtimeVerbosity),
			},
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
				Name: "POD_IP",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			}},
			ImagePullPolicy: v1.PullAlways,
			SecurityContext: &v1.SecurityContext{
				RunAsUser: &sidecarProxyUID,
			},
		},
	)
	return nil
}

func injectInfoFile(in io.Reader, out io.Writer) error {
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
				typ: &v1beta1.Job{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(&((typ.(*v1beta1.Job)).Spec.Template))
				},
			},
			"DaemonSet": {
				typ: &v1beta1.DaemonSet{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(&((typ.(*v1beta1.DaemonSet)).Spec.Template))
				},
			},
			"ReplicaSet": {
				typ: &v1beta1.ReplicaSet{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(&((typ.(*v1beta1.ReplicaSet)).Spec.Template))
				},
			},
			"Deployment": {
				typ: &v1beta1.Deployment{},
				inject: func(typ interface{}) error {
					return injectIntoPodTemplateSpec(&((typ.(*v1beta1.Deployment)).Spec.Template))
				},
			},
		}
		var updated []byte
		var meta metav1.TypeMeta
		if err := yaml.Unmarshal(raw, &meta); err != nil {
			return err
		}
		if kind, ok := kinds[meta.Kind]; ok {
			if err := yaml.Unmarshal(raw, kind.typ); err != nil {
				return err
			}
			if err := kind.inject(kind.typ); err != nil {
				return err
			}
			var err error
			if updated, err = yaml.Marshal(kind.typ); err != nil {
				return err
			}
		} else {
			updated = raw // unchanged
		}

		if _, err = out.Write(updated); err != nil {
			return err
		}
		fmt.Fprint(out, "---\n")
	}
	return nil
}
