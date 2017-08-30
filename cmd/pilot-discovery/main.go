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

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"

	proxyconfig "istio.io/api/proxy/v1/config"
	configaggregate "istio.io/pilot/adapter/config/aggregate"
	"istio.io/pilot/adapter/config/crd"
	"istio.io/pilot/adapter/config/ingress"
	registryaggregate "istio.io/pilot/adapter/serviceregistry/aggregate"
	"istio.io/pilot/cmd"
	"istio.io/pilot/model"
	"istio.io/pilot/platform"
	"istio.io/pilot/platform/consul"
	"istio.io/pilot/platform/kube"
	"istio.io/pilot/proxy"
	"istio.io/pilot/proxy/envoy"
	"istio.io/pilot/tools/version"
)

// ConsulArgs store the args related to Consul configuration
type ConsulArgs struct {
	config    string
	serverURL string
}

type args struct {
	kubeconfig string
	meshconfig string

	// ingress sync mode is set to off by default
	controllerOptions kube.ControllerOptions
	discoveryOptions  envoy.DiscoveryServiceOptions

	registries []string
	consulargs ConsulArgs
}

var (
	flags args

	rootCmd = &cobra.Command{
		Use:   "pilot",
		Short: "Istio Pilot",
		Long:  "Istio Pilot provides management plane functionality to the Istio service mesh and Istio Mixer.",
	}

	discoveryCmd = &cobra.Command{
		Use:   "discovery",
		Short: "Start Istio proxy discovery service",
		RunE: func(c *cobra.Command, args []string) error {
			// receive mesh configuration
			mesh, err := cmd.ReadMeshConfig(flags.meshconfig)
			if err != nil {
				defaultMesh := proxy.DefaultMeshConfig()
				mesh = &defaultMesh
				glog.Warningf("failed to read mesh configuration, using default: %v", err)
			}
			glog.V(2).Infof("mesh configuration %s", spew.Sdump(mesh))

			stop := make(chan struct{})
			configClient, err := crd.NewClient(flags.kubeconfig, model.ConfigDescriptor{
				model.RouteRule,
				model.DestinationPolicy,
			})
			if err != nil {
				return multierror.Prefix(err, "failed to open a config client.")
			}

			if err = configClient.RegisterResources(); err != nil {
				return multierror.Prefix(err, "failed to register custom resources.")
			}
			configController := crd.NewController(configClient, flags.controllerOptions)
			environment := proxy.Environment{
				Mesh:             mesh,
				IstioConfigStore: model.MakeIstioStore(configController),
			}

			serviceControllers := make(map[platform.ServiceRegistry]registryaggregate.Registry)
			for _, r := range flags.registries {
				serviceRegistry := platform.ServiceRegistry(r)
				if _, exists := serviceControllers[serviceRegistry]; exists {
					return multierror.Prefix(err, r+" registry specified multiple times.")
				}

				glog.V(2).Infof("Adding %s registry adapter", serviceRegistry)
				switch serviceRegistry {
				case platform.KubernetesRegistry:

					var client kubernetes.Interface
					_, client, err = kube.CreateInterface(flags.kubeconfig)
					if err != nil {
						return multierror.Prefix(err, "failed to connect to Kubernetes API.")
					}

					if flags.controllerOptions.Namespace == "" {
						flags.controllerOptions.Namespace = os.Getenv("POD_NAMESPACE")
					}

					glog.V(2).Infof("version %s", version.Line())
					glog.V(2).Infof("flags %s", spew.Sdump(flags))

					serviceControllers[serviceRegistry] = kube.NewController(client, mesh, flags.controllerOptions)
					if mesh.IngressControllerMode != proxyconfig.ProxyMeshConfig_OFF {
						configController, err = configaggregate.MakeCache([]model.ConfigStoreCache{
							configController,
							ingress.NewController(client, mesh, flags.controllerOptions),
						})
						if err != nil {
							return err
						}
					}

					environment.SecretRegistry = kube.MakeSecretRegistry(client)
					ingressSyncer := ingress.NewStatusSyncer(mesh, client, flags.controllerOptions)

					go ingressSyncer.Run(stop)
				case platform.ConsulRegistry:
					glog.V(2).Infof("Adding %s registry adapter", serviceRegistry)
					glog.V(2).Infof("Consul url: %v", flags.consulargs.serverURL)

					serviceControllers[serviceRegistry], err = consul.NewController(
						flags.consulargs.serverURL, "dc1", 2*time.Second)
					if err != nil {
						return fmt.Errorf("failed to create Consul controller: %v", err)
					}

				default:
					return multierror.Prefix(err, "Service registry "+r+" is not supported.")
				}
			}

			regAggregate := registryaggregate.NewController(serviceControllers)
			environment.ServiceDiscovery = regAggregate
			environment.ServiceAccounts = regAggregate

			discovery, err := envoy.NewDiscoveryService(
				regAggregate,
				configController,
				environment,
				flags.discoveryOptions)
			if err != nil {
				return fmt.Errorf("failed to create discovery service: %v", err)
			}

			go regAggregate.Run(stop)
			go configController.Run(stop)
			go discovery.Run()
			cmd.WaitSignal(stop)
			return nil
		},
	}
)

func init() {
	discoveryCmd.PersistentFlags().StringSliceVar(&flags.registries, "registries",
		[]string{string(platform.KubernetesRegistry)},
		fmt.Sprintf("Comma separated list of service registries to read from (choose one or more from {%s, %s, %s})",
			platform.KubernetesRegistry, platform.ConsulRegistry, platform.EurekaRegistry))
	discoveryCmd.PersistentFlags().StringVar(&flags.kubeconfig, "kubeconfig", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	discoveryCmd.PersistentFlags().StringVar(&flags.meshconfig, "meshConfig", "/etc/istio/config/mesh",
		fmt.Sprintf("File name for Istio mesh configuration"))
	discoveryCmd.PersistentFlags().StringVarP(&flags.controllerOptions.Namespace, "namespace", "n", "",
		"Internal use. Select a namespace for the controller loop. If not set, uses ${POD_NAMESPACE} environment variable")
	discoveryCmd.PersistentFlags().StringVarP(&flags.controllerOptions.AppNamespace, "app namespace", "a", "",
		"Restrict the applications namespace that controller manages, do n")
	discoveryCmd.PersistentFlags().DurationVar(&flags.controllerOptions.ResyncPeriod, "resync", time.Second,
		"Controller resync interval")
	discoveryCmd.PersistentFlags().StringVar(&flags.controllerOptions.DomainSuffix, "domain", "cluster.local",
		"DNS domain suffix")

	discoveryCmd.PersistentFlags().IntVar(&flags.discoveryOptions.Port, "port", 8080,
		"Discovery service port")
	discoveryCmd.PersistentFlags().BoolVar(&flags.discoveryOptions.EnableProfiling, "profile", true,
		"Enable profiling via web interface host:port/debug/pprof")
	discoveryCmd.PersistentFlags().BoolVar(&flags.discoveryOptions.EnableCaching, "discovery_cache", true,
		"Enable caching discovery service responses")
	discoveryCmd.PersistentFlags().StringVar(&flags.consulargs.config, "consulconfig", "",
		"Consul Config file for discovery")
	discoveryCmd.PersistentFlags().StringVar(&flags.consulargs.serverURL, "consulserverURL", "",
		"URL for the consul server")

	cmd.AddFlags(rootCmd)

	rootCmd.AddCommand(discoveryCmd)
	rootCmd.AddCommand(cmd.VersionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		glog.Error(err)
		os.Exit(-1)
	}
}
