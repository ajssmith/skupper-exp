package client

import (
	"fmt"
	"io/ioutil"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
)

func (cli *VanClient) ConnectorList() ([]*types.Connector, error) {
	var connectors []*types.Connector

	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return connectors, fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return connectors, fmt.Errorf("Failed to intialize client: %w", err)
	}

	// verify that the transport is interior mode
	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return connectors, fmt.Errorf("Unable to retrieve transport container (need init?): %w", err)
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
