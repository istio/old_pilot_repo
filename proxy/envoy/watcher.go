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

package envoy

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/golang/glog"

	proxyconfig "istio.io/api/proxy/v1/config"
	"istio.io/pilot/proxy"
)

// Watcher observes service registry and triggers a reload on a change
type Watcher interface {
	Run(stop <-chan struct{})
}

type watcher struct {
	agent proxy.Agent
	mesh  *proxyconfig.ProxyMeshConfig
}

// NewWatcher creates a new watcher instance with an agent
func NewWatcher(mesh *proxyconfig.ProxyMeshConfig, role proxy.Role) Watcher {
	var node string
	switch role.(type) {
	case proxy.Sidecar:
		// Use proxy node IP as the node name
		// This parameter is used as the value for "service-node"
		node = role.(proxy.Sidecar).IPAddress
	case proxy.Egress:
		node = proxy.EgressNode
	default:
		node = "unknown"
	}
	glog.V(2).Infof("Proxy node: %s", node)

	if mesh.StatsdUdpAddress != "" {
		if addr, err := resolveStatsdAddr(mesh.StatsdUdpAddress); err == nil {
			mesh.StatsdUdpAddress = addr
		} else {
			glog.Warningf("Error resolving statsd address; clearing to prevent bad config: %v", err)
			mesh.StatsdUdpAddress = ""
		}
	}

	agent := proxy.NewAgent(runEnvoy(mesh, node), proxy.DefaultRetry)
	out := &watcher{
		agent: agent,
		mesh:  mesh,
	}

	return out
}

func (w *watcher) Run(stop <-chan struct{}) {
	// agent consumes notifications from the controllerr
	go w.agent.Run(stop)

	// kickstart the proxy with partial state (in case there are no notifications coming)
	w.reload()

	// monitor certificates
	if w.mesh.AuthPolicy == proxyconfig.ProxyMeshConfig_MUTUAL_TLS {
		go watchCerts(w.mesh.AuthCertsPath, stop, w.reload)
	}

	<-stop
}

func (w *watcher) reload() {
	config := buildConfig(make(Listeners, 0), make(Clusters, 0), true, w.mesh)
	if w.mesh.AuthPolicy == proxyconfig.ProxyMeshConfig_MUTUAL_TLS {
		config.Hash = generateCertHash(w.mesh.AuthCertsPath)
	}
	w.agent.ScheduleConfigUpdate(config)
}

const (
	// EpochFileTemplate is a template for the root config JSON
	EpochFileTemplate = "%s/envoy-rev%d.json"

	// BinaryPath is the path to envoy binary
	BinaryPath = "/usr/local/bin/envoy"

	// ConfigPath is the directory to hold enovy epoch configurations
	ConfigPath = "/etc/envoy"
)

func configFile(config string, epoch int) string {
	return fmt.Sprintf(EpochFileTemplate, config, epoch)
}

func envoyArgs(fname string, epoch int, mesh *proxyconfig.ProxyMeshConfig, node string) []string {
	return []string{"-c", fname,
		"--restart-epoch", fmt.Sprint(epoch),
		"--drain-time-s", fmt.Sprint(int(convertDuration(mesh.DrainDuration) / time.Second)),
		"--parent-shutdown-time-s", fmt.Sprint(int(convertDuration(mesh.ParentShutdownDuration) / time.Second)),
		"--service-cluster", mesh.IstioServiceCluster,
		"--service-node", node,
	}
}

func runEnvoy(mesh *proxyconfig.ProxyMeshConfig, node string) proxy.Proxy {
	return proxy.Proxy{
		Run: func(config interface{}, epoch int, abort <-chan error) error {
			envoyConfig, ok := config.(*Config)
			if !ok {
				return fmt.Errorf("Unexpected config type: %#v", config)
			}

			// attempt to write file
			fname := configFile(ConfigPath, epoch)
			if err := envoyConfig.WriteFile(fname); err != nil {
				return err
			}

			// spin up a new Envoy process
			args := envoyArgs(fname, epoch, mesh, node)

			// inject tracing flag for higher levels
			if glog.V(4) {
				args = append(args, "-l", "trace")
			} else if glog.V(3) {
				args = append(args, "-l", "debug")
			}

			glog.V(2).Infof("Envoy command: %v", args)

			/* #nosec */
			cmd := exec.Command(BinaryPath, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				return err
			}

			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case err := <-abort:
				glog.Warningf("Aborting epoch %d", epoch)
				if errKill := cmd.Process.Kill(); errKill != nil {
					glog.Warningf("killing epoch %d caused an error %v", epoch, errKill)
				}
				return err
			case err := <-done:
				return err
			}
		},
		Cleanup: func(epoch int) {
			path := configFile(ConfigPath, epoch)
			if err := os.Remove(path); err != nil {
				glog.Warningf("Failed to delete config file %s for %d, %v", path, epoch, err)
			}
		},
		Panic: func(_ interface{}) {
			glog.Fatal("cannot start the proxy with the desired configuration")
		},
	}
}
