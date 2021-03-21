package host

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
)

func (cli *hostClient) ConnectorList() ([]*types.Connector, error) {
	var connectors []*types.Connector

	_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return connectors, fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	cmd := exec.Command("systemctl", "check", "qdrouterd")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return connectors, fmt.Errorf("Failed transport check, systemctl finished with non-zero: %w", exitErr)
		} else{
			return connectors, fmt.Errorf("Failed transport check, systemctl error: %w", err)
		}
	} else {
		status := strings.TrimSuffix(string(out),"\n")
		if status != "active" {
			return connectors, fmt.Errorf("Transport check %s (need init?)", status)
		}
	}	

	current, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return connectors, fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	files, err := ioutil.ReadDir(types.GetSkupperPath(types.ConnectionsPath))
	if err != nil {
		return connectors, fmt.Errorf("Failed to read connector definitions: %w", err)
	}

	var role types.ConnectorRole
	var host []byte
	var port []byte
	var suffix string
	if current.IsEdge() {
		role = types.ConnectorRoleEdge
		suffix = "/edge-"
	} else {
		role = types.ConnectorRoleInterRouter
		suffix = "/inter-router-"
	}

	for _, f := range files {
		path := types.GetSkupperPath(types.ConnectionsPath)
		host, _ = ioutil.ReadFile(path + "/" + f.Name() + suffix + "host")
		port, _ = ioutil.ReadFile(path + "/" + f.Name() + suffix + "port")
		connectors = append(connectors, &types.Connector{
			Name: f.Name(),
			Host: string(host),
			Port: string(port),
			Role: string(role),
		})
	}
	return connectors, nil
}
