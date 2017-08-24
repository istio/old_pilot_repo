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

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"istio.io/pilot/platform/kube"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	registerCmd = &cobra.Command{
		Use:   "register <svcname> <ip> <ports...>",
		Short: "Registers a service instance (VM)",
		Args:  cobra.MinimumNArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			svcName := args[0]
			ip := args[1]
			portsListStr := args[2:]
			portsList := make([]int32, len(portsListStr))
			for i := range portsListStr {
				p, err := strconv.Atoi(portsListStr[i])
				if err != nil {
					return err
				}
				portsList[i] = int32(p)
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

func namePort(port int32) string {
	return fmt.Sprintf("%d", port)
}

func registerSvc(svcName string, ip string, portsList []int32) error {
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
			svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{Name: namePort(p), Port: p})
		}
		_, err = client.CoreV1().Services(namespace).Create(&svc)
		if err != nil {
			glog.Error("Unable to create service: ", err)
			return err
		}
	}
	eps, err = client.CoreV1().Endpoints(namespace).Get(svcName, getOpt)
	if err != nil {
		glog.Warningf("Got '%v' looking up endpoints for '%s' in namespace '%s', attempting to create them", err, svcName, namespace)
		endP := v1.Endpoints{}
		endP.Name = svcName // same but does it need to be
		eps, err = client.CoreV1().Endpoints(namespace).Create(&endP)
		if err != nil {
			glog.Error("Bailing with: ", err)
			return err
		}
	}
	glog.Infof("Before: found endpoints %+v", eps)
	for _, ss := range eps.Subsets {
		glog.Infof("On ports %+v", ss.Ports)
		for _, ip := range ss.Addresses {
			glog.Infof("Found %+v", ip)
		}
	}
	// TODO: if port numbers match existing entry, reuse
	newSubSet := v1.EndpointSubset{}
	newSubSet.Addresses = []v1.EndpointAddress{
		v1.EndpointAddress{IP: ip},
	}
	for _, p := range portsList {
		newSubSet.Ports = append(newSubSet.Ports, v1.EndpointPort{Name: namePort(p), Port: p})
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
