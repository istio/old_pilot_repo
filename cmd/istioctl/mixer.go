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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	rpc "github.com/googleapis/googleapis/google/rpc"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
)

// TODO This should come from something like istio.io/api instead of
// being hand copied from istio.io/mixer.
type mixerAPIResponse struct {
	Data   interface{} `json:"data,omitempty"`
	Status rpc.Status  `json:"status,omitempty"`
}

const (
	requestTimeout = 60 * time.Second
)

var (
	mixerFile          string
	mixerAPIServerAddr string

	mixerCmd = &cobra.Command{
		Use:   "mixer",
		Short: "Istio Mixer configuration",
		Long: `
The mixer configuration API allows users to configure all facets of a
mixer deployment. The mixer configuration is organized in 'scope' ->
'subject' -> 'rule' hierarchy. 'scope' and 'subject' are analogous to
a manager > employee relationship. A manager ('scope') may configure
any direct or indirect reports ('subject') including self.

* 'scope' models a scope of authority. It pertains to the authority that
  is configuring the system. 'global' and 'namespace:ns1.cluster.local'
  are examples of scopes. Rules contained in a scope can only affect
  that scope.

* 'subject' models the entity (often service) being configured. In
   order for a 'scope' to configure a 'subject', the 'scope' must
   contain the 'subject'

Consider '/scopes/{scope}/subjects/{subject}/rules', subject must be
contained within the scope.

	* '/scopes/namespace:ns1./subjects/service:svc1.ns1./rules' is
      valid because service svc1 is contained in namespace
      ns1. Therefore scope 'namespace:ns1.' may define policies that
      affect 'service:svc1.ns1.'

    * '/scopes/namespace:ns1./subjects/service:svc1.ns2./rules' is
      not valid, because svc1.ns2 is not contained in
      namespace ns1.

'scope' and 'subject' draw from the same set of values with 'scope >= subject'.

Example usage:
	# The mixer config server can be accessed from outside the
    # kubernetes cluster using port forwarding.
    CONFIG_PORT=$(kubectl get pod -l istio=mixer -o jsonpath='{.items[0].spec.containers[0].ports[1].containerPort}')
    export ISTIO_MIXER_API_SERVER=localhost:${CONFIG_PORT}
    kubectl port-forward $(kubectl get pod -l istio=mixer -o jsonpath='{.items[0].metadata.name}') ${CONFIG_PORT}:${CONFIG_PORT} &
`,
		SilenceUsage: true,
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			if mixerAPIServerAddr == "" {
				return errors.New("no mixer configuration server specified (use --mixer)")
			}
			return nil
		},
	}

	mixerRuleCmd = &cobra.Command{
		Use:   "rule",
		Short: "Istio Mixer Rule configuration",
		Long: `
Create and list mixer rules in the configuration server.
`,
		SilenceUsage: true,
	}

	mixerRuleCreateCmd = &cobra.Command{
		Use:   "create",
		Short: "Create Istio Mixer rules",
		Long: `
Example usage:
    # Create a new mixer rule for the given scope and subject.
    istioctl mixer rule create global myservice.ns.svc.cluster.local -f mixer-rule.yml
`,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 || mixerFile == "" {
				return errors.New(c.UsageString())
			}
			rule, err := ioutil.ReadFile(mixerFile)
			if err != nil {
				return fmt.Errorf("failed opening %s: %v", mixerFile, err)
			}
			return mixerRuleCreate(mixerAPIServerAddr, args[0], args[1], rule)
		},
	}
	mixerRuleGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get Istio Mixer rules",
		Long: `
Get a mixer rule for a given scope and subject. The output
Example usage:
	# Get the mixer rule with scope='global' and subject='myservice.ns.svc.cluster.local'
    istioctl mixer rule get global myservice.ns.svc.cluster.local
`,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errors.New(c.UsageString())
			}
			out, err := mixerRuleGet(mixerAPIServerAddr, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
)

func mixerRulePath(host, scope, subject string) string {
	if !strings.HasPrefix(host, "http://") {
		host = "http://" + host
	}
	return fmt.Sprintf("%s/api/v1/scopes/%s/subjects/%s/rules", host, scope, subject)
}

func mixerRuleCreate(host, scope, subject string, rule []byte) error {
	request, err := http.NewRequest(http.MethodPut, mixerRulePath(host, scope, subject), bytes.NewReader(rule))
	if err != nil {
		return fmt.Errorf("failed creating request: %v", err)
	}
	request.Header.Set("Content-Type", "application/yaml")

	client := http.Client{Timeout: requestTimeout}
	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("failed sending request: %v", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed processing response: %v", err)
		}
		var response mixerAPIResponse
		message := "unknown"
		if err := json.Unmarshal(body, &response); err == nil {
			message = response.Status.Message
		}
		return fmt.Errorf("failed rule creation with status %q: %q", resp.StatusCode, message)
	}
	return nil
}

func mixerRuleGet(host, scope, subject string) (string, error) {
	client := http.Client{Timeout: requestTimeout}
	resp, err := client.Get(mixerRulePath(host, scope, subject))
	if err != nil {
		return "", fmt.Errorf("failed sending request: %v", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(http.StatusText(resp.StatusCode))
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed reading response: %v", err)
	}
	var response mixerAPIResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed processing response: %v", err)
	}
	data, err := yaml.Marshal(response.Data)
	if err != nil {
		return "", fmt.Errorf("failed formatting response: %v", err)
	}
	return string(data), nil
}

func init() {
	mixerRuleCreateCmd.PersistentFlags().StringVarP(&mixerFile, "file", "f", "",
		"Input file with contents of the mixer rule")
	mixerCmd.PersistentFlags().StringVarP(&mixerAPIServerAddr, "mixer", "m", os.Getenv("ISTIO_MIXER_API_SERVER"),
		"Address of the mixer configuration server as <host>:<port>")

	mixerRuleCmd.AddCommand(mixerRuleCreateCmd)
	mixerRuleCmd.AddCommand(mixerRuleGetCmd)
	mixerCmd.AddCommand(mixerRuleCmd)
	rootCmd.AddCommand(mixerCmd)
}
