package vms

import (
	"github.com/amalgam8/amalgam8/registry/client"
)

const (
	IstioResourceVersion = "v1alpha1"
)

type Client struct {
	client.Client
}

type ClientConfig struct {
	client.Config
}

//func NewClient(config ClientConfig) (*Client, error) {
//	return 
//}
