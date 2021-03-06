package client

import (
	"fmt"
	"path/filepath"
	"plugin"

	"github.com/ajssmith/skupper-exp/driver"
)

// A VAN client manages orchestration and communication with the network components
type VanClient struct {
	CeDriver driver.Driver
}

func NewClient() (*VanClient, error) {
	c := &VanClient{}
	// TODO: what init can we do here
	return c, nil
}

func (cli *VanClient) Init(ced string) error {
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

func (cli *VanClient) init2(path string, ced string) error {
	var p *plugin.Plugin

	if cli.CeDriver != nil {
		return nil
	}

	module := fmt.Sprintf("%s/%s.so", path, ced)
	fmt.Println("In client init: ", module)
	plugins, err := filepath.Glob(module)
	if err != nil {
		return err
	} else {
		fmt.Println("Plugin found: ", plugins[0])
	}

	if p, err = plugin.Open(module); err != nil {
		fmt.Println("plugin error: ", err.Error())
		return err
	}

	symDriver, err := p.Lookup("Driver")
	drv, ok := symDriver.(driver.Driver)
	if !ok {
		return fmt.Errorf("Plugin % is not a driver", module)
	} else {
		fmt.Println("Plugin IS a driver")
		err = drv.New()
		if err != nil {
			return fmt.Errorf("Error connecting to ce backend: %w", err)
		}
		cli.CeDriver = drv
	}

	return nil
}
