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
	"context"
	"strings"
	"errors"
	"fmt"
	"os"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/cmd"
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

var (
	meshconfig string
	sidecar    proxy.Sidecar
	adapter    Adapter
	vmsArgs    VMsArgs

	rootCmd = &cobra.Command{
		Use:   "agent",
		Short: "Istio Pilot agent",
		Long:  "Istio Pilot provides management plane functionality to the Istio service mesh and Istio Mixer.",
	}

	proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Envoy proxy agent",
		RunE: func(c *cobra.Command, args []string) error {
			if adapter == KubernetesAdapter {
				// set values from environment variables
				if sidecar.IPAddress == "" {
					sidecar.IPAddress = os.Getenv("INSTANCE_IP")
				}
				if sidecar.ID == "" {
					sidecar.ID = os.Getenv("POD_NAME") + "." + os.Getenv("POD_NAMESPACE")
				}
				// receive mesh configuration
				mesh, err := cmd.ReadMeshConfig(meshconfig)
				if err != nil {
					return multierror.Prefix(err, "failed to read mesh configuration.")
				}

				glog.V(2).Infof("version %s", version.Line())
				glog.V(2).Infof("sidecar %#v", sidecar)
				glog.V(2).Infof("mesh configuration %#v", mesh)

				var role proxy.Role = sidecar
				if len(args) > 0 {
					switch args[0] {
					case proxy.EgressNode:
						if mesh.EgressProxyAddress == "" {
							return errors.New("egress proxy requires address configuration")
						}
						role = proxy.EgressRole{}

					case proxy.IngressNode:
						if mesh.IngressControllerMode == proxyconfig.ProxyMeshConfig_OFF {
							return errors.New("ingress proxy is disabled")
						}
						role = proxy.IngressRole{}

					default:
						return fmt.Errorf("failed to recognize proxy role %s", args[0])
					}
				}

				watcher := envoy.NewWatcher(mesh, role)
				ctx, cancel := context.WithCancel(context.Background())
				go watcher.Run(ctx)

				stop := make(chan struct{})
				cmd.WaitSignal(stop)
				<-stop
				cancel()
			} else if adapter == VMsAdapter {
				vmsConfig := *&vmsconfig.DefaultConfig
				if vmsArgs.config != "" {
					err := vmsConfig.LoadFromFile(vmsArgs.config)
					if err != nil {
						return multierror.Prefix(err, "failed to read vms config file.")
					}
				}
				if vmsArgs.serverURL != "" {
					vmsConfig.A8Registry.URL = vmsArgs.serverURL
				}
				if vmsArgs.authToken != "" {
					vmsConfig.A8Registry.Token = vmsArgs.authToken
				}

				mesh := &(proxy.DefaultMeshConfig())
				vmsClient, err := vmsclient.New(vmsclient.Config{
					URL:       vmsConfig.A8Registry.URL,
					AuthToken: vmsConfig.A8Registry.Token,
				})
				if err != nil {
					return multierror.Prefix(err, "failed to create VMs client.")
				}

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

				sidecar.IPAddress = strings.Split(id.Endpoint.Value, ":")[0]
				sidecar.Registration = regAgent

				var role proxy.Role = sidecar
				watcher := envoy.NewWatcher(mesh, role)
				ctx, cancel := context.WithCancel(context.Background())
				go watcher.Run(ctx)

				stop := make(chan struct{})
				cmd.WaitSignal(stop)
				<-stop
				cancel()
			}
			return nil
		},
	}
)

func init() {
	proxyCmd.PersistentFlags().StringVar((*string)(&adapter), "adapter", string(KubernetesAdapter),
		fmt.Sprintf("Select the underlying running platform, options are {%s, %s}", string(KubernetesAdapter), string(VMsAdapter)))
	proxyCmd.PersistentFlags().StringVar(&meshconfig, "meshconfig", "/etc/istio/config/mesh",
		fmt.Sprintf("File name for Istio mesh configuration"))
	proxyCmd.PersistentFlags().StringVar(&sidecar.IPAddress, "ip", "",
		"Sidecar proxy IP address. If not provided uses ${INSTANCE_IP} environment variable.")
	proxyCmd.PersistentFlags().StringVar(&sidecar.ID, "id", "",
		"Sidecar proxy unique ID. If not provided uses ${POD_NAME}.${POD_NAMESPACE} environment variables")
	proxyCmd.PersistentFlags().StringVar(&sidecar.Domain, "domain", "cluster.local",
		"DNS domain suffix")
	proxyCmd.PersistentFlags().StringVar(&vmsArgs.config, "config", "",
		"Config file for sidecar")

	proxyCmd.PersistentFlags().StringVar(&vmsArgs.serverURL, "serverURL", "",
		"URL for the registry server")

	proxyCmd.PersistentFlags().StringVar(&vmsArgs.config, "authToken", "",
		"Authorization token used to access the registry server")

	cmd.AddFlags(rootCmd)

	rootCmd.AddCommand(proxyCmd)
	rootCmd.AddCommand(cmd.VersionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		glog.Error(err)
		os.Exit(-1)
	}
}
