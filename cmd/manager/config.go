// Copyright 2017 Google Inc.
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
	"io/ioutil"
	"os"
	"sort"

	yaml "gopkg.in/yaml.v2"

	"k8s.io/client-go/pkg/api"

	"github.com/spf13/cobra"

	"istio.io/manager/model"
)

var (
	kind string

	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Istio configuration registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			kinds := make([]string, 0)
			for kind := range model.IstioConfig {
				kinds = append(kinds, kind)
			}
			sort.Strings(kinds)
			fmt.Printf("Available configuration resources: %v\n", kinds)
			return nil
		},
	}

	putCmd = &cobra.Command{
		Use:   "put [kind] [name]",
		Short: "Store a configuration object from standard input YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Provide kind and name")
			}
			kind, ok := model.IstioConfig[args[0]]
			if !ok {
				return fmt.Errorf("Missing kind %s", args[0])
			}
			if flags.namespace == "" {
				flags.namespace = api.NamespaceDefault
			}

			// read stdin
			bytes, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("Cannot read input: %v", err)
			}

			out := make(map[string]interface{})
			err = yaml.Unmarshal(bytes, &out)
			if err != nil {
				return fmt.Errorf("Cannot read YAML input: %v", err)
			}

			v, err := kind.FromJSONMap(out)
			if err != nil {
				return fmt.Errorf("Cannot parse proto message: %v", err)
			}

			err = flags.client.Put(model.Key{
				Kind:      args[0],
				Name:      args[1],
				Namespace: flags.namespace,
			}, v)

			return err
		},
	}

	getCmd = &cobra.Command{
		Use:   "get [kind] [name]",
		Short: "Retrieve a configuration object",
	}

	listCmd = &cobra.Command{
		Use:   "list [kind]",
		Short: "List configuration objects",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, kind := range args {
				list, err := flags.client.List(kind, flags.namespace)
				if err != nil {
					fmt.Printf("Error listing %s: %v\n", kind, err)
				} else {
					for name, item := range list {
						fmt.Printf("Name %s\n", name)
						fmt.Println("======")
						data, err := yaml.Marshal(item)
						if err != nil {
							fmt.Printf("Error %v\n", err)
						} else {
							fmt.Println(string(data))
						}
					}
				}
			}
			return nil
		},
	}

	deleteCmd = &cobra.Command{
		Use:   "delete [kind] [name]",
		Short: "Delete a configuration object",
	}
)

func init() {
	configCmd.AddCommand(putCmd)
	configCmd.AddCommand(getCmd)
	configCmd.AddCommand(listCmd)
	configCmd.AddCommand(deleteCmd)
}
