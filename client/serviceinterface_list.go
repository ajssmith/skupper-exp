package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *VanClient) ServiceInterfaceList() ([]types.ServiceInterface, error) {
	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return nil, fmt.Errorf("Failed to intialize client: %w", err)
	}

	var vsis []types.ServiceInterface
	svcDefs := make(map[string]types.ServiceInterface)

	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	svcFile, err := ioutil.ReadFile(types.GetSkupperPath(types.ServicesPath) + "/skupper-services")
	if err != nil {
		return vsis, err
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return vsis, fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	for _, v := range svcDefs {
		_, err := cli.CeDriver.ContainerInspect(v.Address)
		if err == nil {
			// TODO: driver network settings
			v.Alias = "10.10.10.1"
			//			v.Alias = string(current.NetworkSettings.Networks["skupper-network"].IPAddress)
		}
		vsis = append(vsis, v)
	}

	return vsis, err
}
