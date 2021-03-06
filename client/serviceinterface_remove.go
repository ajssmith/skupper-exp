package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *VanClient) ServiceInterfaceRemove(address string) error {
	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return fmt.Errorf("Failed to intialize client: %w", err)
	}

	svcDefs := make(map[string]types.ServiceInterface)

	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	svcFile, err := ioutil.ReadFile(types.GetSkupperPath(types.ServicesPath) + "/skupper-services")
	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service interace definitions: %w", err)
	}
	err = json.Unmarshal([]byte(svcFile), &svcDefs)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service interface definitions: %w", err)
	}

	if _, ok := svcDefs[address]; !ok {
		return fmt.Errorf("Unexpose service interface definition not found")
	}

	delete(svcDefs, address)

	encoded, err := json.Marshal(svcDefs)
	if err != nil {
		return fmt.Errorf("Failed to encode json for service interface: %w", err)
	}

	err = ioutil.WriteFile(types.GetSkupperPath(types.ServicesPath)+"/skupper-services", encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service interface file: %w", err)
	}

	return nil
}
