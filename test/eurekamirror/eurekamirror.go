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
	"time"

	log "fmt"

	"flag"

	"istio.io/pilot/platform/kube"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"sync"

	"istio.io/pilot/model"
)

var (
	kubeconfig string
	eurekaURL  string
	namespace  string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "",
		"Use a Kubernetes configuration file instead of in-cluster configuration")
	flag.StringVar(&eurekaURL, "url", "",
		"Eureka server url")
	flag.StringVar(&namespace, "namespace", "",
		"Select a namespace for the controller loop. If not set, uses ${POD_NAMESPACE} environment variable")
}

type RegisterInstance struct {
	Instance Instance `json:"instance"`
}

type Instance struct {
	ID         string   `json:"instanceId,omitempty"`
	Hostname   string   `json:"hostName"`
	App        string   `json:"app"`
	IPAddress  string   `json:"ipAddr"`
	Port       Port     `json:"port,omitempty"`
	SecurePort Port     `json:"securePort,omitempty"`
	Metadata   Metadata `json:"metadata,omitempty"`
}

type Port struct {
	Port    int  `json:"$,string"`
	Enabled bool `json:"@enabled,string"`
}

type Metadata map[string]string

const (
	heartbeatInterval = 30 * time.Second
	retryInterval     = 10 * time.Second
	appPath           = "%s/eureka/v2/apps/%s"
	instancePath      = "%s/eureka/v2/apps/%s/%s"
)

// agent is a simple state machine that maintains an instance's registration with Eureka.
type agent struct {
	stop     <-chan struct{}
	client   http.Client
	url      string
	instance *Instance
}

// TODO: stop is synchronous, which may be slow
func (a *agent) Run(stop <-chan struct{}) {
	log.Printf("Starting registration agent for %s", a.instance.ID)
	a.stop = stop
	go a.unregistered()
	<-stop
}

func (a *agent) unregistered() {
	var retry, heartbeatDelay <-chan time.Time
	if err := a.register(); err != nil {
		log.Println(err)
		retry = time.After(retryInterval)
	} else {
		heartbeatDelay = time.After(heartbeatInterval)
	}

	select {
	case <-retry:
		go a.unregistered() // attempt to re-register
	case <-heartbeatDelay:
		go a.registered() // start heartbeating
	case <-a.stop:
		return
	}
}

func (a *agent) registered() {
	var retry, heartbeatDelay <-chan time.Time
	if err := a.heartbeat(); err != nil {
		log.Println(err)
		retry = time.After(retryInterval)
	} else {
		heartbeatDelay = time.After(heartbeatInterval)
	}

	select { // TODO: unregistered vs heartbeat failure
	case <-retry:
		go a.unregistered()
	case <-heartbeatDelay:
		go a.registered()
	case <-a.stop:
		// attempt to unregister before terminating
		if err := a.unregister(); err != nil {
			log.Println(err)
		}
		return
	}
}

func (a *agent) register() error {
	payload := RegisterInstance{Instance: *a.instance}
	data, err := json.Marshal(&payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, a.buildRegisterPath(), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code %s", resp.Status)
	}
	return nil
}

func (a *agent) heartbeat() error {
	req, err := http.NewRequest(http.MethodPut, a.buildInstancePath(), nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %s", resp.Status)
	}
	return nil
}

func (a *agent) unregister() error {
	req, err := http.NewRequest(http.MethodDelete, a.buildInstancePath(), nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to unregister %s, got %s", a.instance.ID, resp.Status)
	}
	return nil
}

func (a *agent) buildRegisterPath() string {
	return fmt.Sprintf(appPath, a.url, a.instance.App)
}

func (a *agent) buildInstancePath() string {
	return fmt.Sprintf(instancePath, a.url, a.instance.App, a.instance.ID)
}

type mirror struct {
	url      string
	podCache *podCache
	agents   map[string]map[string]chan struct{}
}

func newMirror(url string, podCache *podCache) *mirror {
	return &mirror{
		url:      url,
		podCache: podCache,
		agents:   make(map[string]map[string]chan struct{}),
	}
}

// TODO: logic for endpoint deletion
func (m *mirror) Sync(endpoints <-chan *v1.Endpoints) {
	for endpoint := range endpoints {
		instances := m.convertEndpoints(endpoint)

		newIDs := make(map[string]bool)
		for _, instance := range instances {
			newIDs[instance.ID] = true
		}

		agents, exists := m.agents[endpoint.Name]
		if !exists {
			m.agents[endpoint.Name] = make(map[string]chan struct{})
		}

		// remove instances that are gone
		toRemove := make([]string, 0)
		for id := range agents {
			if !newIDs[id] {
				toRemove = append(toRemove, id)
			}
		}
		for _, id := range toRemove {
			m.stopAgent(endpoint.Name, id)
		}

		// add instances that are new
		for _, instance := range instances {
			if _, exists := agents[instance.ID]; !exists {
				m.startAgent(endpoint.Name, instance)
			}
		}

		if len(agents) == 0 {
			delete(m.agents, endpoint.Name)
		}
	}

	// cleanup registration agents
	for name := range m.agents {
		for id := range m.agents[name] {
			m.stopAgent(name, id)
		}
	}
}

func (m *mirror) startAgent(name string, instance *Instance) {
	stop := make(chan struct{})
	m.agents[name][instance.ID] = stop
	a := &agent{
		url:      m.url,
		instance: instance,
		client: http.Client{
			Timeout: time.Second * 15,
		},
	}
	go a.Run(stop)
}

func (m *mirror) stopAgent(name, id string) {
	close(m.agents[name][id])
	delete(m.agents[name], id)
}

func (m *mirror) convertEndpoints(ep *v1.Endpoints) []*Instance {
	instances := make([]*Instance, 0)
	for _, ss := range ep.Subsets {
		for _, addr := range ss.Addresses {
			for _, port := range ss.Ports {
				metadata := make(Metadata)

				// add labels
				pod, exists := m.podCache.getByIP(addr.IP)
				if exists {
					for k, v := range pod.Labels {
						metadata[k] = v
					}
				}

				// add protocol labels
				protocol := kube.ConvertProtocol(port.Name, port.Protocol)
				switch protocol {
				case model.ProtocolUDP:
					metadata["istio.protocol"] = "udp"
				case model.ProtocolTCP:
					metadata["istio.protocol"] = "tcp"
				case model.ProtocolHTTP:
					metadata["istio.protocol"] = "http"
				case model.ProtocolHTTP2:
					metadata["istio.protocol"] = "http2"
				case model.ProtocolHTTPS:
					metadata["istio.protocol"] = "https"
				case model.ProtocolGRPC:
					metadata["istio.protocol"] = "grpc"
				}

				hostname := fmt.Sprintf("%s.%s.svc.cluster.local",
					ep.ObjectMeta.Name, ep.ObjectMeta.Namespace)

				instances = append(instances, &Instance{
					ID:        fmt.Sprintf("%s-%s-%d", ep.ObjectMeta.Name, addr.IP, port.Port),
					App:       ep.ObjectMeta.Name,
					Hostname:  hostname,
					IPAddress: addr.IP,
					Port: Port{
						Port:    int(port.Port),
						Enabled: true,
					},
					Metadata: metadata,
				})
			}
		}
	}
	return instances
}

type podCache struct {
	mutex sync.Mutex
	cache map[string]string
	store cache.Store
}

func newPodCache(informer cache.SharedIndexInformer) *podCache {
	c := &podCache{
		cache: make(map[string]string),
		store: informer.GetStore(),
	}

	informer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.add(obj.(*v1.Pod))
			},
			UpdateFunc: func(old, cur interface{}) {
				c.remove(old.(*v1.Pod))
				c.add(cur.(*v1.Pod))
			},
			DeleteFunc: func(obj interface{}) {
				c.remove(obj.(*v1.Pod))
			},
		},
	)

	return c
}

func (c *podCache) key(pod *v1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func (c *podCache) add(pod *v1.Pod) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if pod.Status.PodIP != "" {
		c.cache[pod.Status.PodIP] = c.key(pod)
	}
}

func (c *podCache) remove(pod *v1.Pod) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.cache, c.key(pod))
}

func (c *podCache) getByIP(addr string) (*v1.Pod, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	name, exists := c.cache[addr]
	if !exists {
		return nil, false
	}
	obj, exists, err := c.store.GetByKey(name)
	if err != nil {
		log.Println(err)
	}
	if !exists {
		return nil, false
	}
	return obj.(*v1.Pod), true
}

func main() {
	flag.Parse()

	_, client, err := kube.CreateInterface(kubeconfig)
	if err != nil {
		log.Println(err)
		return
	}
	endpoints := make(chan *v1.Endpoints, 1)

	endpointInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().Endpoints(namespace).List(opts)
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Endpoints(namespace).Watch(opts)
			},
		},
		&v1.Endpoints{}, 1*time.Second, cache.Indexers{},
	)

	endpointInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				endpoints <- obj.(*v1.Endpoints)
			},
			UpdateFunc: func(old, cur interface{}) {
				endpoints <- cur.(*v1.Endpoints)
			},
			DeleteFunc: func(obj interface{}) {
				endpoints <- obj.(*v1.Endpoints) // TODO: what does the obj look like in this case?
			},
		},
	)

	podInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				return client.CoreV1().Pods(namespace).List(opts)
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				return client.CoreV1().Pods(namespace).Watch(opts)
			},
		},
		&v1.Pod{}, 1*time.Second, cache.Indexers{},
	)

	pc := newPodCache(podInformer)
	m := newMirror(eurekaURL, pc)

	stop := make(chan struct{})

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range c {
			log.Printf("captured sig %v, exiting\n", sig)
			close(stop)
			close(endpoints)
			os.Exit(1)
		}
	}()

	go endpointInformer.Run(stop)
	go podInformer.Run(stop)
	go m.Sync(endpoints)
	<-stop
}
