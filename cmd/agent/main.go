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
	"errors"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/cmd"
	"istio.io/pilot/proxy"
	"istio.io/pilot/proxy/envoy"
	"istio.io/pilot/tools/version"
)

var (
	meshconfig string
	sidecar    proxy.Sidecar

	rootCmd = &cobra.Command{
		Use:   "agent",
		Short: "Istio Pilot agentt",
		Long:  "Istio Pilot provides management plane functionality to the Istio service mesh and Istio Mixer.",
	}

	proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Envoy proxy agent",
		RunE: func(c *cobra.Command, args []string) error {
			// set values from environment variables
			if sidecar.IPAddress == "" {
				sidecar.IPAddress = os.Getenv("POD_IP")
			}
			if sidecar.InstanceName == "" {
				sidecar.InstanceName = os.Getenv("POD_NAME")
			}
			if sidecar.InstanceNamespace == "" {
				sidecar.InstanceNamespace = os.Getenv("POD_NAMESPACE")
			}

			glog.V(2).Infof("version %s", version.Line())

			// receive mesh configuration
			mesh, err := cmd.ReadMeshConfig(meshconfig)
			if err != nil {
				return multierror.Prefix(err, "failed to read mesh configuration.")
			}

			glog.V(2).Infof("mesh configuration %s", spew.Sdump(mesh))

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

			watcher := envoy.NewWatcher(mesh, role, nil)
			ctx, cancel := context.WithCancel(context.Background())
			go watcher.Run(ctx)

			stop := make(chan struct{})
			cmd.WaitSignal(stop)
			<-stop
			cancel()
			return nil
		},
	}
)

func init() {
	proxyCmd.PersistentFlags().StringVar(&meshconfig, "meshconfig", "/etc/istio/mesh",
		fmt.Sprintf("File name for Istio mesh configuration"))
	proxyCmd.PersistentFlags().StringVar(&sidecar.IPAddress, "ipAddress", "",
		"Sidecar IP address. If not provided uses ${POD_IP} environment variable.")
	proxyCmd.PersistentFlags().StringVar(&sidecar.InstanceName, "podName", "",
		"Sidecar pod name. If not provided uses ${POD_NAME} environment variable")
	proxyCmd.PersistentFlags().StringVar(&sidecar.InstanceNamespace, "podNamespace", "",
		"Sidecar pod namespace. If not provided uses ${POD_NAMESPACE} environment variable")

	//sidecarCmd.PersistentFlags().IntSliceVar(&passthrough, "passthrough", nil,
	//	"Passthrough ports for health checks")

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
