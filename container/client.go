package container

import (
	"fmt"

	"github.com/ajssmith/skupper-exp/driver"
)

// A VAN client manages orchestration and communication with the network components
type containerClient struct {
	CeDriver driver.Driver
}

var ContainerClient containerClient

//func NewClient() (*VanClient, error) {
//	c := &VanClient{}
//	// TODO: what init can we do here
//	return c, nil
//}

func New() error {
	fmt.Println("Inside container new")
	// TODO: what init can we do here
	return nil
}

func (cli *containerClient) Init(ced string) error {
	var drv driver.Driver

	fmt.Println("client init for ce: ", ced)
	if cli.CeDriver != nil {
		return nil
	}

	if ced == "docker" {
		drv = &driver.DockerDriver
	} else if ced == "podman" {
		drv = &driver.PodmanDriver
	} else {
		return fmt.Errorf("CE driver %s not recognized", ced)
	}

	err := drv.New()
	if err != nil {
		return fmt.Errorf("Error connecting to CE backend: %w", err)
	}
	cli.CeDriver = drv
	return nil
}
