package eureka

import (
	"net/http"
	"encoding/json"
	"io/ioutil"
	"fmt"
	"time"
)

// TODO: supposedly Eureka uses the key 'application' for 1, or 'applications' for 2+
type GetApplications struct {
	Applications Applications `json:"applications"`
}

type Applications struct {
	Applications []*Application `json:"application"`
}

type Application struct {
	Name string `json:"name"`
	Instances []*Instance `json:"instance"`
}

type Instance struct {
	Hostname string `json:"hostName"`
	App string `json:"app"`
	IPAddress string `json:"ipAddr"`
	Port *Port `json:"port,omitempty"`
	SecurePort *Port `json:"securePort,omitempty"`
	Metadata Metadata `json:"metadata,omitempty"`
}

type Port struct {
	Port int `json:"$"`
	Enabled bool `json:"@enabled,string"`
}

type Metadata map[string]string

func NewClient(url string) Client {
	return &client{
		client: http.Client{Timeout: 30 * time.Second},
		url: url,
	}
}

type Client interface {
	Applications() ([]*Application, error)
}

type client struct {
	client http.Client
	url string
}

func (c *client) Applications() ([]*Application, error) {
	req, err := http.NewRequest("GET", c.url + "/eureka/v2/apps", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from Eureka server: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apps GetApplications
	if err = json.Unmarshal(data, &apps); err != nil {
		return nil, err
	}

	return apps.Applications.Applications, nil
}
