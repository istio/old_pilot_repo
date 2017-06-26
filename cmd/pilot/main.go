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

	"k8s.io/client-go/kubernetes"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/adapter/config/aggregate"
	"istio.io/pilot/adapter/config/ingress"
	"istio.io/pilot/adapter/config/tpr"
	"istio.io/pilot/cmd"
	"istio.io/pilot/model"
	"istio.io/pilot/platform/kube"
	"istio.io/pilot/platform/vms"
	"istio.io/pilot/proxy"
	"istio.io/pilot/proxy/envoy"
	"istio.io/pilot/tools/version"

	vmsclient "github.com/amalgam8/amalgam8/registry/client"
	vmsconfig "github.com/amalgam8/amalgam8/sidecar/config"
	"github.com/amalgam8/amalgam8/sidecar/identity"
	"github.com/amalgam8/amalgam8/sidecar/register"
)

// Adapter defines options for underlying platform
type Adapter string

const (
	KubernetesAdapter Adapter = "Kubernetes"
	VMsAdapter        Adapter = "VMs"
)

// store the args related to VMs configuration
type VMsArgs struct {
	config    string
	serverURL string
	authToken string
}

// store the args related to K8s configuration
type KubeConfig struct {
}

type args struct {
	kubeconfig string
	meshConfig string

	ipAddress   string
	podName     string
	passthrough []int

	// ingress sync mode is set to off by default
	controllerOptions kube.ControllerOptions
	discoveryOptions  envoy.DiscoveryServiceOptions

	adapter Adapter

	vmsArgs VMsArgs
}

var (
	flags     args
	client    kubernetes.Interface
	vmsClient *vmsclient.Client
	vmsConfig vmsconfig.Config
	mesh      *proxyconfig.ProxyMeshConfig

	rootCmd = &cobra.Command{
		Use:   "pilot",
		Short: "Istio Pilot",
		Long:  "Istio Pilot provides management plane functionality to the Istio service mesh and Istio Mixer.",
		PersistentPreRunE: func(*cobra.Command, []string) (err error) {
			if flags.adapter == "" {
				flags.adapter = KubernetesAdapter
			}

			if flags.adapter == KubernetesAdapter {
				if flags.kubeconfig == "" {
					if v := os.Getenv("KUBECONFIG"); v != "" {
						glog.V(2).Infof("Setting configuration from KUBECONFIG environment variable")
						flags.kubeconfig = v
					}
				}

				client, err = kube.CreateInterface(flags.kubeconfig)
				if err != nil {
					return multierror.Prefix(err, "failed to connect to Kubernetes API.")
				}

				// set values from environment variables
				if flags.ipAddress == "" {
					flags.ipAddress = os.Getenv("POD_IP")
				}
				if flags.podName == "" {
					flags.podName = os.Getenv("POD_NAME")
				}
				if flags.controllerOptions.Namespace == "" {
					flags.controllerOptions.Namespace = os.Getenv("POD_NAMESPACE")
				}
				glog.V(2).Infof("version %s", version.Line())
				glog.V(2).Infof("flags %s", spew.Sdump(flags))

				// receive mesh configuration
				mesh, err = cmd.GetMeshConfig(client, flags.controllerOptions.Namespace, flags.meshConfig)
				if err != nil {
					return multierror.Prefix(err, "failed to retrieve mesh configuration.")
				}
			} else if flags.adapter == VMsAdapter {
				vmsConfig = *&vmsconfig.DefaultConfig
				err := vmsConfig.LoadFromFile(flags.vmsArgs.config)
				if err != nil {
					return multierror.Prefix(err, "failed to read vms config file.")
				}

				if flags.vmsArgs.serverURL != ""{
					vmsConfig.A8Registry.URL = flags.vmsArgs.serverURL
				}
				if flags.vmsArgs.authToken != ""{
					vmsConfig.A8Registry.Token = flags.vmsArgs.authToken
				}

				vmsClient, err = vmsclient.New(vmsclient.Config{
					URL:       vmsConfig.A8Registry.URL,
					AuthToken: vmsConfig.A8Registry.Token,
				})
				if err != nil {
					return multierror.Prefix(err, "failed to create VMs client.")
				}
				m := proxy.DefaultMeshConfig()
				mesh = &m
			}

			glog.V(2).Infof("mesh configuration %s", spew.Sdump(mesh))
			return
		},
	}

	discoveryCmd = &cobra.Command{
		Use:   "discovery",
		Short: "Start Istio proxy discovery service",
		RunE: func(c *cobra.Command, args []string) error {
			var discovery *envoy.DiscoveryService
			var err error
			stop := make(chan struct{})
			if flags.adapter == KubernetesAdapter {
				tprClient, err := tpr.NewClient(flags.kubeconfig, model.ConfigDescriptor{
					model.RouteRuleDescriptor,
					model.DestinationPolicyDescriptor,
				}, flags.controllerOptions.Namespace)
				if err != nil {
					return multierror.Prefix(err, "failed to open a TPR client")
				}

				if err = tprClient.RegisterResources(); err != nil {
					return multierror.Prefix(err, "failed to register Third-Party Resources.")
				}

				serviceController := kube.NewController(client, mesh, flags.controllerOptions)
				configController, err := aggregate.MakeCache([]model.ConfigStoreCache{
					tpr.NewController(tprClient, flags.controllerOptions.ResyncPeriod),
					ingress.NewController(client, mesh, flags.controllerOptions),
				})
				if err != nil {
					return err
				}
				ingressSyncer := ingress.NewStatusSyncer(mesh, client, flags.controllerOptions)
				context := &proxy.Context{
					Discovery:  serviceController,
					Accounts:   serviceController,
					Config:     model.MakeIstioStore(configController),
					MeshConfig: mesh,
				}
				discovery, err = envoy.NewDiscoveryService(serviceController, configController, context, flags.discoveryOptions)
				if err != nil {
					return fmt.Errorf("failed to create discovery service: %v", err)
				}

				go serviceController.Run(stop)
				go configController.Run(stop)
				go ingressSyncer.Run(stop)

			} else if flags.adapter == VMsAdapter {
				controller := vms.NewController(vms.ControllerConfig{
					Discovery: vmsClient,
					Mesh:      mesh,
				})
				context := &proxy.Context{
					Discovery:  controller,
					Accounts:   controller,
					Config:     model.MakeIstioStore(controller),
					MeshConfig: mesh,
				}
				discovery, err = envoy.NewDiscoveryService(controller, controller, context, flags.discoveryOptions)
				if err != nil {
					return fmt.Errorf("failed to create discovery service: %v", err)
				}

				go controller.Run(stop)
			}

			go discovery.Run()
			cmd.WaitSignal(stop)

			return nil
		},
	}

	proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Envoy agent",
	}

	sidecarCmd = &cobra.Command{
		Use:   "sidecar",
		Short: "Envoy sidecar agent",
		RunE: func(c *cobra.Command, args []string) error {
			var watcher envoy.Watcher
			stop := make(chan struct{})
			mesh.IngressControllerMode = proxyconfig.ProxyMeshConfig_OFF

			if flags.adapter == KubernetesAdapter {
				serviceController := kube.NewController(client, mesh, flags.controllerOptions)
				tprClient, err := tpr.NewClient(flags.kubeconfig, model.ConfigDescriptor{
					model.RouteRuleDescriptor,
					model.DestinationPolicyDescriptor,
				}, flags.controllerOptions.Namespace)
				if err != nil {
					return err
				}

				configController := tpr.NewController(tprClient, flags.controllerOptions.ResyncPeriod)
				context := &proxy.Context{
					Discovery:        serviceController,
					Accounts:         serviceController,
					Config:           model.MakeIstioStore(configController),
					MeshConfig:       mesh,
					IPAddress:        flags.ipAddress,
					UID:              fmt.Sprintf("kubernetes://%s.%s", flags.podName, flags.controllerOptions.Namespace),
					PassthroughPorts: flags.passthrough,
				}

				watcher, err = envoy.NewWatcher(serviceController, configController, context)
				if err != nil {
					return err
				}

				// must start watcher after starting dependent controllers
				go serviceController.Run(stop)
				go configController.Run(stop)
			} else if flags.adapter == VMsAdapter {
				controller := vms.NewController(vms.ControllerConfig{
					Discovery: vmsClient,
					Mesh:      mesh,
				})

				// Get app info from config file
				provider, err := identity.New(&vmsConfig, nil)
				if err != nil {
					return multierror.Prefix(err, "failed to create identity for service.")
				}
				id, err := provider.GetIdentity()
				if err != nil {
					return multierror.Prefix(err, "failed to get identity.")
				}
				// Create a registration agent for vms platform
				regAgent, err := register.NewRegistrationAgent(register.RegistrationConfig{
					Registry: vmsClient,
					Identity: provider,
				})
				if err != nil {
					return multierror.Prefix(err, "failed to create registration agent.")
				}
				mesh.IstioServiceCluster = id.ServiceName
				context := &proxy.Context{
					Discovery:        controller,
					Accounts:         controller,
					Config:           model.MakeIstioStore(controller),
					MeshConfig:       mesh,
					IPAddress:        id.Endpoint.Value,
					UID:              fmt.Sprintf("vms://%s", id.Endpoint.Value),
					Registration:     regAgent,
					PassthroughPorts: flags.passthrough,
				}

				watcher, err = envoy.NewWatcher(controller, controller, context)
				if err != nil {
					return err
				}

				go controller.Run(stop)
			}

			go watcher.Run(stop)
			cmd.WaitSignal(stop)

			return nil
		},
	}

	ingressCmd = &cobra.Command{
		Use:   "ingress",
		Short: "Envoy ingress agent",
		RunE: func(c *cobra.Command, args []string) error {
			watcher, err := envoy.NewIngressWatcher(mesh, kube.MakeSecretRegistry(client))
			if err != nil {
				return err
			}

			stop := make(chan struct{})
			go watcher.Run(stop)
			cmd.WaitSignal(stop)

			return nil
		},
	}

	egressCmd = &cobra.Command{
		Use:   "egress",
		Short: "Envoy external service agent",
		RunE: func(c *cobra.Command, args []string) error {
			watcher, err := envoy.NewEgressWatcher(mesh)
			if err != nil {
				return err
			}
			stop := make(chan struct{})
			go watcher.Run(stop)
			cmd.WaitSignal(stop)
			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information and exit",
		Run: func(*cobra.Command, []string) {
			fmt.Print(version.Version())
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar((*string)(&flags.adapter), "adapter", string(KubernetesAdapter),
		fmt.Sprintf("Select the underlying running platform, options are {%s, %s}", string(KubernetesAdapter),string(VMsAdapter)))
	rootCmd.PersistentFlags().StringVar(&flags.kubeconfig, "kubeconfig", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	rootCmd.PersistentFlags().StringVarP(&flags.controllerOptions.Namespace, "namespace", "n", "",
		"Select a namespace for the controller loop. If not set, uses ${POD_NAMESPACE} environment variable")
	rootCmd.PersistentFlags().DurationVar(&flags.controllerOptions.ResyncPeriod, "resync", time.Second,
		"Controller resync interval")
	rootCmd.PersistentFlags().StringVar(&flags.controllerOptions.DomainSuffix, "domainSuffix", "cluster.local",
		"Kubernetes DNS domain suffix")
	rootCmd.PersistentFlags().StringVar(&flags.meshConfig, "meshConfig", cmd.DefaultConfigMapName,
		fmt.Sprintf("ConfigMap name for Istio mesh configuration, config key should be %q", cmd.ConfigMapKey))

	discoveryCmd.PersistentFlags().IntVar(&flags.discoveryOptions.Port, "port", 8080,
		"Discovery service port")
	discoveryCmd.PersistentFlags().BoolVar(&flags.discoveryOptions.EnableProfiling, "profile", true,
		"Enable profiling via web interface host:port/debug/pprof")
	discoveryCmd.PersistentFlags().BoolVar(&flags.discoveryOptions.EnableCaching, "discovery_cache", true,
		"Enable caching discovery service responses")

	proxyCmd.PersistentFlags().StringVar(&flags.ipAddress, "ipAddress", "",
		"IP address. If not provided uses ${POD_IP} environment variable.")
	proxyCmd.PersistentFlags().StringVar(&flags.podName, "podName", "",
		"Pod name. If not provided uses ${POD_NAME} environment variable")

	sidecarCmd.PersistentFlags().IntSliceVar(&flags.passthrough, "passthrough", nil,
		"Passthrough ports for health checks")

	sidecarCmd.PersistentFlags().StringVar(&flags.vmsArgs.config, "config", "",
		"Config file for sidecar")

	sidecarCmd.PersistentFlags().StringVar(&flags.vmsArgs.serverURL, "serverURL", "",
		"URL for the registry server")

	sidecarCmd.PersistentFlags().StringVar(&flags.vmsArgs.config, "authToken", "",
		"Authorization token used to access the registry server")

	proxyCmd.AddCommand(sidecarCmd)
	proxyCmd.AddCommand(ingressCmd)
	proxyCmd.AddCommand(egressCmd)

	cmd.AddFlags(rootCmd)

	rootCmd.AddCommand(discoveryCmd)
	rootCmd.AddCommand(proxyCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		glog.Error(err)
		os.Exit(-1)
	}
}
