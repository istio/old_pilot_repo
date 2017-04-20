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

var (
	mixerFile          string
	mixerAPIServerAddr string

	mixerCmd = &cobra.Command{
		Use:          "mixer",
		Short:        "Istio Mixer configuration",
		SilenceUsage: true,
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			if mixerAPIServerAddr == "" {
				return errors.New("no mixer API server specified (use --mixer)")
			}
			return nil
		},
	}

	mixerRuleCmd = &cobra.Command{
		Use:          "rule",
		Short:        "Istio Mixer Rule configuration",
		SilenceUsage: true,
	}

	mixerRuleCreateCmd = &cobra.Command{
		Use:   "create <scope> <subject>",
		Short: "Create Istio Mixer rules",
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
		Use:   "get <scope> <subject>",
		Short: "Get Istio Mixer rules",
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

	var client http.Client
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
	resp, err := http.Get(mixerRulePath(host, scope, subject))
	if err != nil {
		return "", fmt.Errorf("failed sending request: %v", err)
	}
	defer resp.Body.Close() // nolint: errcheck

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(http.StatusText(resp.StatusCode))
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed processing response: %v", err)
	}
	var response mixerAPIResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed processing response: %v", err)
	}
	data, err := yaml.Marshal(response.Data)
	if err != nil {
		return "", fmt.Errorf("failed processing response: %v", err)
	}
	return string(data), nil
}

func init() {
	mixerRuleCreateCmd.PersistentFlags().StringVarP(&mixerFile, "file", "f", "",
		"Input file with contents of the mixer rule")
	mixerCmd.PersistentFlags().StringVarP(&mixerAPIServerAddr, "mixer", "m", os.Getenv("ISTIO_MIXER_API_SERVER"),
		"Address of the mixer API server as <host>:<port>")

	mixerRuleCmd.AddCommand(mixerRuleCreateCmd)
	mixerRuleCmd.AddCommand(mixerRuleGetCmd)
	mixerCmd.AddCommand(mixerRuleCmd)
	rootCmd.AddCommand(mixerCmd)
}
