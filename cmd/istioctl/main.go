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
	"io/ioutil"
	"os"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/api"

	"istio.io/manager/cmd"
	"istio.io/manager/model"
)

var (
	putCmd = &cobra.Command{
		Use:   "put [kind] [name]",
		Short: "Store a configuration object from standard input YAML",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Provide kind and name")
			}
			kind, ok := model.IstioConfig[args[0]]
			if !ok {
				return fmt.Errorf("Missing kind %s", args[0])
			}
			if cmd.RootFlags.Namespace == "" {
				cmd.RootFlags.Namespace = api.NamespaceDefault
			}

			// read stdin
			bytes, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("Cannot read input: %v", err)
			}

			out, err := yaml.YAMLToJSON(bytes)
			if err != nil {
				return fmt.Errorf("Cannot read YAML input: %v", err)
			}

			v, err := kind.FromJSON(string(out))
			if err != nil {
				return fmt.Errorf("Cannot parse proto message: %v", err)
			}

			err = cmd.Client.Put(model.Key{
				Kind:      args[0],
				Name:      args[1],
				Namespace: cmd.RootFlags.Namespace,
			}, v)

			return err
		},
	}

	getCmd = &cobra.Command{
		Use:   "get [kind] [name]",
		Short: "Retrieve a configuration object",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Provide kind and name")
			}
			if cmd.RootFlags.Namespace == "" {
				cmd.RootFlags.Namespace = api.NamespaceDefault
			}
			item, exists := cmd.Client.Get(model.Key{
				Kind:      args[0],
				Name:      args[1],
				Namespace: cmd.RootFlags.Namespace,
			})
			if !exists {
				return fmt.Errorf("Does not exist")
			}
			print(args[0], item)
			return nil
		},
	}

	listCmd = &cobra.Command{
		Use:   "list [kind...]",
		Short: "List configuration objects",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("Please specify kind (one or many of %v)", model.IstioConfig.Kinds())
			}
			for _, kind := range args {
				list, err := cmd.Client.List(kind, cmd.RootFlags.Namespace)
				if err != nil {
					fmt.Printf("Error listing %s: %v\n", kind, err)
				} else {
					for key, item := range list {
						fmt.Printf("kind: %s\n", key.Kind)
						fmt.Printf("name: %s\n", key.Name)
						fmt.Printf("namespace: %s\n", key.Namespace)
						print(key.Kind, item)
						fmt.Println("---")
					}
				}
			}
			return nil
		},
	}

	deleteCmd = &cobra.Command{
		Use:   "delete [kind] [name]",
		Short: "Delete a configuration object",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("Provide kind and name")
			}
			if cmd.RootFlags.Namespace == "" {
				cmd.RootFlags.Namespace = api.NamespaceDefault
			}
			err := cmd.Client.Delete(model.Key{
				Kind:      args[0],
				Name:      args[1],
				Namespace: cmd.RootFlags.Namespace,
			})
			return err
		},
	}
)

func init() {
	cmd.RootCmd.Use = "istioctl"
	cmd.RootCmd.Long = fmt.Sprintf("Istio configuration command line utility. Available configuration kinds: %v",
		model.IstioConfig.Kinds())
	cmd.RootCmd.AddCommand(putCmd)
	cmd.RootCmd.AddCommand(getCmd)
	cmd.RootCmd.AddCommand(listCmd)
	cmd.RootCmd.AddCommand(deleteCmd)
}

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		glog.Error(err)
		os.Exit(-1)
	}
}

func print(kind string, item proto.Message) {
	schema := model.IstioConfig[kind]
	js, err := schema.ToJSON(item)
	if err != nil {
		fmt.Printf("Error converting to JSON: %v", err)
		return
	}
	yml, err := yaml.JSONToYAML([]byte(js))
	if err != nil {
		fmt.Printf("Error converting to YAML: %v", err)
	}
	fmt.Print(string(yml))
}
