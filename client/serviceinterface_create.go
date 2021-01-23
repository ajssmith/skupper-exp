package client

import (
	"fmt"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *VanClient) ServiceInterfaceCreate(service *types.ServiceInterface) error {
	// TODO: query site config to get patch and ce
	cli.Init("/usr/lib64/skupper-plugins", "docker")

	_, err := cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	err = validateServiceInterface(service)
	if err != nil {
		return err
	}
	return updateServiceInterface(service, false, cli)

}
