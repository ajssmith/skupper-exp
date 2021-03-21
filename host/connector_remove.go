package host

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
)

func (cli *hostClient) ConnectorRemove(name string) error {
	_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	cmd := exec.Command("systemctl", "check", "qdrouterd")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("Failed transport check, systemctl finished with non-zero: %w", exitErr)
		} else{
			return fmt.Errorf("Failed transport check, systemctl error: %w", err)
		}
	} else {
		status := strings.TrimSuffix(string(out),"\n")
		if status != "active" {
			return fmt.Errorf("Transport check %s (need init?)", status)
		}
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

	cmd = exec.Command("systemctl", "restart", "qdrouterd")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("Failed to restart qdrouterd service: %w", err)
	}

	//	err = driver.RecreateContainer("skupper-router", cli.CeDriver)
	//	if err != nil {
	//		return fmt.Errorf("Failed to re-start transport container: %w", err)
	//	}

	//	err = driver.RecreateContainer("skupper-service-controller", cli.CeDriver)
	//	if err != nil {
	//		return fmt.Errorf("Failed to re-start service controller container: %w", err)
	//	}

	// TODO: Note this is where cli Init might happen twice, is that ok?
	// restart proxies
	//	vsis, err := cli.ServiceInterfaceList()
	//	if err != nil {
	//		return fmt.Errorf("Failed to list proxies to restart: %w", err)
	//	}
	//	for _, vs := range vsis {
	//		fmt.Println("Need to restart: ", vs.Address)
	//		err = cli.CeDriver.ContainerRestart(vs.Address)
	//		if err != nil {
	//			return fmt.Errorf("Failed to restart proxy container: %w", err)
	//		}
	//	}

	return nil
}
