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
)

const (
	mixerSvcAccName string = "istio-mixer-service-account"
	pilotSvcAccName string = "istio-pilot-service-account"
)

func GetMixerSAN(domain, ns string) []string {
	mixerSvcAcc := fmt.Sprintf("spiffe://%v/ns/%v/sa/%v", domain, ns, mixerSvcAccName)
	mixerSvcAccounts := []string{mixerSvcAcc}
	return mixerSvcAccounts
}

func GetPilotSAN(domain, ns string) []string {
	pilotSvcAcc := fmt.Sprintf("spiffe://%v/ns/%v/sa/%v", domain, ns, pilotSvcAccName)
	pilotSvcAccounts := []string{pilotSvcAcc}
	return pilotSvcAccounts
}
