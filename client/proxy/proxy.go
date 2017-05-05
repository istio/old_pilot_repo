package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"

	"istio.io/manager/apiserver"
	"istio.io/manager/model"
)

// RESTRequester is yet another client wrapper for making REST
// calls. Ideally rest.Interface from "k8s.io/client-go/rest" would be
// used instead, but that returns not-interface types which makes it
// more difficult fake mock for unit-test, e.g. rest.Request.
type RESTRequester interface {
	Request(method, path string, inBody []byte) ([]byte, error)
}

// ManagerClient is a client wrapper that contains the base URL and API version
type ManagerClient struct {
	rr RESTRequester
}

// Client defines the interface for the proxy specific functionality of the manager client
type Client interface {
	GetConfig(model.Key) (*apiserver.Config, error)
	AddConfig(model.Key, apiserver.Config) error
	UpdateConfig(model.Key, apiserver.Config) error
	DeleteConfig(model.Key) error
	ListConfig(string, string) ([]apiserver.Config, error)
}

// NewManagerClient creates a new ManagerClient instance. It trims the apiVersion of leading and trailing slashes
// and the base path of trailing slashes to ensure consistency
func NewManagerClient(rr RESTRequester) *ManagerClient {
	return &ManagerClient{rr: rr}
}

func (m *ManagerClient) doConfigCRUD(key model.Key, method string, inBody []byte) ([]byte, error) {
	uriSuffix := fmt.Sprintf("config/%v/%v/%v", key.Kind, key.Namespace, key.Name)
	return m.rr.Request(method, uriSuffix, inBody)
}

// GetConfig retrieves the configuration resource for the passed key
func (m *ManagerClient) GetConfig(key model.Key) (*apiserver.Config, error) {
	body, err := m.doConfigCRUD(key, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}
	config := &apiserver.Config{}
	if err := json.Unmarshal(body, config); err != nil {
		return nil, err
	}
	return config, nil
}

// AddConfig creates a configuration resources for the passed key using the passed configuration
// It is idempotent
func (m *ManagerClient) AddConfig(key model.Key, config apiserver.Config) error {
	bodyIn, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if _, err = m.doConfigCRUD(key, http.MethodPost, bodyIn); err != nil {
		return err
	}
	return nil
}

// UpdateConfig updates the configuration resource for the passed key using the passed configuration
// It is idempotent
func (m *ManagerClient) UpdateConfig(key model.Key, config apiserver.Config) error {
	bodyIn, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if _, err = m.doConfigCRUD(key, http.MethodPut, bodyIn); err != nil {
		return err
	}
	return nil
}

// DeleteConfig deletes the configuration resource for the passed key
func (m *ManagerClient) DeleteConfig(key model.Key) error {
	_, err := m.doConfigCRUD(key, http.MethodDelete, nil)
	return err
}

// ListConfig retrieves all configuration resources of the passed kind in the given namespace
// If namespace is an empty string it retrieves all configs of the passed kind across all namespaces
func (m *ManagerClient) ListConfig(kind, namespace string) ([]apiserver.Config, error) {
	var reqURL string
	if namespace != "" {
		reqURL = fmt.Sprintf("config/%v/%v", kind, namespace)
	} else {
		reqURL = fmt.Sprintf("config/%v", kind)
	}
	body, err := m.rr.Request(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	var config []apiserver.Config
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	return config, nil
}
