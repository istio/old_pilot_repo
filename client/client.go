package client

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"istio.io/manager/model"
)

type ManagerClient struct {
	base             *url.URL
	versionedAPIPath string
	client           *http.Client
}

func NewManagerClient(base *url.URL, apiVersion string, client *http.Client) *ManagerClient {
	trimmedBase := base
	if strings.HasSuffix(trimmedBase.Path, "/") {
		trimmedBase.Path = trimmedBase.Path[:len(trimmedBase.Path)-1]
	}
	trimmedVersion := apiVersion
	if strings.HasSuffix(trimmedVersion, "/") {
		trimmedVersion = trimmedVersion[:len(trimmedVersion)-1]
	}
	if strings.HasPrefix(trimmedVersion, "/") {
		trimmedVersion = trimmedVersion[1:]
	}
	if client == nil {
		client = &http.Client{}
	}
	return &ManagerClient{
		base:             trimmedBase,
		versionedAPIPath: trimmedVersion,
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
	response, err := m.client.Do(request)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		body, err := ioutil.ReadAll(response.Body)
		if err == nil && len(body) > 0 {
			if response.StatusCode == 404 {
				return nil, &model.ItemNotFoundError{Msg: string(body)}
			}
			return nil, errors.New(string(body))
		} else {
			return nil, fmt.Errorf("received non-success status code %v", response.StatusCode)
		}
	}
	return response, err
}
