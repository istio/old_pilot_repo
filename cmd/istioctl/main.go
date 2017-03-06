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
	"io"
	"io/ioutil"
	"os"
	"strings"
    "reflect"
    "errors"
    "encoding/json"
    "bytes"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/api"

	"istio.io/manager/cmd"
	"istio.io/manager/model"

	// Note that to get this package I had to do
	//     cd $GOPATH/src/k8s.io/client-go
	//     git checkout v2.0.0
	// because the default branch that `go get` uses couldn't find this package.
	"k8s.io/client-go/pkg/util/yaml"
)

type MetadataAndRule struct {
	Type string
	Name string
	// Spec istio.proxy.v1alpha.config.RouteRule
	Spec interface{}
	Rule proto.Message
}

var (
	// input file name
	file string

	key    model.Key
	schema model.ProtoSchema

	postCmd = &cobra.Command{
		Use:   "create",
		Short: "Create configuration objects",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("create takes no arguments")
			}
			// Initialize schema global
			varr, err := readInputs()
			if err != nil {
				return err
			}
			if len(varr) == 0 {
				return errors.New("Nothing to create")
			}
			for _, v := range varr {
				fmt.Printf("About to setup using %v with name %v\n", v.Type, v.Name)
				setup(v.Type, v.Name)
				fmt.Printf("About to Post rule %v\n", v.Rule)
				err = cmd.Client.Post(key, v.Rule)
				if err != nil {
					return err
				}
				fmt.Printf("Posted rule\n")
			}
			fmt.Printf("Posted %v rules\n", len(varr))

			return nil
		},
	}

	putCmd = &cobra.Command{
		Use:   "update [type] [name]",
		Short: "Update a configuration object",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("provide configuration type and name")
			}
			if err := setup(args[0], args[1]); err != nil {
				return err
			}
			v, err := readInput()
			if err != nil {
				return err
			}
			return cmd.Client.Put(key, v)
		},
	}

	getCmd = &cobra.Command{
		Use:   "get [type] [name]",
		Short: "Retrieve a configuration object",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("provide configuration type and name")
			}
			if err := setup(args[0], args[1]); err != nil {
				return err
			}
			item, exists := cmd.Client.Get(key)
			if !exists {
				return fmt.Errorf("does not exist")
			}
			out, err := schema.ToYAML(item)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}

	deleteCmd = &cobra.Command{
		Use:   "delete [type] [name]",
		Short: "Delete a configuration object",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("provide configuration type and name")
			}
			if err := setup(args[0], args[1]); err != nil {
				return err
			}
			err := cmd.Client.Delete(key)
			return err
		},
	}

	listCmd = &cobra.Command{
		Use:   "list [type]",
		Short: "List configuration objects",
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("please specify configuration type (one of %v)", model.IstioConfig.Kinds())
			}
			if err := setup(args[0], ""); err != nil {
				return err
			}

			list, err := cmd.Client.List(key.Kind, key.Namespace)
			if err != nil {
				return fmt.Errorf("error listing %s: %v", key.Kind, err)
			}

			for key, item := range list {
				out, err := schema.ToYAML(item)
				if err != nil {
					fmt.Println(err)
				} else {
					fmt.Printf("kind: %s\n", key.Kind)
					fmt.Printf("name: %s\n", key.Name)
					fmt.Printf("namespace: %s\n", key.Namespace)
					fmt.Println("spec:")
					lines := strings.Split(out, "\n")
					for _, line := range lines {
						if line != "" {
							fmt.Printf("  %s\n", line)
						}
					}
				}
				fmt.Println("---")
			}
			return nil
		},
	}
)

func init() {
	postCmd.PersistentFlags().StringVarP(&file, "file", "f", "",
		"Input file with the content of the configuration object (if not set, command reads from the standard input)")
	putCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("file"))

	cmd.RootCmd.Use = "istioctl"
	cmd.RootCmd.Long = fmt.Sprintf("Istio configuration command line utility. Available configuration types: %v",
		model.IstioConfig.Kinds())
	cmd.RootCmd.AddCommand(postCmd)
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

func setup(kind, name string) error {
	var ok bool
	// set proto schema
	schema, ok = model.IstioConfig[kind]
	if !ok {
		return fmt.Errorf("missing configuration type %s", kind)
	}

	// use default namespace by default
	if cmd.RootFlags.Namespace == "" {
		cmd.RootFlags.Namespace = api.NamespaceDefault
	}

	// set the config key
	key = model.Key{
		Kind:      kind,
		Name:      name,
		Namespace: cmd.RootFlags.Namespace,
	}

	return nil
}

// readInput reads from the input and checks with the schema
func readInput() (proto.Message, error) {
	var reader io.Reader
	var err error

	if file == "" {
		reader = os.Stdin
	} else {
		reader, err = os.Open(file)
		if err != nil {
			return nil, err
		}
	}

	// read from reader
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("cannot read input: %v", err)
	}

	// convert
	v, err := schema.FromYAML(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("cannot parse proto message: %v", err)
	}

	return v, nil
}

// readInput reads multiple documents from the input and checks with the schema
func readInputs() ([]MetadataAndRule, error) {

	var reader io.Reader
	var err error

	if file == "" {
		reader = os.Stdin
	} else {
		reader, err = os.Open(file)
		if err != nil {
			return nil, err
		}
	}

	var varr []MetadataAndRule

	// We store route-rules as a YaML stream; there may be more than one decoder.
	var yamlDecoder *yaml.YAMLOrJSONDecoder = yaml.NewYAMLOrJSONDecoder(reader, 512 * 1024)
	for {
		v := MetadataAndRule{}
		err = yamlDecoder.Decode(&v)
		if err == io.EOF {
			break;
		}
		if err != nil {
			fmt.Printf("cannot parse proto message: %v", err)
			os.Exit(5)
		}
		fmt.Printf("Parsed, value=%v\n", v)

		// Do a second decode pass, to get the data into structured format
		byteRule, err := json.Marshal(v.Spec)
		reader2 := bytes.NewReader(byteRule)

		fmt.Printf("Creating pbt from %v\n", v.Type)
		schema, ok := model.IstioConfig[v.Type]
		if !ok {
			fmt.Printf("Unknown spec type %s", v.Type)
			os.Exit(7)
		}
		pbt := proto.MessageType(schema.MessageName)
		if pbt == nil {
			fmt.Printf("cannot create pbt from %v", v.Type)
			os.Exit(6)
		}
		rr := reflect.New(pbt.Elem()).Interface().(proto.Message)
		var yamlDecoder2 *yaml.YAMLOrJSONDecoder = yaml.NewYAMLOrJSONDecoder(reader2, 512 * 1024)
		err = yamlDecoder2.Decode(&rr)
		if err != nil {
			fmt.Printf("cannot parse proto message: %v", err)
			os.Exit(5)
		}

		v.Rule = rr
		fmt.Printf("After second decoding, value=%v\n", v)

		varr = append(varr, v)
	}

	return varr, nil
}
