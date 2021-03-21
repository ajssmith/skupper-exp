package host

import (
	"fmt"
)

type hostClient struct {
	something string
}

var HostClient hostClient

func New() error {
	fmt.Println("Inside host new")
	// TODO: what init can we do here
	return nil
}

func (cli *hostClient) Init(something string) error {
	cli.something = something

	return nil
}
