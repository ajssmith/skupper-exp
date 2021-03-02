package client

import (
	"fmt"
	"log"
	"time"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
)

func (cli *VanClient) RouterInspect() (*types.RouterInspectResponse, error) {
	vir := &types.RouterInspectResponse{}

	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return vir, fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return vir, fmt.Errorf("Failed to intialize client: %w", err)
	}

	transport, err := cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		log.Println("Failed to retrieve transport container (need init?): ", err.Error())
		return vir, err
	}

	vir.TransportVersion, err = cli.CeDriver.ImageVersion(transport.Config.Image)
	if err != nil {
		log.Println("Failed to retrieve transport container version:", err.Error())
		return vir, err
	}
	vir.Status.State = transport.State.Status

	controller, err := cli.CeDriver.ContainerInspect(types.ControllerDeploymentName)
	if err != nil {
		log.Println("Failed to retrieve controller container (need init?): ", err.Error())
		return vir, err
	}

	vir.ControllerVersion, err = cli.CeDriver.ImageVersion(controller.Config.Image)
	if err != nil {
		log.Println("Failed to retrieve controller container version:", err.Error())
		return vir, err
	}

	routerConfig, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return vir, fmt.Errorf("Failed to retrieve router config: %w", err)
	}
	vir.Status.Mode = string(routerConfig.Metadata.Mode)

	connected, err := qdr.GetConnectedSites(cli.CeDriver)
	for i := 0; i < 5 && err != nil; i++ {
		time.Sleep(500 * time.Millisecond)
		connected, err = qdr.GetConnectedSites(cli.CeDriver)
	}
	if err != nil {
		return vir, err
	}
	vir.Status.ConnectedSites = connected

	vsis, err := cli.ServiceInterfaceList()
	if err != nil {
		vir.ExposedServices = 0
	} else {
		vir.ExposedServices = len(vsis)
	}

	return vir, err
}
