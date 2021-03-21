package container

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *containerClient) ServiceInterfaceInspect(address string) (*types.ServiceInterface, error) {

	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return nil, fmt.Errorf("Failed to intialize client: %w", err)
	}

	svcDefs := make(map[string]types.ServiceInterface)

	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	svcFile, err := ioutil.ReadFile(types.GetSkupperPath(types.ServicesPath) + "/skupper-services")
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}
	if vsi, ok := svcDefs[address]; !ok {
		return nil, nil
	} else {
		return &vsi, nil
	}
}
