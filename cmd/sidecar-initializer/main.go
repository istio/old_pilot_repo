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

import (
	"fmt"
	"os"
	"time"

	"k8s.io/api/core/v1"

	"istio.io/pilot/cmd"
	"istio.io/pilot/platform/kube"
	"istio.io/pilot/platform/kube/inject"
	"istio.io/pilot/tools/version"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
)

func getRootCmd() *cobra.Command {
	flags := struct {
		kubeconfig   string
		meshconfig   string
		hub          string
		tag          string
		namespace    string
		policy       string
		resyncPeriod time.Duration
	}{}

	root := &cobra.Command{
		Use:   "sidecar-initializer",
		Short: "Kubernetes initializer for Istio sidecar",
		RunE: func(*cobra.Command, []string) error {
			switch inject.InjectionPolicy(flags.policy) {
			case inject.InjectionPolicyOff, inject.InjectionPolicyOptIn, inject.InjectionPolicyOptOut:
			default:
				return fmt.Errorf("unknown namespace injection policy: %v", flags.policy)
			}

			client, err := kube.CreateInterface(flags.kubeconfig)
			if err != nil {
				return multierror.Prefix(err, "failed to connect to Kubernetes API.")
			}

			glog.V(2).Infof("version %s", version.Line())
			glog.V(2).Infof("flags %s", spew.Sdump(flags))

			// receive mesh configuration
			mesh, err := cmd.ReadMeshConfig(flags.meshconfig)
			if err != nil {
				return multierror.Prefix(err, "failed to read mesh configuration.")
			}

			options := inject.InitializerOptions{
				ResyncPeriod: flags.resyncPeriod,
				Hub:          flags.hub,
				Tag:          flags.tag,
				Namespace:    flags.namespace,

				InjectionPolicy: inject.InjectionPolicy(flags.policy),
			}
			initializer := inject.NewInitializer(client, mesh, options)

			stop := make(chan struct{})
			go initializer.Run(stop)
			cmd.WaitSignal(stop)

			return nil
		},
	}

	root.PersistentFlags().StringVar(&flags.hub, "hub", "docker.io/istio", "Docker hub")
	root.PersistentFlags().StringVar(&flags.tag, "tag", "0.1", "Docker tag")
	root.PersistentFlags().StringVar(&flags.namespace, "namespace",
		v1.NamespaceDefault, "Namespace managed by initializer")
	root.PersistentFlags().StringVar(&flags.policy, "policy",
		string(inject.InjectionPolicyOff), "default injection policy")

	root.PersistentFlags().StringVar(&flags.kubeconfig, "kubeconfig", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	root.PersistentFlags().StringVar(&flags.meshconfig, "meshConfig", "/etc/istio/config/mesh",
		fmt.Sprintf("File name for Istio mesh configuration"))
	root.PersistentFlags().DurationVar(&flags.resyncPeriod, "resync", time.Duration(0),
		"Initializers resync interval")

	cmd.AddFlags(root)

	return root
}

func main() {
	if err := getRootCmd().Execute(); err != nil {
		os.Exit(-1)
	}
}
