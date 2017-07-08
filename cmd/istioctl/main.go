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
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/api"

	"istio.io/pilot/adapter/config/tpr"
	"istio.io/pilot/cmd"
	"istio.io/pilot/model"
	"istio.io/pilot/tools/version"
)

var (
	kubeconfig  string
	istioSystem string

	configClient model.ConfigStore

	// input file name
	file string

	// output format (yaml or short)
	outputFormat string

	rootCmd = &cobra.Command{
		Use:               "istioctl",
		Short:             "Istio control interface",
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		Long: fmt.Sprintf(`
Istio configuration command line utility.

Create, list, modify, and delete configuration resources in the Istio
system.

Available routing and traffic management configuration types:

	%v

See http://istio.io/docs/reference for an overview of routing rules
and destination policies.

More information on the mixer API configuration can be found under the
istioctl mixer command documentation.
`, model.IstioConfigTypes.Types()),
		PersistentPreRunE: func(*cobra.Command, []string) (err error) {
			configClient, err = tpr.NewClient(kubeconfig, model.ConfigDescriptor{
				model.RouteRuleDescriptor,
				model.DestinationPolicyDescriptor,
			}, istioSystem)

			return
		},
	}

	postCmd = &cobra.Command{
		Use:   "create",
		Short: "Create policies and rules",
		Example: `
		istioctl create -f example-routing.yaml
		`,
		RunE: func(c *cobra.Command, args []string) error {
			/*
				if len(args) != 0 {
					c.Println(c.UsageString())
					return fmt.Errorf("create takes no arguments")
				}
				varr, err := readInputs()
				if err != nil {
					return err
				}
				if len(varr) == 0 {
					return errors.New("nothing to create")
				}
				for _, config := range varr {
					if err = setup(config.Type, config.Name); err != nil {
						return err
					}
					err = apiClient.AddConfig(key, config)
					if err != nil {
						return err
					}
					fmt.Printf("Created config: %v %v\n", config.Type, config.Name)
				}
			*/

			return nil
		},
	}

	putCmd = &cobra.Command{
		Use:   "replace",
		Short: "Replace existing policies and rules",
		Example: `
		istioctl replace -f example-routing.yaml
		`,
		RunE: func(c *cobra.Command, args []string) error {
			/*
				if len(args) != 0 {
					c.Println(c.UsageString())
					return fmt.Errorf("replace takes no arguments")
				}
				varr, err := readInputs()
				if err != nil {
					return err
				}
				if len(varr) == 0 {
					return errors.New("nothing to replace")
				}
				for _, config := range varr {
					if err = setup(config.Type, config.Name); err != nil {
						return err
					}
					err = apiClient.UpdateConfig(key, config)
					if err != nil {
						return err
					}
					fmt.Printf("Updated config: %v %v\n", config.Type, config.Name)
				}
			*/

			return nil
		},
	}

	getCmd = &cobra.Command{
		Use:   "get <type> [<name>]",
		Short: "Retrieve policies and rules",
		Example: `
		# List all route rules
		istioctl get route-rules

		# List all destination policies
		istioctl get destination-policies

		# Get a specific rule named productpage-default
		istioctl get route-rule productpage-default
		`,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) < 1 {
				c.Println(c.UsageString())
				return fmt.Errorf("specify the type of resource to get. Types are %v",
					strings.Join(configClient.ConfigDescriptor().Types(), ", "))
			}

			typ, err := schema(args[0])
			if err != nil {
				c.Println(c.UsageString())
				return err
			}

			var configs []model.Config
			if len(args) > 1 {
				config, exists, rev := configClient.Get(typ.Type, args[1])
				if exists {
					configs = append(configs, model.Config{
						Type:     typ.Type,
						Key:      typ.Key(config),
						Revision: rev,
						Content:  config,
					})
				}
			} else {
				configs, err = configClient.List(typ.Type)
				if err != nil {
					return err
				}
			}

			if len(configs) == 0 {
				fmt.Fprintf(os.Stderr, "No resources found.\n")
				return nil
			}

			var outputters = map[string](func([]model.Config)){
				"yaml":  printYamlOutput,
				"short": printShortOutput,
			}

			if outputFunc, ok := outputters[outputFormat]; ok {
				outputFunc(configs)
			} else {
				return fmt.Errorf("unknown output format %v. Types are yaml|short", outputFormat)
			}

			return nil
		},
	}

	deleteCmd = &cobra.Command{
		Use:   "delete <type> <name> [<name2> ... <nameN>]",
		Short: "Delete policies or rules",
		Example: `
		# Delete a rule using the definition in example-routing.yaml.
		istioctl delete -f example-routing.yaml

		# Delete the rule productpage-default
		istioctl delete route-rule productpage-default
		`,
		RunE: func(c *cobra.Command, args []string) error {
			/*
				// If we did not receive a file option, get names of resources to delete from command line
				if file == "" {
					if len(args) < 2 {
						c.Println(c.UsageString())
						return fmt.Errorf("provide configuration type and name or -f option")
					}
					var errs error
					for i := 1; i < len(args); i++ {
						if err := setup(args[0], args[i]); err != nil {
							// If the user specified an invalid rule kind on the CLI,
							// don't keep processing -- it's probably a typo.
							return err
						}
						if err := apiClient.DeleteConfig(key); err != nil {
							errs = multierror.Append(errs,
								fmt.Errorf("cannot delete %s: %v", args[i], err))
						} else {
							fmt.Printf("Deleted config: %v %v\n", args[0], args[i])
						}
					}
					return errs
				}

				// As we did get a file option, make sure the command line did not include any resources to delete
				if len(args) != 0 {
					c.Println(c.UsageString())
					return fmt.Errorf("delete takes no arguments when the file option is used")
				}
				varr, err := readInputs()
				if err != nil {
					return err
				}
				if len(varr) == 0 {
					return errors.New("nothing to delete")
				}
				var errs error
				for _, v := range varr {
					if err = setup(v.Type, v.Name); err != nil {
						errs = multierror.Append(errs, err)
					} else {
						if err = apiClient.DeleteConfig(key); err != nil {
							errs = multierror.Append(errs, fmt.Errorf("cannot delete %s: %v", v.Name, err))
						} else {
							fmt.Printf("Deleted config: %v %v\n", v.Type, v.Name)
						}
					}
				}
				return errs
			*/

			return nil
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Printf(version.Version())
			return nil
		},
	}
)

func init() {
	defaultKubeconfig := os.Getenv("HOME") + "/.kube/config"
	if v := os.Getenv("KUBECONFIG"); v != "" {
		defaultKubeconfig = v
	}
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "c", defaultKubeconfig,
		"Kubernetes configuration file")
	rootCmd.PersistentFlags().StringVarP(&istioSystem, "namespace", "n", api.NamespaceDefault,
		"Kubernetes Istio system namespace")

	postCmd.PersistentFlags().StringVarP(&file, "file", "f", "",
		"Input file with the content of the configuration objects (if not set, command reads from the standard input)")
	putCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("file"))
	deleteCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("file"))

	getCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "short",
		"Output format. One of:yaml|short")

	cmd.AddFlags(rootCmd)

	rootCmd.AddCommand(postCmd)
	rootCmd.AddCommand(putCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

// The schema is based on the kind (for example "route-rule" or "destination-policy")
func schema(typ string) (model.ProtoSchema, error) {
	var singularForm = map[string]string{
		"route-rules":          "route-rule",
		"destination-policies": "destination-policy",
	}
	if singular, ok := singularForm[typ]; ok {
		typ = singular
	}

	out, ok := configClient.ConfigDescriptor().GetByType(typ)
	if !ok {
		return model.ProtoSchema{}, fmt.Errorf("Istio doesn't have configuration type %s, the types are %v",
			typ, strings.Join(configClient.ConfigDescriptor().Types(), ", "))
	}

	return out, nil
}

/*
// readInputs reads multiple documents from the input and checks with the schema
func readInputs() ([]apiserver.Config, error) {

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

	var varr []apiserver.Config

	// We store route-rules as a YaML stream; there may be more than one decoder.
	yamlDecoder := kubeyaml.NewYAMLOrJSONDecoder(reader, 512*1024)
	for {
		v := apiserver.Config{}
		err = yamlDecoder.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot parse proto message: %v", err)
		}
		varr = append(varr, v)
	}

	return varr, nil
}
*/

// Print a simple list of names
func printShortOutput(configList []model.Config) {
	for _, c := range configList {
		fmt.Printf("%v\n", c.Key)
	}
}

// Print as YAML
func printYamlOutput(configList []model.Config) {
	for _, c := range configList {
		schema, _ := configClient.ConfigDescriptor().GetByType(c.Type)
		out, _ := schema.ToYAML(c.Content)
		fmt.Printf("type: %s\n", c.Type)
		fmt.Printf("key: %s\n", c.Key)
		fmt.Printf("revision: %s\n", c.Revision)
		fmt.Println("spec:")
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
		fmt.Println("---")
	}
}
