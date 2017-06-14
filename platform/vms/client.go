package vms

import (
	"github.com/amalgam8/amalgam8/pkg/adapters/discovery/amalgam8"
	"github.com/amalgam8/amalgam8/registry/client"
)

const (
	IstioResourceVersion = "v1alpha1"
)

type Client client.Client

type ClientConfig struct {
	amalgam8.RegistryConfig
}

func NewClient(config ClientConfig) (*Client, error) {
	return client.New(client.Config(config))
}
