package host

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/ajssmith/skupper-exp/api/types"
)

//TODO should there be remove options

// RouterRemove delete a VAN (transport and controller) deployment
func (cli *hostClient) RouterRemove() []error {
	results := []error{}

	_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return append(results, fmt.Errorf("Unable to retrieve site config: %w", err))
	}

	//	err = cli.Init(sc.Spec.ContainerEngineDriver)
	//	if err != nil {
	//		return append(results, fmt.Errorf("Failed to intialize client: %w", err))
	//	}

	//	_, err = cli.CeDriver.ContainerInspect(types.ControllerDeploymentName)
	//	if err == nil {
	//		// stop controller
	//		err = cli.CeDriver.ContainerStop(types.ControllerDeploymentName)
	//		if err != nil {
	//			results = append(results, fmt.Errorf("Could not stop controller container: %w", err))
	//		} else {
	//			err = cli.CeDriver.ContainerRemove(types.ControllerDeploymentName)
	//			if err != nil {
	//				results = append(results, fmt.Errorf("Could not remove controller container: %w", err))
	//			}
	//		}
	//	}

	// remove proxies
	//	filters := map[string][]string{
	//		"label": {"skupper.io/component"},
	//	}
	//	opts := driver.ContainerListOptions{
	//		Filters: filters,
	//		All:     true,
	//	}
	//	containers, err := cli.CeDriver.ContainerList(opts)
	//	if err == nil {
	//		for _, container := range containers {
	//			if value, ok := container.Labels["skupper.io/component"]; ok {
	//				if value == "proxy" {
	//					err := cli.CeDriver.ContainerStop(container.ID)
	//					if err != nil {
	//						results = append(results, fmt.Errorf("Failed to stop proxy container: %w", err))
	//					} else {
	//						err = cli.CeDriver.ContainerRemove(container.ID)
	//						if err != nil {
	//							results = append(results, fmt.Errorf("Failed to remove proxy container: %w", err))
	//						}
	//					}
	//				}
	//			}
	//		}
	//	} else {
	//		results = append(results, fmt.Errorf("Failed to list proxy containers: %w", err))
	//	}

	//	_, err = cli.CeDriver.ContainerInspect(types.TransportDeploymentName)
	//	if err == nil {
	//		// stop transport
	//		err = cli.CeDriver.ContainerStop(types.TransportDeploymentName)
	//		if err != nil {
	//			results = append(results, fmt.Errorf("Could not stop transport container: %w", err))
	//		} else {
	//			err = cli.CeDriver.ContainerRemove(types.TransportDeploymentName)
	//			if err != nil {
	//				results = append(results, fmt.Errorf("Could not remove controller container: %w", err))
	//			}
	//		}
	//	}

	//	_, err = cli.CeDriver.NetworkInspect("skupper-network")
	//	if err == nil {
	//		// remove network
	//		err = cli.CeDriver.NetworkRemove("skupper-network")
	//		if err != nil {
	//			results = append(results, fmt.Errorf("Could not remove skupper network: %w", err))
	//		}
	//	}

	// removed qdrouterd - should we stop it first?
	cmd := exec.Command("dnf", "help")
	//	cmd := exec.Command("dnf", "remove", "-y", "qpid-dispatch-router")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to remove qpid-dispatch-router: %w", err))
	}

	cmd = exec.Command("systemctl", "stop", "qdrouterd")
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to daemon-reload systemctl: %w", err))
	}

	cmd = exec.Command("systemctl", "stop", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to daemon-reload systemctl: %w", err))
	}

	cmd = exec.Command("systemctl", "disable", "qdrouterd")
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to daemon-reload systemctl: %w", err))
	}

	cmd = exec.Command("systemctl", "disable", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to daemon-reload systemctl: %w", err))
	}

	// remove host files
	err = os.RemoveAll(types.GetSkupperPath(types.HostPath))
	if err != nil {
		results = append(results, fmt.Errorf("Failed to remove skupper files and directory: %w", err))
	}

	// remove service file
	err = os.Remove("/etc/systemd/system/qdrouterd.service")
	if err != nil {
		results = append(results, fmt.Errorf("Failed to remove qdrouterd service file: %w", err))
	}

	err = os.Remove("/etc/systemd/system/skupper-controller.service")
	if err != nil {
		results = append(results, fmt.Errorf("Failed to remove skupper-controller service file: %w", err))
	}

	cmd = exec.Command("systemctl", "daemon-reload")
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to daemon-reload systemctl: %w", err))
	}

	cmd = exec.Command("userdel", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		results = append(results, fmt.Errorf("Failed to userdel: %w", err))
	}

	return results
}
