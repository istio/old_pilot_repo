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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		Long:  "Istio Pilot provides fleet-wide traffic management capabilities in the Istio Service Mesh.",
	}

	discoveryCmd = &cobra.Command{
		Use:   "discovery",
		Short: "Start Istio proxy discovery service",
		RunE: func(c *cobra.Command, args []string) error {

			// receive mesh configuration
			mesh, fail := cmd.ReadMeshConfig(flags.meshconfig)
			if fail != nil {
				defaultMesh := proxy.DefaultMeshConfig()
				mesh = &defaultMesh
				glog.Warningf("failed to read mesh configuration, using default: %v", fail)
			}
			environment := proxy.Environment{
				Mesh: mesh,
			}

			glog.V(2).Infof("mesh configuration %s", spew.Sdump(mesh))
			glog.V(2).Infof("version %s", version.Line())
			glog.V(2).Infof("flags %s", spew.Sdump(flags))

			stop := make(chan struct{})

			configClient, err := crd.NewClient(flags.kubeconfig, model.ConfigDescriptor{
				model.RouteRule,
				model.EgressRule,
				model.DestinationPolicy,
			}, flags.controllerOptions.DomainSuffix)
			if err != nil {
				return multierror.Prefix(err, "failed to open a config client.")
			}

			if err = configClient.RegisterResources(); err != nil {
				return multierror.Prefix(err, "failed to register custom resources.")
			}

			configController := crd.NewController(configClient, flags.controllerOptions)
			serviceControllers := make(map[platform.ServiceRegistry]registryaggregate.Registry)

			var regOrder []platform.ServiceRegistry

			for _, r := range flags.registries {
				serviceRegistry := platform.ServiceRegistry(r)
				if _, exists := serviceControllers[serviceRegistry]; exists {
					return multierror.Prefix(err, r+" registry specified multiple times.")
				}

				glog.V(2).Infof("Adding %s registry adapter", serviceRegistry)
				regOrder = append(regOrder, serviceRegistry)

				switch serviceRegistry {
				case platform.KubernetesRegistry:
					_, client, kuberr := kube.CreateInterface(flags.kubeconfig)
					if kuberr != nil {
						return multierror.Prefix(kuberr, "failed to connect to Kubernetes API.")
					}

					if flags.controllerOptions.Namespace == "" {
						flags.controllerOptions.Namespace = os.Getenv("POD_NAMESPACE")
					}

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

					ingressSyncer := ingress.NewStatusSyncer(mesh, client, flags.controllerOptions)

					go ingressSyncer.Run(stop)

				case platform.ConsulRegistry:
					glog.V(2).Infof("Consul url: %v", flags.consulargs.serverURL)
					var conerr error
					serviceControllers[serviceRegistry], conerr = consul.NewController(
						// TODO: Remove this hardcoding!
						flags.consulargs.serverURL, "dc1", 2*time.Second)
					if conerr != nil {
						return fmt.Errorf("failed to create Consul controller: %v", conerr)
					}

				default:
					return multierror.Prefix(err, "Service registry "+r+" is not supported.")
				}
			}

			environment.IstioConfigStore = model.MakeIstioStore(configController)
			regAggregate := registryaggregate.NewController(serviceControllers, regOrder)
			environment.ServiceDiscovery = regAggregate
			environment.ServiceAccounts = regAggregate

			// Set up discovery service
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
		fmt.Sprintf("Comma separated list of platform service registries to read from (choose one or more from {%s, %s, %s})",
			platform.KubernetesRegistry, platform.ConsulRegistry, platform.EurekaRegistry))
	discoveryCmd.PersistentFlags().StringVar(&flags.kubeconfig, "kubeconfig", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	discoveryCmd.PersistentFlags().StringVar(&flags.meshconfig, "meshConfig", "/etc/istio/config/mesh",
		fmt.Sprintf("File name for Istio mesh configuration"))
	discoveryCmd.PersistentFlags().StringVarP(&flags.controllerOptions.Namespace, "namespace", "n", "",
		"Select a namespace for the controller loop. If not set, uses ${POD_NAMESPACE} environment variable")
	discoveryCmd.PersistentFlags().StringVarP(&flags.controllerOptions.WatchedNamespace, "app namespace",
		"a", metav1.NamespaceAll,
		"Restrict the applications namespace the controller manages; if not set, controller watches all namespaces")
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

	_ = discoveryCmd.PersistentFlags().MarkHidden("namespace")
	_ = discoveryCmd.PersistentFlags().MarkHidden("profile")
	_ = discoveryCmd.PersistentFlags().MarkHidden("discovery_cache")
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
