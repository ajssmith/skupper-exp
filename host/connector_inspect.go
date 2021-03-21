package host

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
)

func (cli *hostClient) ConnectorInspect(name string) (*types.ConnectorInspectResponse, error) {
	vci := &types.ConnectorInspectResponse{}
	var role types.ConnectorRole
	var suffix string

	_, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return vci, fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	cmd := exec.Command("systemctl", "check", "qdrouterd")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return vci, fmt.Errorf("Failed transport check, systemctl finished with non-zero: %w", exitErr)
		} else{
			return vci, fmt.Errorf("Failed transport check, systemctl error: %w", err)
		}
	} else {
		status := strings.TrimSuffix(string(out),"\n")
		if status != "active" {
			return vci, fmt.Errorf("Transport check %s (need init?)", status)
		}
	}	

	current, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return vci, fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	if current.IsEdge() {
		role = types.ConnectorRoleEdge
		suffix = "/edge-"
	} else {
		role = types.ConnectorRoleInterRouter
		suffix = "/inter-router-"
	}

	host, err := ioutil.ReadFile(types.GetSkupperPath(types.ConnectionsPath) + "/" + name + suffix + "host")
	if err != nil {
		return vci, fmt.Errorf("Could not retrieve connection-token files: %w", err)
	}
	port, err := ioutil.ReadFile(types.GetSkupperPath(types.ConnectionsPath) + "/" + name + suffix + "port")
	if err != nil {
		return vci, fmt.Errorf("Could not retrieve connection-token files: %w", err)
	}
	vci.Connector = &types.Connector{
		Name: name,
		Host: string(host),
		Port: string(port),
		Role: string(role),
	}

	connections, err := qdr.GetConnections(true, nil)

	if err == nil {
		connection := qdr.GetInterRouterOrEdgeConnection(vci.Connector.Host+":"+vci.Connector.Port, connections)
		if connection == nil || !connection.Active {
			vci.Connected = false
		} else {
			vci.Connected = true
		}
		return vci, nil
	}
	return vci, fmt.Errorf("Unable to get connections from transport: %w", err)

}
