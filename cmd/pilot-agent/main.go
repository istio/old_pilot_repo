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
	"fmt"
	"os"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	"istio.io/pilot/cmd"
	"istio.io/pilot/model"
	"istio.io/pilot/platform"
	"istio.io/pilot/proxy"
	"istio.io/pilot/proxy/envoy"
	"istio.io/pilot/tools/version"
)

var (
	role            proxy.Node
	serviceregistry platform.ServiceRegistry
	config          string

	rootCmd = &cobra.Command{
		Use:   "agent",
		Short: "Istio Pilot agent",
		Long:  "Istio Pilot provides management plane functionality to the Istio service mesh and Istio Mixer.",
	}

	proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Envoy proxy agent",
		RunE: func(c *cobra.Command, args []string) error {
			glog.V(2).Infof("version %s", version.Line())
			role.Type = proxy.Sidecar
			if len(args) > 0 {
				role.Type = proxy.NodeType(args[0])
			}

			// set values from registry platform
			if role.IPAddress == "" {
				if serviceregistry == platform.KubernetesRegistry {
					role.IPAddress = os.Getenv("INSTANCE_IP")
				} else if serviceregistry == platform.ConsulRegistry {
					ipAddr := "127.0.0.1"
					if ok := proxy.WaitForPrivateNetwork(); ok {
						ipAddr = proxy.GetPrivateIP().String()
						glog.V(2).Infof("obtained private IP %v", ipAddr)
					}

					role.IPAddress = ipAddr
				}
			}
			if role.ID == "" {
				if serviceregistry == platform.KubernetesRegistry {
					role.ID = os.Getenv("POD_NAME") + "." + os.Getenv("POD_NAMESPACE")
				} else if serviceregistry == platform.ConsulRegistry {
					role.ID = role.IPAddress + ".service.consul"
				}
			}
			if role.Domain == "" {
				if serviceregistry == platform.KubernetesRegistry {
					role.Domain = os.Getenv("POD_NAMESPACE") + ".svc.cluster.local"
				} else if serviceregistry == platform.ConsulRegistry {
					role.Domain = "service.consul"
				}
			}

			glog.V(2).Infof("proxy role: %#v", role)

			proxyConfig := proxy.DefaultProxyConfig()
			if config != "" {
				if err := model.ApplyYAML(config, &proxyConfig); err != nil {
					return multierror.Prefix(err, "failed to apply config YAML: ")
				}
			}

			if proxyConfig.StatsdUdpAddress != "" {
				if addr, err := proxy.ResolveStatsdAddr(proxyConfig.StatsdUdpAddress); err == nil {
					proxyConfig.StatsdUdpAddress = addr
				} else {
					return err
				}
			}

			glog.V(2).Infof("effective config: %#v", proxyConfig)

			certs := []envoy.CertSource{
				{
					Directory: proxy.AuthCertsPath,
					Files:     []string{proxy.CertChainFilename, proxy.KeyFilename, proxy.RootCertFilename},
				},
				{
					Directory: proxy.IngressCertsPath,
					Files:     []string{"tls.crt", "tls.key"},
				},
			}
			envoyProxy := envoy.NewProxy(proxyConfig, role.ServiceNode())
			agent := proxy.NewAgent(envoyProxy, proxy.DefaultRetry)
			watcher := envoy.NewWatcher(proxyConfig, agent, role, certs)
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
	proxyCmd.PersistentFlags().StringVar(&config, "config", "",
		"Configuration overrides for the proxy as YAML/JSON")
	proxyCmd.PersistentFlags().StringVar((*string)(&serviceregistry), "serviceregistry",
		string(platform.KubernetesRegistry),
		fmt.Sprintf("Select the platform for service registry, options are {%s, %s}",
			string(platform.KubernetesRegistry), string(platform.ConsulRegistry)))
	proxyCmd.PersistentFlags().StringVar(&role.IPAddress, "ip", "",
		"Proxy IP address. If not provided uses ${INSTANCE_IP} environment variable.")
	proxyCmd.PersistentFlags().StringVar(&role.ID, "id", "",
		"Proxy unique ID. If not provided uses ${POD_NAME}.${POD_NAMESPACE} from environment variables")
	proxyCmd.PersistentFlags().StringVar(&role.Domain, "domain", "",
		"DNS domain suffix. If not provided uses ${POD_NAMESPACE}.svc.cluster.local")

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
