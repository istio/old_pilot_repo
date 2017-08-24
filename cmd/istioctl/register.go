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
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/pilot/platform/kube"
)

type namedPort struct {
	Port int32
	Name string
}

func (p *namedPort) String() string {
	return fmt.Sprintf("%s:%d", p.Name, p.Port)
}

var (
	// For most common ports allow the protocol to be guessed
	portsToName = map[int32]string{
		80:   "http",
		443:  "https",
		8000: "http",
		8080: "http",
	}
)

func str2NamedPort(str string) (namedPort, error) {
	var r namedPort
	idx := strings.Index(str, ":")
	if idx >= 0 {
		r.Name = str[:idx]
		str = str[idx+1:]
	}
	p, err := strconv.Atoi(str)
	if err != nil {
		return r, err
	}
	r.Port = int32(p)
	if len(r.Name) == 0 {
		name, found := portsToName[r.Port]
		r.Name = name
		if !found {
			r.Name = str
		}
	}
	return r, nil
}

var (
	registerCmd = &cobra.Command{
		Use:   "register <svcname> <ip> [name1:]port1 [name2:]port2 ...",
		Short: "Registers a service instance (VM)",
		Args:  cobra.MinimumNArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			svcName := args[0]
			ip := args[1]
			portsListStr := args[2:]
			portsList := make([]namedPort, len(portsListStr))
			for i := range portsListStr {
				p, err := str2NamedPort(portsListStr[i])
				if err != nil {
					return err
				}
				portsList[i] = p
			}
			glog.Infof("Registering for service '%s' ip '%s', ports list %v",
				svcName, ip, portsList)
			return registerSvc(svcName, ip, portsList)
		},
	}
)

func init() {
	rootCmd.AddCommand(registerCmd)
}

func registerSvc(svcName string, ip string, portsList []namedPort) error {
	client, err := kube.CreateInterface(kubeconfig)
	if err != nil {
		return err
	}
	var eps *v1.Endpoints
	getOpt := meta_v1.GetOptions{IncludeUninitialized: true}
	_, err = client.Core().Services(namespace).Get(svcName, getOpt)
	if err != nil {
		glog.Warningf("Got '%v' looking up svc '%s' in namespace '%s', attempting to create it", err, svcName, namespace)
		svc := v1.Service{}
		svc.Name = svcName
		for _, p := range portsList {
			svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{Name: p.Name, Port: p.Port})
		}
		_, err = client.CoreV1().Services(namespace).Create(&svc)
		if err != nil {
			glog.Error("Unable to create service: ", err)
			return err
		}
	}
	eps, err = client.CoreV1().Endpoints(namespace).Get(svcName, getOpt)
	if err != nil {
		glog.Warningf("Got '%v' looking up endpoints for '%s' in namespace '%s', attempting to create them",
			err, svcName, namespace)
		endP := v1.Endpoints{}
		endP.Name = svcName // same but does it need to be
		eps, err = client.CoreV1().Endpoints(namespace).Create(&endP)
		if err != nil {
			glog.Error("Unable to create endpoint: ", err)
			return err
		}
	}
	glog.V(2).Infof("Before: found endpoints %+v", eps)
	if glog.V(1) {
		for _, ss := range eps.Subsets {
			glog.Infof("On ports %+v", ss.Ports)
			for _, ip := range ss.Addresses {
				glog.Infof("Found %+v", ip)
			}
		}
	}
	// TODO: if port numbers match existing entry, reuse
	newSubSet := v1.EndpointSubset{}
	newSubSet.Addresses = []v1.EndpointAddress{
		{IP: ip},
	}
	for _, p := range portsList {
		newSubSet.Ports = append(newSubSet.Ports, v1.EndpointPort{Name: p.Name, Port: p.Port})
	}
	eps.Subsets = append(eps.Subsets, newSubSet)
	eps, err = client.CoreV1().Endpoints(namespace).Update(eps)
	if err != nil {
		glog.Error("Update failed with: ", err)
		return err
	}
	glog.Infof("Successfully updated %v", eps)
	return nil
}
