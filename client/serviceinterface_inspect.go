package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *VanClient) ServiceInterfaceInspect(address string) (*types.ServiceInterface, error) {
	// TODO: query site config to get patch and ce
	cli.Init("/usr/lib64/skupper-plugins", "docker")

	svcDefs := make(map[string]types.ServiceInterface)

	_, err := cli.CeDriver.ContainerInspect("skupper-router")
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
