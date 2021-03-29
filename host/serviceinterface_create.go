package host

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/ajssmith/skupper-exp/api/types"
)

func (cli *hostClient) ServiceInterfaceCreate(service *types.ServiceInterface) error {

	_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	cmd := exec.Command("systemctl", "check", "qdrouterd")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("Failed transport check, systemctl finished with non-zero: %w", exitErr)
		} else {
			return fmt.Errorf("Failed transport check, systemctl error: %w", err)
		}
	} else {
		status := strings.TrimSuffix(string(out), "\n")
		if status != "active" {
			return fmt.Errorf("Transport check %s (need init?)", status)
		}
	}

	err = validateServiceInterface(service)
	if err != nil {
		return err
	}
	return updateServiceInterface(service, false, cli)

}
