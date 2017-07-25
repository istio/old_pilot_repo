package main

import (
	"istio.io/pilot/model"
)

const (
	userCookie      = "user"
	requestIDHeader = "X-Request-ID"
)

type productPage struct {
	Details map[string]string  `json:"details,omitempty"`
	Reviews map[string]*review `json:"reviews,omitempty"`
}

type review struct {
	Text   string  `json:"text,omitempty"`
	Rating *rating `json:"rating,omitempty"`
}

type rating struct {
	Stars int    `json:"stars,omitempty"`
	Color string `json:"color,omitempty"`
}

func MakeService(hostname, address string) *model.Service {
	return &model.Service{
		Hostname: hostname,
		Address:  address,
		Ports: []*model.Port{
			{
				Name:     "istio-http",
				Port:     9080, // target port 80
				Protocol: model.ProtocolHTTP,
			}},
	}
}
