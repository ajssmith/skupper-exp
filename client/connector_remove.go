package client

import (
	"fmt"
	"os"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/driver"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
)

func (cli *VanClient) ConnectorRemove(name string) error {
	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return fmt.Errorf("Failed to intialize client: %w", err)
	}

	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return fmt.Errorf("Failed to retrieve transport container: %w", err)
	}

	current, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	found := current.RemoveConnector(name)
	if found {
		current.RemoveConnSslProfile(name)

		err = os.RemoveAll(types.GetSkupperPath(types.ConnectionsPath) + "/" + name)
		if err != nil {
			return fmt.Errorf("Failed to remove connector file contents: %w", err)
		}

		err = current.WriteToConfigFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
		if err != nil {
			return fmt.Errorf("Failed to update router config file: %w", err)
		}
	}

	err = driver.RecreateContainer("skupper-router", cli.CeDriver)
	if err != nil {
		return fmt.Errorf("Failed to re-start transport container: %w", err)
	}

	err = driver.RecreateContainer("skupper-service-controller", cli.CeDriver)
	if err != nil {
		return fmt.Errorf("Failed to re-start service controller container: %w", err)
	}

	// TODO: Note this is where cli Init might happen twice, is that ok?
	// restart proxies
	vsis, err := cli.ServiceInterfaceList()
	if err != nil {
		return fmt.Errorf("Failed to list proxies to restart: %w", err)
	}
	for _, vs := range vsis {
		fmt.Println("Need to restart: ", vs.Address)
		err = cli.CeDriver.ContainerRestart(vs.Address)
		if err != nil {
			return fmt.Errorf("Failed to restart proxy container: %w", err)
		}
	}

	return nil
}
