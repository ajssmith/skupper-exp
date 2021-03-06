package client

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/driver"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
	"github.com/skupperproject/skupper/pkg/certs"
)

func generateConnectorName(path string) (string, error) {
	files, err := ioutil.ReadDir(path)
	max := 1
	if err == nil {
		connectorNamePattern := regexp.MustCompile("conn([0-9])+")
		for _, f := range files {
			count := connectorNamePattern.FindStringSubmatch(f.Name())
			if len(count) > 1 {
				v, _ := strconv.Atoi(count[1])
				if v >= max {
					max = v + 1
				}
			}
		}
	} else {
		return "", fmt.Errorf("Could not retrieve configured connectors (need init?): %w", err)
	}
	return "conn" + strconv.Itoa(max), nil
}

func (cli *VanClient) ConnectorCreate(secretFile string, options types.ConnectorCreateOptions) (string, error) {

	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return "", fmt.Errorf("Unable to retrieve site config: %w", err)
	}

	err = cli.Init(sc.Spec.ContainerEngineDriver)
	if err != nil {
		return "", fmt.Errorf("Failed to intialize client: %w", err)
	}

	// TODO certs should return err
	secret, err := certs.GetSecretContent(secretFile)
	if err != nil {
		return "", fmt.Errorf("Failed to make connector: %w", err)
	}

	_, err = cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve transport container (need init?): %w", err)
	}

	generatedBy, ok := secret["skupper.io/generated-by"]
	if !ok {
		return "", fmt.Errorf("Cannot find secret origin for token '%s'", secretFile)
	}

	if sc.UID == string(generatedBy) {
		return "", fmt.Errorf("Cannot create connection to self with token '%s'", secretFile)
	}

	if options.Name == "" {
		options.Name, err = generateConnectorName(types.GetSkupperPath(types.ConnectionsPath))
		if err != nil {
			return "", err
		}
	}
	connPath := types.GetSkupperPath(types.ConnectionsPath) + "/" + options.Name

	if err := os.Mkdir(connPath, 0755); err != nil {
		return "", fmt.Errorf("Failed to create skupper connector directory: %w", err)
	}
	for k, v := range secret {
		if k == types.TokenGeneratedBy {
			if err := ioutil.WriteFile(connPath+"/generated-by", v, 0755); err != nil {
				return "", fmt.Errorf("Failed to write connector file: %w", err)
			}
		} else {
			if err := ioutil.WriteFile(connPath+"/"+k, v, 0755); err != nil {
				return "", fmt.Errorf("Failed to write connector certificate file: %w", err)
			}
		}
	}

	current, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	profileName := options.Name + "-profile"
	current.AddConnSslProfile(qdr.SslProfile{
		Name: profileName,
	})
	connector := qdr.Connector{
		Name:       options.Name,
		Cost:       options.Cost,
		SslProfile: profileName,
	}
	if current.IsEdge() {
		hostString, _ := ioutil.ReadFile(connPath + "/edge-host")
		portString, _ := ioutil.ReadFile(connPath + "/edge-port")
		connector.Host = string(hostString)
		connector.Port = string(portString)
		connector.Role = qdr.RoleEdge
	} else {
		hostString, _ := ioutil.ReadFile(connPath + "/inter-router-host")
		portString, _ := ioutil.ReadFile(connPath + "/inter-router-port")
		connector.Host = string(hostString)
		connector.Port = string(portString)
		connector.Role = qdr.RoleInterRouter
	}
	current.AddConnector(connector)
	err = current.WriteToConfigFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return "", fmt.Errorf("Failed to update router config file: %w", err)
	}

	err = driver.RecreateContainer("skupper-router", cli.CeDriver)
	if err != nil {
		return "", fmt.Errorf("Failed to re-start transport container: %w", err)
	}

	err = driver.RecreateContainer("skupper-service-controller", cli.CeDriver)
	if err != nil {
		return "", fmt.Errorf("Failed to re-start service controller container: %w", err)
	}

	// restart proxies
	vsis, err := cli.ServiceInterfaceList()
	if err != nil {
		return "", fmt.Errorf("Failed to list proxies to restart: %w", err)
	}
	for _, vs := range vsis {
		fmt.Println("Need to restart container", vs.Address)
		err = cli.CeDriver.ContainerRestart(vs.Address)
		if err != nil {
			return "", fmt.Errorf("Failed to restart proxy container: %w", err)
		}
	}

	return options.Name, nil
}
