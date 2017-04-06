package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"istio.io/manager/model"

	restful "github.com/emicklei/go-restful"
)

var schema model.ProtoSchema

func (api *API) GetConfig(request *restful.Request, response *restful.Response) {

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	proto, ok := api.registry.Get(key)
	if !ok {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusNotFound, "item not found")
		return
	}

	retrieved, err := schema.ToJSON(proto)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
	var retJSON interface{}
	err = json.Unmarshal([]byte(retrieved), &retJSON)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}
	config := Config{
		Name: params["name"],
		Type: params["kind"],
		Spec: retJSON,
	}
	response.WriteHeaderAndEntity(http.StatusOK, config)
}

func (api *API) AddConfig(request *restful.Request, response *restful.Response) {

	// ToDo: check url name matches body name

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	config := &Config{}
	err = request.ReadEntity(config)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	err = config.ParseSpec()
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	err = api.registry.Post(key, config.ParsedSpec)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		switch err.(type) {
		case *model.ItemAlreadyExistsError:
			response.WriteErrorString(http.StatusConflict, err.Error())
			return
		default:
			response.WriteErrorString(http.StatusInternalServerError, err.Error())
			return
		}
	}
	response.WriteHeaderAndEntity(http.StatusCreated, config)
}

func (api *API) UpdateConfig(request *restful.Request, response *restful.Response) {

	// ToDo: check url name matches body name

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	config := &Config{}
	err = request.ReadEntity(config)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	err = config.ParseSpec()
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	err = api.registry.Put(key, config.ParsedSpec)

	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		switch err.(type) {
		case *model.ItemNotFoundError:
			response.WriteErrorString(http.StatusNotFound, err.Error())
			return
		default:
			response.WriteErrorString(http.StatusInternalServerError, err.Error())
			return
		}
	}
	response.WriteHeaderAndEntity(http.StatusOK, config)
}

func (api *API) DeleteConfig(request *restful.Request, response *restful.Response) {

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusBadRequest, err.Error())
		return
	}

	err = api.registry.Delete(key)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		switch err.(type) {
		case *model.ItemNotFoundError:
			response.WriteErrorString(http.StatusNotFound, err.Error())
			return
		default:
			response.WriteErrorString(http.StatusInternalServerError, err.Error())
			return
		}
	}
	response.WriteHeader(http.StatusOK)
}
func (api *API) ListConfigs(request *restful.Request, response *restful.Response) {

	params := request.PathParameters()
	namespace, kind := params["namespace"], params["kind"]

	_, ok := model.IstioConfig[kind]
	if !ok {
		response.WriteErrorString(http.StatusBadRequest, fmt.Sprintf("unknown configuration type %s; use one of %v", kind, model.IstioConfig.Kinds()))
		return
	}

	result, err := api.registry.List(kind, namespace)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		response.WriteErrorString(http.StatusInternalServerError, err.Error())
		return
	}

	// Parse back to config
	out := []Config{}
	for k, v := range result {
		retrieved, err := schema.ToJSON(v)
		if err != nil {
			response.AddHeader("Content-Type", "text/plain")
			response.WriteErrorString(http.StatusInternalServerError, err.Error())
			return
		}
		var retJSON interface{}
		err = json.Unmarshal([]byte(retrieved), &retJSON)
		if err != nil {
			response.AddHeader("Content-Type", "text/plain")
			response.WriteErrorString(http.StatusInternalServerError, err.Error())
			return
		}
		config := Config{
			Name: k.Name,
			Type: k.Kind,
			Spec: retJSON,
		}
		out = append(out, config)
	}
	response.WriteHeaderAndEntity(http.StatusOK, out)
}

func setup(params map[string]string) (model.Key, error) {
	name, namespace, kind := params["name"], params["namespace"], params["kind"]
	_, ok := model.IstioConfig[kind]
	if !ok {
		return model.Key{}, fmt.Errorf("unknown configuration type %s; use one of %v", kind, model.IstioConfig.Kinds())
	}

	return model.Key{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}, nil
}
