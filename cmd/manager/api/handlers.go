package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"istio.io/manager/cmd"
	"istio.io/manager/model"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
)

// configDoc is the complete configuration including a parsed spec
type configDoc struct {
	*configInput
	// ParsedSpec will be one of the messages in model.IstioConfig: for example an
	// istio.proxy.v1alpha.config.RouteRule or DestinationPolicy
	ParsedSpec proto.Message `json:"-"`
}

// configInput is what the manager expects to receive from a request body
type configInput struct {
	// Type SHOULD be one of the kinds in model.IstioConfig; a route-rule, ingress-rule, or destination-policy
	Type string      `json:"type,omitempty"`
	Name string      `json:"name,omitempty"`
	Spec interface{} `json:"spec,omitempty"`
}

func GetConfig(w http.ResponseWriter, r *http.Request) {
	// Do some stuff
}

func AddConfig(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	name, namespace, kind := vars["name"], vars["namespace"], vars["kind"]
	keyPtr, err := setup(kind, namespace, name)
	key := *keyPtr

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	config, err := processBody(body)
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = cmd.Client.Post(key, config.ParsedSpec)
	if err != nil {
		w.Write([]byte(err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Write([]byte(fmt.Sprintf("Created %v %v\n", config.Type, config.Name)))
	w.WriteHeader(200)
}

func UpdateConfig(w http.ResponseWriter, r *http.Request) {
	// Do some stuff
}
func DeleteConfig(w http.ResponseWriter, r *http.Request) {
	// Do some stuff
}
func ListConfigs(w http.ResponseWriter, r *http.Request) {
	// Do some stuff
}

// processBody reads the config from the request body and vaildates with the schema
func processBody(body []byte) (*configDoc, error) {

	var cInput *configInput
	err := json.Unmarshal(body, cInput)
	if err != nil {
		return nil, fmt.Errorf("could not marshal request body: %v", err)
	}
	byteSpec, err := json.Marshal(cInput.Spec)
	if err != nil {
		return nil, fmt.Errorf("could not encode Spec: %v", err)
	}
	schema, ok := model.IstioConfig[cInput.Type]
	if !ok {
		return nil, fmt.Errorf("unknown spec type %s", cInput.Type)
	}
	message, err := schema.FromJSON(string(byteSpec))
	if err != nil {
		return nil, fmt.Errorf("cannot parse proto message: %v", err)
	}
	glog.V(2).Info(fmt.Sprintf("Parsed %v %v into %v %v", cInput.Type, cInput.Name, schema.MessageName, message))

	return &configDoc{
		cInput,
		message,
	}, nil
}

func setup(kind, namespace, name string) (*model.Key, error) {
	// Validate proto schema
	_, ok := model.IstioConfig[kind]
	if !ok {
		return nil, fmt.Errorf("unknown configuration type %s; use one of %v", kind, model.IstioConfig.Kinds())
	}

	// set the config key
	key := model.Key{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}

	return &key, nil
}
