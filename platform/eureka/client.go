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

package eureka

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type application struct {
	Name      string      `json:"name"`
	Instances []*instance `json:"instance"`
}

type instance struct {
	Hostname   string   `json:"hostName"`
	App        string   `json:"app"`
	IPAddress  string   `json:"ipAddr"`
	Status     string   `json:"status"`
	Port       *port    `json:"port,omitempty"`
	SecurePort *port    `json:"securePort,omitempty"`
	Metadata   metadata `json:"metadata,omitempty"`
}

type port struct {
	Port    int  `json:"$"`
	Enabled bool `json:"@enabled,string"`
}

type metadata map[string]string

// Client for Eureka
type Client interface {
	// Applications registered on the Eureka server
	Applications() ([]*application, error)
}

// Minimal client for Eureka server's REST APIs.
// TODO: support multiple Eureka servers
// TODO: caching
type client struct {
	client http.Client
	url    string
}

// NewClient instantiates a new Eureka client
func NewClient(url string) Client {
	return &client{
		client: http.Client{Timeout: 30 * time.Second},
		url:    url,
	}
}

const statusUp = "UP"

const (
	basePath = "/eureka/v2"
	appsPath = basePath + "/apps"
)

type getApplications struct {
	Applications applications `json:"applications"`
}

type applications struct {
	Applications []*application `json:"application"`
}

func (c *client) Applications() ([]*application, error) {
	req, err := http.NewRequest("GET", c.url+appsPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from Eureka server: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apps getApplications
	if err = json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}

	return apps.Applications.Applications, nil
}
