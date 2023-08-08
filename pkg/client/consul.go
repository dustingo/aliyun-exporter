package client

import (
	capi "github.com/hashicorp/consul/api"
)

// init consul client
func NewConsulClient(address, token string) (*capi.Client, error) {
	client, err := capi.NewClient(&capi.Config{Address: address, Token: token})
	if err != nil {
		return nil, err
	}
	return client, err
}
