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

package server

import (
	"encoding/json"
	"net/http"

	"istio.io/pilot/cmd/version"
	"istio.io/pilot/model"

	restful "github.com/emicklei/go-restful"
	"github.com/golang/glog"
)

// Status returns 200 to indicate healthy
// Could be expanded later to identify the health of downstream dependencies such as kube, etc.
func (api *API) Status(request *restful.Request, response *restful.Response) {
	response.WriteHeader(http.StatusOK)
}

// GetConfig retrieves the config object from the configuration registry
func (api *API) GetConfig(request *restful.Request, response *restful.Response) {
	params := request.PathParameters()
	typ := params[kind]
	k := params[key]

	schema, ok := api.registry.ConfigDescriptor().GetByType(typ)
	if !ok {
		api.writeError(http.StatusBadRequest, "missing type", response)
		return
	}

	glog.V(2).Infof("Getting config from Istio registry: %+v", k)

	proto, ok, rev := api.registry.Get(typ, k)
	if !ok {
		errLocal := &model.ItemNotFoundError{Key: k}
		api.writeError(http.StatusNotFound, errLocal.Error(), response)
		return
	}

	retrieved, err := schema.ToJSON(proto)
	if err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
		return
	}
	var retJSON interface{}
	if err = json.Unmarshal([]byte(retrieved), &retJSON); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
		return
	}

	config := Config{
		Type:     typ,
		Key:      k,
		Revision: rev,
		Content:  retJSON,
	}
	glog.V(2).Infof("Retrieved config %+v", config)
	if err = response.WriteHeaderAndEntity(http.StatusOK, config); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

// AddConfig creates and stores the passed config object in the configuration registry
// It is equivalent to a restful PUT and is idempotent
func (api *API) AddConfig(request *restful.Request, response *restful.Response) {
	params := request.PathParameters()
	typ := params[kind]
	schema, ok := api.registry.ConfigDescriptor().GetByType(typ)
	if !ok {
		api.writeError(http.StatusBadRequest, "missing type", response)
		return
	}

	config := &Config{}
	if err := request.ReadEntity(config); err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	byteSpec, err := json.Marshal(config.Content)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
	}

	msg, err := schema.FromJSON(string(byteSpec))
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	glog.V(2).Infof("Adding config to Istio registry: config %+v", config)
	if _, err = api.registry.Post(msg); err != nil {
		response.AddHeader("Content-Type", "text/plain")
		switch err.(type) {
		case *model.ItemAlreadyExistsError:
			api.writeError(http.StatusConflict, err.Error(), response)
		default:
			api.writeError(http.StatusInternalServerError, err.Error(), response)
		}
		return
	}

	glog.V(2).Infof("Added config %+v", config)
	if err := response.WriteHeaderAndEntity(http.StatusCreated, config); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

// UpdateConfig updates the passed config object in the configuration registry
func (api *API) UpdateConfig(request *restful.Request, response *restful.Response) {
	params := request.PathParameters()
	typ := params[kind]
	rev := params[revision]
	schema, ok := api.registry.ConfigDescriptor().GetByType(typ)
	if !ok {
		api.writeError(http.StatusBadRequest, "missing type", response)
		return
	}

	config := &Config{}
	if err := request.ReadEntity(config); err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	byteSpec, err := json.Marshal(config.Content)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
	}

	msg, err := schema.FromJSON(string(byteSpec))
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	glog.V(2).Infof("Updating config in Istio registry: config %+v", config)
	if _, err = api.registry.Put(msg, rev); err != nil {
		switch err.(type) {
		case *model.ItemNotFoundError:
			api.writeError(http.StatusNotFound, err.Error(), response)
		default:
			api.writeError(http.StatusInternalServerError, err.Error(), response)
		}
		return
	}
	glog.V(2).Infof("Updated config to %+v", config)
	if err := response.WriteHeaderAndEntity(http.StatusOK, config); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

// DeleteConfig deletes the passed config object in the configuration registry
func (api *API) DeleteConfig(request *restful.Request, response *restful.Response) {
	params := request.PathParameters()
	typ := params[kind]
	k := params[key]
	_, ok := api.registry.ConfigDescriptor().GetByType(typ)
	if !ok {
		api.writeError(http.StatusBadRequest, "missing type", response)
		return
	}

	glog.V(2).Infof("Deleting config from Istio registry: %+v", k)
	if err := api.registry.Delete(typ, k); err != nil {
		switch err.(type) {
		case *model.ItemNotFoundError:
			api.writeError(http.StatusNotFound, err.Error(), response)
		default:
			api.writeError(http.StatusInternalServerError, err.Error(), response)
		}
		return
	}
	response.WriteHeader(http.StatusOK)
}

// ListConfigs lists the configuration objects in the the configuration registry
// If kind and namespace are passed then it retrieves all rules of a kind in a namespace
// If kind is passed and namespace is an empty string it retrieves all rules of a kind across all namespaces
func (api *API) ListConfigs(request *restful.Request, response *restful.Response) {
	params := request.PathParameters()
	typ := params[kind]
	schema, ok := api.registry.ConfigDescriptor().GetByType(typ)
	if !ok {
		api.writeError(http.StatusBadRequest, "missing type", response)
		return
	}

	glog.V(2).Infof("Getting configs of kind %s", typ)
	result, err := api.registry.List(typ)
	if err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
		return
	}

	// Parse back to config
	out := []Config{}
	for _, v := range result {
		retrieved, errLocal := schema.ToJSON(v.Content)
		if errLocal != nil {
			api.writeError(http.StatusInternalServerError, errLocal.Error(), response)
			return
		}
		var retJSON interface{}
		if errLocal = json.Unmarshal([]byte(retrieved), &retJSON); errLocal != nil {
			api.writeError(http.StatusInternalServerError, errLocal.Error(), response)
			return
		}
		config := Config{
			Type:     v.Type,
			Key:      v.Key,
			Revision: v.Revision,
			Content:  retJSON,
		}
		glog.V(2).Infof("Retrieved config %+v", config)
		out = append(out, config)
	}
	if err = response.WriteHeaderAndEntity(http.StatusOK, out); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

// Version returns the version information of apiserver
func (api *API) Version(request *restful.Request, response *restful.Response) {
	glog.V(2).Infof("Returning version information")
	if err := response.WriteHeaderAndEntity(http.StatusOK, version.Info); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

func (api *API) writeError(status int, msg string, response *restful.Response) {
	glog.Warning(msg)
	response.AddHeader("Content-Type", "text/plain")
	if err := response.WriteErrorString(status, msg); err != nil {
		glog.Warning(err)
	}
}
