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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/golang/glog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"istio.io/pilot/adapter/config/crd"
	"istio.io/pilot/cmd"
	"istio.io/pilot/model"
	"istio.io/pilot/platform/kube"
	"istio.io/pilot/tools/version"
)

const (
	kubePlatform = "kube"
)

var (
	platform string

	kubeconfig     string
	namespace      string
	istioNamespace string
	kubectlcommand string

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

`, model.IstioConfigTypes.Types()),
	}

	postCmd = &cobra.Command{
		Use:   "create",
		Short: "Create policies and rules",
		Example: `
			istioctl create -f example-routing.yaml
			`,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) != 0 {
				c.Println(c.UsageString())
				return fmt.Errorf("create takes no arguments")
			}
			varr, others, err := readInputs()
			if err != nil {
				return err
			}
			if len(varr) == 0 && len(others) == 0 {
				return errors.New("nothing to create")
			}
			for _, config := range varr {
				if config.Namespace == "" {
					config.Namespace = namespace
				}

				var configClient *crd.Client
				if configClient, err = newClient(); err != nil {
					return err
				}
				var rev string
				if rev, err = configClient.Create(config); err != nil {
					return err
				}
				fmt.Printf("Created config %v at revision %v\n", config.Key(), rev)
			}

			if len(others) > 0 {
				if err = preprocMixerConfig(others); err != nil {
					return err
				}
				otherClient, resources, oerr := prepareClientForOthers(others)
				if oerr != nil {
					return oerr
				}
				var errs *multierror.Error
				var updated crd.IstioKind
				for _, config := range others {
					resource, ok := resources[config.Kind]
					if !ok {
						errs = multierror.Append(errs, fmt.Errorf("kind %s is not known", config.Kind))
						continue
					}
					err = otherClient.Post().
						Namespace(config.Namespace).
						Resource(resource.Name).
						Body(&config).
						Do().
						Into(&updated)
					if err != nil {
						errs = multierror.Append(errs, err)
						continue
					}
					key := model.Key(config.Kind, config.Name, config.Namespace)
					fmt.Printf("Created config %s at revision %v\n", key, updated.ResourceVersion)
				}
				if errs != nil {
					return errs
				}
			}

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
			if len(args) != 0 {
				c.Println(c.UsageString())
				return fmt.Errorf("replace takes no arguments")
			}
			varr, others, err := readInputs()
			if err != nil {
				return err
			}
			if len(varr) == 0 && len(others) == 0 {
				return errors.New("nothing to replace")
			}
			for _, config := range varr {
				if config.Namespace == "" {
					config.Namespace = namespace
				}

				var configClient *crd.Client
				if configClient, err = newClient(); err != nil {
					return err
				}
				// fill up revision
				if config.ResourceVersion == "" {
					current, exists := configClient.Get(config.Type, config.Name, config.Namespace)
					if exists {
						config.ResourceVersion = current.ResourceVersion
					}
				}

				var newRev string
				if newRev, err = configClient.Update(config); err != nil {
					return err
				}

				fmt.Printf("Updated config %v to revision %v\n", config.Key(), newRev)
			}

			if len(others) > 0 {
				if err = preprocMixerConfig(others); err != nil {
					return err
				}
				otherClient, resources, oerr := prepareClientForOthers(others)
				if oerr != nil {
					return oerr
				}
				var errs *multierror.Error
				var current crd.IstioKind
				var updated crd.IstioKind
				for _, config := range others {
					resource, ok := resources[config.Kind]
					if !ok {
						errs = multierror.Append(errs, fmt.Errorf("kind %s is not known", config.Kind))
						continue
					}
					if config.ResourceVersion == "" {
						err = otherClient.Get().
							Namespace(config.Namespace).
							Name(config.Name).
							Resource(resource.Name).
							Do().
							Into(&current)
						if err == nil && current.ResourceVersion != "" {
							config.ResourceVersion = current.ResourceVersion
						}
					}

					err = otherClient.Put().
						Namespace(config.Namespace).
						Name(config.Name).
						Resource(resource.Name).
						Body(&config).
						Do().
						Into(&updated)
					if err != nil {
						errs = multierror.Append(errs, err)
						continue
					}
					key := model.Key(config.Kind, config.Name, config.Namespace)
					fmt.Printf("Updated config %s to revision %v\n", key, updated.ResourceVersion)
				}
				if errs != nil {
					return errs
				}
			}

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
			configClient, err := newClient()
			if err != nil {
				return err
			}
			if len(args) < 1 {
				c.Println(c.UsageString())
				return fmt.Errorf("specify the type of resource to get. Types are %v",
					strings.Join(configClient.ConfigDescriptor().Types(), ", "))
			}

			typ, err := schema(configClient, args[0])
			if err != nil {
				c.Println(c.UsageString())
				return err
			}

			var configs []model.Config
			if len(args) > 1 {
				config, exists := configClient.Get(typ.Type, args[1], namespace)
				if exists {
					configs = append(configs, *config)
				}
			} else {
				configs, err = configClient.List(typ.Type, namespace)
				if err != nil {
					return err
				}
			}

			if len(configs) == 0 {
				fmt.Println("No resources found.")
				return nil
			}

			var outputters = map[string](func(*crd.Client, []model.Config)){
				"yaml":  printYamlOutput,
				"short": printShortOutput,
			}

			if outputFunc, ok := outputters[outputFormat]; ok {
				outputFunc(configClient, configs)
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
			configClient, errs := newClient()
			if errs != nil {
				return errs
			}
			// If we did not receive a file option, get names of resources to delete from command line
			if file == "" {
				if len(args) < 2 {
					c.Println(c.UsageString())
					return fmt.Errorf("provide configuration type and name or -f option")
				}
				typ, err := schema(configClient, args[0])
				if err != nil {
					return err
				}
				for i := 1; i < len(args); i++ {
					if err := configClient.Delete(typ.Type, args[i], namespace); err != nil {
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
			varr, others, err := readInputs()
			if err != nil {
				return err
			}
			if len(varr) == 0 && len(others) == 0 {
				return errors.New("nothing to delete")
			}
			for _, config := range varr {
				if config.Namespace == "" {
					config.Namespace = namespace
				}

				// compute key if necessary
				if err = configClient.Delete(config.Type, config.Name, config.Namespace); err != nil {
					errs = multierror.Append(errs, fmt.Errorf("cannot delete %s: %v", config.Key(), err))
				} else {
					fmt.Printf("Deleted config: %v\n", config.Key())
				}
			}
			if errs != nil {
				return errs
			}

			if len(others) > 0 {
				if err = preprocMixerConfig(others); err != nil {
					return err
				}
				otherClient, resources, oerr := prepareClientForOthers(others)
				if oerr != nil {
					return oerr
				}
				for _, config := range others {
					resource, ok := resources[config.Kind]
					if !ok {
						errs = multierror.Append(errs, fmt.Errorf("kind %s is not known", config.Kind))
						continue
					}
					err = otherClient.Delete().
						Namespace(config.Namespace).
						Resource(resource.Name).
						Do().
						Error()
					if err != nil {
						errs = multierror.Append(errs, fmt.Errorf("failed to delete: %v", err))
						continue
					}
					fmt.Printf("Deleted cofig: %s\n", model.Key(config.Kind, config.Name, config.Namespace))
				}
			}

			return errs
		},
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(version.Version())
			return nil
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&platform, "platform", "p", kubePlatform,
		"Istio host platform")
	defaultKubeconfig := os.Getenv("HOME") + "/.kube/config"
	if v := os.Getenv("KUBECONFIG"); v != "" {
		defaultKubeconfig = v
	}
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "c", defaultKubeconfig,
		"Kubernetes configuration file")

	rootCmd.PersistentFlags().StringVarP(&istioNamespace, "istioNamespace", "i", kube.IstioNamespace,
		"Istio system namespace")

	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", v1.NamespaceDefault,
		"Config namespace")

	postCmd.PersistentFlags().StringVarP(&file, "file", "f", "",
		"Input file with the content of the configuration objects (if not set, command reads from the standard input)")
	postCmd.PersistentFlags().StringVar(&kubectlcommand, "kubectl", "",
		"The path to kubectl command for uploading mixer config. If not set, kubectl will be found from $PATH.")
	putCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("file"))
	putCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("kubectl"))
	deleteCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("file"))
	deleteCmd.PersistentFlags().AddFlag(postCmd.PersistentFlags().Lookup("kubectl"))

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
	if platform != kubePlatform {
		glog.Warningf("Platform '%s' not supported.", platform)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

// The schema is based on the kind (for example "route-rule" or "destination-policy")
func schema(configClient *crd.Client, typ string) (model.ProtoSchema, error) {
	for _, desc := range configClient.ConfigDescriptor() {
		if desc.Type == typ || desc.Plural == typ {
			return desc, nil
		}
	}
	return model.ProtoSchema{}, fmt.Errorf("Istio doesn't have configuration type %s, the types are %v",
		typ, strings.Join(configClient.ConfigDescriptor().Types(), ", "))
}

// readInputs reads multiple documents from the input and checks with the schema
func readInputs() ([]model.Config, []crd.IstioKind, error) {
	var reader io.Reader
	if file == "" {
		reader = os.Stdin
	} else {
		var err error
		reader, err = os.Open(file)
		if err != nil {
			return nil, nil, err
		}
	}
	input, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}
	if out, err := readInputsLegacy(bytes.NewReader(input)); err == nil {
		return out, nil, nil
	}
	return readInputsKubectl(bytes.NewReader(input))
}

// readInputsKubectl reads multiple documents from the input and checks with
// the schema.
//
// NOTE: This function only decodes a subset of the complete k8s
// ObjectMeta as identified by the fields in model.ConfigMeta. This
// would typically only be a problem if a user dumps an configuration
// object with kubectl and then re-ingests it through istioctl.
func readInputsKubectl(reader io.Reader) ([]model.Config, []crd.IstioKind, error) {
	var varr []model.Config
	var others []crd.IstioKind

	// We store route-rules as a YaML stream; there may be more than one decoder.
	yamlDecoder := kubeyaml.NewYAMLOrJSONDecoder(reader, 512*1024)
	for {
		obj := crd.IstioKind{}
		err := yamlDecoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("cannot parse proto message: %v", err)
		}

		if obj.APIVersion != "" && obj.APIVersion != crd.IstioAPIGroupVersion.String() {
			return nil, nil, fmt.Errorf("unrecognized api version: %s", obj.APIVersion)
		}
		schema, exists := model.IstioConfigTypes.GetByType(crd.CamelCaseToKabobCase(obj.Kind))
		if !exists {
			glog.V(7).Infof("Unrecognized type %v; considering as mixer config", obj.Kind)
			others = append(others, obj)
			continue
		}

		config, err := crd.ConvertObject(schema, &obj, "")
		if err != nil {
			return nil, nil, fmt.Errorf("cannot parse proto message: %v", err)
		}

		if err := schema.Validate(config.Spec); err != nil {
			return nil, nil, fmt.Errorf("configuration is invalid: %v", err)
		}

		varr = append(varr, *config)
	}
	glog.V(2).Infof("parsed %d inputs", len(varr))

	return varr, others, nil
}

// readInputsLegacy reads multiple documents from the input and checks
// with the schema.
func readInputsLegacy(reader io.Reader) ([]model.Config, error) {
	var varr []model.Config

	// We store route-rules as a YaML stream; there may be more than one decoder.
	yamlDecoder := kubeyaml.NewYAMLOrJSONDecoder(reader, 512*1024)
	for {
		v := model.JSONConfig{}
		err := yamlDecoder.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot parse proto message: %v", err)
		}

		config, err := model.IstioConfigTypes.FromJSON(v)
		if err != nil {
			return nil, fmt.Errorf("cannot parse proto message: %v", err)
		}

		varr = append(varr, *config)
	}
	glog.V(2).Infof("parsed %d inputs", len(varr))

	return varr, nil
}

// Print a simple list of names
func printShortOutput(_ *crd.Client, configList []model.Config) {
	for _, c := range configList {
		fmt.Printf("%v\n", c.Key())
	}
}

// Print as YAML
func printYamlOutput(configClient *crd.Client, configList []model.Config) {
	for _, c := range configList {
		yaml, _ := configClient.ConfigDescriptor().ToYAML(c)
		fmt.Print(yaml)
		fmt.Println("---")
	}
}

func newClient() (*crd.Client, error) {
	return crd.NewClient(kubeconfig, model.ConfigDescriptor{
		model.RouteRule,
		model.EgressRule,
		model.DestinationPolicy,
	}, "")
}

func preprocMixerConfig(configs []crd.IstioKind) error {
	for i, config := range configs {
		if config.Namespace == "" {
			configs[i].Namespace = namespace
		}
		if config.APIVersion == "" {
			configs[i].APIVersion = crd.IstioAPIGroupVersion.String()
		}
		// TODO: invokes the mixer validation webhook.
	}
	return nil
}

func apiResources(config *rest.Config, configs []crd.IstioKind) (map[string]metav1.APIResource, error) {
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	resources, err := client.ServerResourcesForGroupVersion(crd.IstioAPIGroupVersion.String())
	if err != nil {
		return nil, err
	}
	kindsSet := map[string]bool{}
	for _, config := range configs {
		if !kindsSet[config.Kind] {
			kindsSet[config.Kind] = true
		}
	}
	result := make(map[string]metav1.APIResource, len(kindsSet))
	for _, resource := range resources.APIResources {
		if kindsSet[resource.Kind] {
			result[resource.Kind] = resource
		}
	}
	return result, nil
}

func restClientForOthers(config *rest.Config) (*rest.RESTClient, error) {
	configCopied := *config
	configCopied.ContentConfig = dynamic.ContentConfig()
	configCopied.GroupVersion = &crd.IstioAPIGroupVersion
	return rest.RESTClientFor(config)
}

func prepareClientForOthers(configs []crd.IstioKind) (*rest.RESTClient, map[string]metav1.APIResource, error) {
	restConfig, err := crd.CreateRESTConfig(kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	resources, err := apiResources(restConfig, configs)
	if err != nil {
		return nil, nil, err
	}
	client, err := restClientForOthers(restConfig)
	if err != nil {
		return nil, nil, err
	}
	return client, resources, nil
}
