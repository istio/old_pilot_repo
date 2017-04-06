package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"istio.io/manager/model"

	restful "github.com/emicklei/go-restful"
	"github.com/golang/glog"
)

var schema model.ProtoSchema

func (api *API) GetConfig(request *restful.Request, response *restful.Response) {

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	proto, ok := api.registry.Get(key)
	if !ok {
		api.writeError(http.StatusNotFound, "item not found", response)
		return
	}

	retrieved, err := schema.ToJSON(proto)
	if err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
		return
	}
	var retJSON interface{}
	err = json.Unmarshal([]byte(retrieved), &retJSON)
	if err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
		return
	}
	config := Config{
		Name: params["name"],
		Type: params["kind"],
		Spec: retJSON,
	}
	if err = response.WriteHeaderAndEntity(http.StatusOK, config); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

func (api *API) AddConfig(request *restful.Request, response *restful.Response) {

	// ToDo: check url name matches body name

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	config := &Config{}
	err = request.ReadEntity(config)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	err = config.ParseSpec()
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	err = api.registry.Post(key, config.ParsedSpec)
	if err != nil {
		response.AddHeader("Content-Type", "text/plain")
		switch err.(type) {
		case *model.ItemAlreadyExistsError:
			api.writeError(http.StatusConflict, err.Error(), response)
			return
		default:
			api.writeError(http.StatusInternalServerError, err.Error(), response)
			return
		}
	}
	if err = response.WriteHeaderAndEntity(http.StatusCreated, config); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

func (api *API) UpdateConfig(request *restful.Request, response *restful.Response) {

	// ToDo: check url name matches body name

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	config := &Config{}
	err = request.ReadEntity(config)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	err = config.ParseSpec()
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	err = api.registry.Put(key, config.ParsedSpec)

	if err != nil {
		switch err.(type) {
		case *model.ItemNotFoundError:
			api.writeError(http.StatusNotFound, err.Error(), response)
			return
		default:
			api.writeError(http.StatusInternalServerError, err.Error(), response)
			return
		}
	}
	if err = response.WriteHeaderAndEntity(http.StatusOK, config); err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
	}
}

func (api *API) DeleteConfig(request *restful.Request, response *restful.Response) {

	params := request.PathParameters()
	key, err := setup(params)
	if err != nil {
		api.writeError(http.StatusBadRequest, err.Error(), response)
		return
	}

	err = api.registry.Delete(key)
	if err != nil {
		switch err.(type) {
		case *model.ItemNotFoundError:
			api.writeError(http.StatusNotFound, err.Error(), response)
			return
		default:
			api.writeError(http.StatusInternalServerError, err.Error(), response)
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
		api.writeError(http.StatusBadRequest,
			fmt.Sprintf("unknown configuration type %s; use one of %v", kind, model.IstioConfig.Kinds()), response)
		return
	}

	result, err := api.registry.List(kind, namespace)
	if err != nil {
		api.writeError(http.StatusInternalServerError, err.Error(), response)
		return
	}

	// Parse back to config
	out := []Config{}
	for k, v := range result {
		retrieved, errLocal := schema.ToJSON(v)
		if errLocal != nil {
			api.writeError(http.StatusInternalServerError, errLocal.Error(), response)
			return
		}
		var retJSON interface{}
		errLocal = json.Unmarshal([]byte(retrieved), &retJSON)
		if errLocal != nil {
			api.writeError(http.StatusInternalServerError, errLocal.Error(), response)
			return
		}
		config := Config{
			Name: k.Name,
			Type: k.Kind,
			Spec: retJSON,
		}
		out = append(out, config)
	}
	if err = response.WriteHeaderAndEntity(http.StatusOK, out); err != nil {
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
