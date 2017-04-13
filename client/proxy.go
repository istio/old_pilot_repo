package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"istio.io/manager/apiserver"
	"istio.io/manager/model"
)

type ManagerClient struct {
	base             url.URL
	versionedAPIPath string
	client           *http.Client
}

func NewManagerClient(base url.URL, apiVersion string, client *http.Client) *ManagerClient {
	base.Path = strings.TrimSuffix(base.Path, "/")
	return &ManagerClient{
		base:             base,
		versionedAPIPath: strings.TrimPrefix(strings.TrimSuffix(apiVersion, "/"), "/"),
		client:           client,
	}
}

func (m *ManagerClient) do(request *http.Request) (*http.Response, error) {
	fullURL, err := url.Parse(fmt.Sprintf("%s/%s/%s",
		m.base.String(), m.versionedAPIPath, request.URL.String()))
	if err != nil {
		return nil, fmt.Errorf("unable to parse URL: %v", err)
	}
	request.URL = fullURL
	if err != nil {
		return nil, err
	}
	response, err := m.client.Do(request)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		body, err := ioutil.ReadAll(response.Body)
		if err == nil && len(body) > 0 {
			if response.StatusCode == 404 {
				return nil, &model.ItemNotFoundError{Msg: string(body)}
			}
			return nil, errors.New(string(body))
		}
		return nil, fmt.Errorf("received non-success status code %v", response.StatusCode)
	}
	return response, err
}

func (m *ManagerClient) GetConfig(key model.Key) (*apiserver.Config, error) {
	response, err := m.doConfigCRUD(key, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	config := &apiserver.Config{}
	if err = json.Unmarshal(body, config); err != nil {
		return nil, err
	}
	return config, nil
}

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

func (m *ManagerClient) DeleteConfig(key model.Key) error {
	if _, err := m.doConfigCRUD(key, http.MethodDelete, nil); err != nil {
		return err
	}
	return nil
}

func (m *ManagerClient) ListConfig(kind, namespace string) ([]apiserver.Config, error) {
	var reqURL string
	if namespace != "" {
		reqURL = fmt.Sprintf("config/%v/%v", kind, namespace)
	} else {
		reqURL = fmt.Sprintf("config/%v", kind)
	}
	request, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := m.do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	config := []apiserver.Config{}
	if err = json.Unmarshal(body, &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (m *ManagerClient) doConfigCRUD(key model.Key, method string, inBody []byte) (*http.Response, error) {
	reqURL := fmt.Sprintf("config/%v/%v/%v", key.Kind, key.Namespace, key.Name)
	var body io.Reader
	if inBody != nil && len(inBody) > 0 {
		body = bytes.NewBuffer(inBody)
	}
	request, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, err
	}
	return m.do(request)
}
