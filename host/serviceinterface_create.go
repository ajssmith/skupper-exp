package host

import (
	"fmt"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *hostClient) ServiceInterfaceCreate(service *types.ServiceInterface) error {

	_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	//	err = cli.Init(sc.Spec.ContainerEngineDriver)
	//	if err != nil {
	//		return fmt.Errorf("Failed to intialize client: %w", err)
	//	}

	//	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	//	if err != nil {
	//		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	//	}

	err = validateServiceInterface(service)
	if err != nil {
		return err
	}
	return updateServiceInterface(service, false, cli)

}
