package client

import (
	"fmt"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
	"github.com/skupperproject/skupper/pkg/certs"
)

func (cli *VanClient) ConnectorTokenCreate(subject string, secretFile string) error {
	// TODO: query site config to get patch and ce
	cli.Init("/usr/lib64/skupper-plugins", "docker")

	// verify that the transport is interior mode
	router, err := cli.CeDriver.ContainerInspect("skupper-router")
	if err != nil {
		return fmt.Errorf("Unable to retrieve transport container (need init?): %w", err)
	}

	current, err := qdr.GetRouterConfigFromFile(types.GetSkupperPath(types.ConfigPath) + "/qdrouterd.json")
	if err != nil {
		return fmt.Errorf("Failed to retrieve router config: %w", err)
	}

	if current.IsEdge() {
		return fmt.Errorf("Edge mode transport configuration cannot accept connections")
	}

	caData, err := getCertData("skupper-internal-ca")
	if err != nil {
		return fmt.Errorf("Unable to retrieve CA data: %w", err)
	}

	// TODO add to driver
	ipAddr := router.NetworkSettings.IPAddress
	//ipAddr := string(router.NetworkSettings.Networks["skupper-network"].IPAddress)
	annotations := make(map[string]string)
	annotations["inter-router-port"] = "55671"
	annotations["inter-router-host"] = ipAddr

	sc, err := cli.SiteConfigInspect(types.DefaultBridgeName)
	if err != nil {
		return fmt.Errorf("Unable to retrieve site config data: %w", err)
	}
	annotations[types.TokenGeneratedBy] = sc.UID

	// TODO err return from certs pkg
	certData := certs.GenerateCertificateData(subject, subject, ipAddr, caData)
	certs.PutCertificateData(subject, secretFile, certData, annotations)

	return nil
}
