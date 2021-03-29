package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/skupperproject/skupper/pkg/certs"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/pkg/qdr"
	"github.com/ajssmith/skupper-exp/pkg/utils"
	"github.com/ajssmith/skupper-exp/pkg/utils/configs"
)

func getCertData(name string) (certs.CertificateData, error) {
	certData := certs.CertificateData{}
	certPath := types.GetSkupperPath(types.CertsPath) + "/" + name

	files, err := ioutil.ReadDir(certPath)
	if err == nil {
		for _, f := range files {
			dataString, err := ioutil.ReadFile(certPath + "/" + f.Name())
			if err == nil {
				certData[f.Name()] = []byte(dataString)
			} else {
				return certData, fmt.Errorf("Failed to read certificat data: %w", err)
			}
		}
	}
	return certData, err
}

func generateCredentials(ca string, name string, subject string, hosts []string, includeConnectJson bool) error {
	caData, _ := getCertData(ca)
	certData := certs.GenerateCertificateData(name, subject, strings.Join(hosts, ","), caData)

	for k, v := range certData {
		if err := ioutil.WriteFile(types.GetSkupperPath(types.CertsPath)+"/"+name+"/"+k, v, 0777); err != nil {
			return fmt.Errorf("Failed to write certificate file: %w", err)
		}
	}

	if includeConnectJson {
		certData["connect.json"] = []byte(configs.ConnectJSON())
		if err := ioutil.WriteFile(types.GetSkupperPath(types.CertsPath)+"/"+name+"/connect.json", []byte(configs.ConnectJSON()), 0777); err != nil {
			return fmt.Errorf("Failed to write connect file: %w", err)
		}
	}

	return nil
}

func ensureCA(name string) (certs.CertificateData, error) {

	// check if existing by looking at path/dir, if not create dir to persist
	caData := certs.GenerateCACertificateData(name, name)

	if err := os.Mkdir(types.GetSkupperPath(types.CertsPath)+"/"+name, 0777); err != nil {
		return nil, fmt.Errorf("Failed to create certificate directory: %w", err)
	}

	for k, v := range caData {
		if err := ioutil.WriteFile(types.GetSkupperPath(types.CertsPath)+"/"+name+"/"+k, v, 0777); err != nil {
			return nil, fmt.Errorf("Failed to write CA certificate file: %w", err)
		}
	}

	return caData, nil
}

func (cli *hostClient) GetRouterSpecFromOpts(options types.SiteConfigSpec, siteId string) (*types.RouterSpec, error) {
	van := &types.RouterSpec{}
	// 	//TODO: think througn van name, router name, secret names, etc.
	if options.SkupperName == "" {
		van.Name = "HostName"
	}

	// TODO: maybe have the option of rpm install or tarball and we create the service

	van.AuthMode = types.ConsoleAuthMode(options.AuthMode)

	// What is the e
	// 	van.Transport.LivenessPort = types.TransportLivenessPort
	// 	van.Transport.Labels = map[string]string{
	// 		"application":          types.TransportDeploymentName,
	// 		"skupper.io/component": types.TransportComponentName,
	// 		"prometheus.io/port":   "9090",
	// 		"prometheus.io/scrape": "true",
	// 	}

	routerConfig := qdr.InitialConfig(van.Name+"-${HOSTNAME}", siteId, options.IsEdge)
	routerConfig.AddAddress(qdr.Address{
		Prefix:       "mc",
		Distribution: "multicast",
	})
	routerConfig.AddListener(qdr.Listener{
		Host:        "0.0.0.0",
		Port:        9090,
		Role:        "normal",
		Http:        true,
		HttpRootDir: "disabled",
		Websockets:  false,
		Healthz:     true,
		Metrics:     true,
	})
	routerConfig.AddListener(qdr.Listener{
		Name: "amqp",
		Host: "localhost",
		Port: types.AmqpDefaultPort,
	})
	routerConfig.AddSslProfile(qdr.SslProfile{
		Name: "skupper-amqps",
	})
	routerConfig.AddListener(qdr.Listener{
		Name:             "amqps",
		Host:             "0.0.0.0",
		Port:             types.AmqpsDefaultPort,
		SslProfile:       "skupper-amqps",
		SaslMechanisms:   "EXTERNAL",
		AuthenticatePeer: true,
	})
	if options.EnableRouterConsole {
		if van.AuthMode == types.ConsoleAuthModeInternal {
			routerConfig.AddListener(qdr.Listener{
				Name:             types.ConsolePortName,
				Host:             "0.0.0.0",
				Port:             types.ConsoleDefaultServicePort,
				Http:             true,
				AuthenticatePeer: true,
			})
		} else if van.AuthMode == types.ConsoleAuthModeUnsecured {
			routerConfig.AddListener(qdr.Listener{
				Name: types.ConsolePortName,
				Host: "0.0.0.0",
				Port: types.ConsoleDefaultServicePort,
				Http: true,
			})
		}
	}
	if !options.IsEdge {
		routerConfig.AddSslProfile(qdr.SslProfile{
			Name: types.InterRouterProfile,
		})
		routerConfig.AddListener(qdr.Listener{
			Name:             "interior-listener",
			Host:             "0.0.0.0",
			Role:             qdr.RoleInterRouter,
			Port:             types.InterRouterListenerPort,
			SslProfile:       types.InterRouterProfile,
			SaslMechanisms:   "EXTERNAL",
			AuthenticatePeer: true,
		})
		routerConfig.AddListener(qdr.Listener{
			Name:             "edge-listener",
			Host:             "0.0.0.0",
			Role:             qdr.RoleEdge,
			Port:             types.EdgeListenerPort,
			SslProfile:       types.InterRouterProfile,
			SaslMechanisms:   "EXTERNAL",
			AuthenticatePeer: true,
		})
	}
	van.RouterConfig, _ = qdr.MarshalRouterConfig(routerConfig)

	envVars := map[string]string{
		"QDROUTERD_CONF":      "/etc/qpid-dispatch/config/" + types.TransportConfigFile,
		"QDROUTERD_CONF_TYPE": "json",
		"SKUPPER_SITE_ID":     siteId,
	}
	if !options.IsEdge {
		envVars["APPLICATION_NAME"] = types.TransportDeploymentName
		// 		// TODO: auto_mesh for non k8s deploy
		// 		//		envVars["QDROUTERD_AUTO_MESH_DISCOVERY"] = "QUERY"
	}
	if options.AuthMode == string(types.ConsoleAuthModeInternal) {
		envVars["QDROUTERD_AUTO_CREATE_SASLDB_SOURCE"] = "/etc/qpid-dispatch/sasl-users/"
		envVars["QDROUTERD_AUTO_CREATE_SASLDB_PATH"] = "/tmp/qdrouterd.sasldb"
	}
	if options.TraceLog {
		envVars["PN_TRACE_FRM"] = "1"
	}

	van.Transport.EnvVar = envVars

	volumes := []string{
		"skupper",
		"skupper-amqps",
		"router-config",
	}
	if !options.IsEdge {
		volumes = append(volumes, "skupper-internal")
	}
	if options.AuthMode == string(types.ConsoleAuthModeInternal) {
		volumes = append(volumes, "skupper-console-users")
		volumes = append(volumes, "skupper-sasl-config")
	}
	van.Transport.Volumes = volumes

	// Note: use index to make directory, use index/value to make mount
	mounts := make(map[string]string)
	mounts[types.GetSkupperPath(types.CertsPath)] = "/etc/qpid-dispatch-certs"
	mounts[types.GetSkupperPath(types.ConnectionsPath)] = "/etc/qpid-dispatch/connections"
	mounts[types.GetSkupperPath(types.ConfigPath)] = "/etc/qpid-dispatch/config"
	mounts[types.GetSkupperPath(types.ConsoleUsersPath)] = "/etc/qpid-dispatch/sasl-users/"
	mounts[types.GetSkupperPath(types.SaslConfigPath)] = "/etc/sasl2"
	van.Transport.Mounts = mounts

	cas := []types.CertAuthority{}
	cas = append(cas, types.CertAuthority{Name: "skupper-ca"})
	if !options.IsEdge {
		cas = append(cas, types.CertAuthority{Name: "skupper-internal-ca"})
	}
	van.CertAuthoritys = cas

	credentials := []types.Credential{}
	credentials = append(credentials, types.Credential{
		CA:          "skupper-ca",
		Name:        "skupper-amqps",
		Subject:     "skupper-messaging",
		Hosts:       []string{"skupper-router"},
		ConnectJson: false,
		Post:        false,
	})
	credentials = append(credentials, types.Credential{
		CA:          "skupper-ca",
		Name:        "skupper",
		Subject:     "skupper-messaging",
		Hosts:       []string{},
		ConnectJson: true,
		Post:        false,
	})
	if !options.IsEdge {
		credentials = append(credentials, types.Credential{
			CA:          "skupper-internal-ca",
			Name:        "skupper-internal",
			Subject:     "skupper-internal",
			Hosts:       []string{"skupper-router"},
			ConnectJson: false,
			Post:        false,
		})
	}
	van.Credentials = credentials

	// 	// Controller spec portion
	// 	if os.Getenv("SKUPPER_CONTROLLER_IMAGE") != "" {
	// 		van.Controller.Image = os.Getenv("SKUPPER_CONTROLLER_IMAGE")
	// 	} else {
	// 		van.Controller.Image = types.DefaultControllerImage
	// 	}
	// 	van.Controller.Labels = map[string]string{
	// 		"application":          types.ControllerDeploymentName,
	// 		"skupper.io/component": types.ControllerComponentName,
	// 	}
	// 	var skupperHost string
	// 	if runtime.GOOS == "linux" {
	// 		skupperHost = utils.GetInternalIP("docker0")
	// 	} else {
	// 		skupperHost = "host-gateway"
	// 	}
	// 	van.Controller.EnvVar = map[string]string{
	// 		"SKUPPER_SITE_ID":          siteId,
	// 		"SKUPPER_TMPDIR":           os.Getenv("SKUPPER_TMPDIR"),
	// 		"SKUPPER_PROXY_IMAGE":      van.Controller.Image,
	// 		"SKUPPER_HOST":             skupperHost,
	// 		"SKUPPER_CONTAINER_ENGINE": options.ContainerEngineDriver,
	// 	}
	// 	if options.MapToHost {
	// 		van.Controller.EnvVar["SKUPPER_MAP_TO_HOST"] = "true"
	// 	}
	// 	if options.TraceLog {
	// 		van.Controller.EnvVar["PN_TRACE_FRM"] = "1"
	// 	}
	// 	// van.Controller.EnvVar = []string{
	// 	// 	"SKUPPER_SITE_ID=" + siteId,
	// 	// 	"SKUPPER_TMPDIR=" + os.Getenv("SKUPPER_TMPDIR"),
	// 	// 	"SKUPPER_PROXY_IMAGE=" + van.Controller.Image,
	// 	// 	"SKUPPER_HOST=" + skupperHost,
	// 	// }
	// 	// if options.MapToHost {
	// 	// 	van.Controller.EnvVar = append(van.Controller.EnvVar, "SKUPPER_MAP_TO_HOST=true")
	// 	// }
	// 	// if options.TraceLog {
	// 	// 	van.Controller.EnvVar = append(van.Controller.EnvVar, "PN_TRACE_FRM=1")
	// 	// }

	// 	van.Controller.Mounts = map[string]string{
	// 		types.GetSkupperPath(types.CertsPath) + "/" + "skupper": "/etc/messaging",
	// 		types.GetSkupperPath(types.ServicesPath):                "/etc/messaging/services",
	// 		"/var/run":                                              "/var/run",
	// 	}

	return van, nil
}

// RouterCreate instantiates a VAN Router (transport and controller)
func (cli *hostClient) RouterCreate(options types.SiteConfigSpec) error {

	//TODO return error
	if options.EnableConsole {
		if options.AuthMode == string(types.ConsoleAuthModeInternal) || options.AuthMode == "" {
			options.AuthMode = string(types.ConsoleAuthModeInternal)
			if options.User == "" {
				options.User = "admin"
			}
			if options.Password == "" {
				options.Password = utils.RandomId(10)
			}
		} else {
			if options.User != "" {
				return fmt.Errorf("--router-console-user only valid when --router-console-auth=internal")
			}
			if options.Password != "" {
				return fmt.Errorf("--router-console-password only valid when --router-console-auth=internal")
			}
		}

	}

	// TODO check if resources already exist: either delete them all or error out
	// setup host dirs
	_ = os.RemoveAll(types.GetSkupperPath(types.HostPath))
	// create host dirs TODO this should not be here
	if err := os.MkdirAll(types.GetSkupperPath(types.HostPath), 0777); err != nil {
		return err
	}
	if err := os.Mkdir(types.GetSkupperPath(types.SitesPath), 0777); err != nil {
		return err
	}

	sc, err := cli.SiteConfigCreate(options)
	if err != nil {
		return err
	}

	van, err := cli.GetRouterSpecFromOpts(options, sc.UID)
	if err != nil {
		return err
	}

	for mnt := range van.Transport.Mounts {
		if err := os.Mkdir(mnt, 0777); err != nil {
			return err
		}
	}
	for _, v := range van.Transport.Volumes {
		if err := os.Mkdir(types.GetSkupperPath(types.CertsPath)+"/"+v, 0777); err != nil {
			return err
		}
	}

	// this one is needed by the controller
	if err := os.Mkdir(types.GetSkupperPath(types.ServicesPath), 0777); err != nil {
		return err
	}

	svcDefs := make(map[string]types.ServiceInterface)
	encoded, err := json.Marshal(svcDefs)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(types.GetSkupperPath(types.ServicesPath)+"/skupper-services", encoded, 0777)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(types.GetSkupperPath(types.ConfigPath)+"/qdrouterd.json", []byte(van.RouterConfig), 0777)
	if err != nil {
		return err
	}

	if options.EnableConsole && options.AuthMode == string(types.ConsoleAuthModeInternal) {
		config := `
pwcheck_method: auxprop
auxprop_plugin: sasldb
sasldb_path: /tmp/qdrouterd.sasldb
`
		err := ioutil.WriteFile(types.GetSkupperPath(types.SaslConfigPath)+"/qdrouterd.conf", []byte(config), 0777)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(types.GetSkupperPath(types.ConsoleUsersPath)+"/"+options.User, []byte(options.Password), 0777)
		if err != nil {
			return err
		}
	}

	for _, ca := range van.CertAuthoritys {
		ensureCA(ca.Name)
	}

	for _, cred := range van.Credentials {
		generateCredentials(cred.CA, cred.Name, cred.Subject, cred.Hosts, cred.ConnectJson)
	}

	fmt.Println("")
	//	cmd := exec.Command("dnf", "install", "-y", "qpid-dispatch-router")
	cmd := exec.Command("dnf", "help")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return err
	}

	qdrService := `
[Unit]
Description=Qpid Dispatch router daemon
Requires=network.target
After=network.target
	
[Service]
User=qdrouterd
Group=qdrouterd
Type=simple
ExecStart=/usr/sbin/qdrouterd -c /var/tmp/skupper/config/qdrouterd.json

[Install]
WantedBy=multi-user.target
`

	err = ioutil.WriteFile("/etc/systemd/system/qdrouterd.service", []byte(qdrService), 0777)
	if err != nil {
		return err
	}

	controllerService := `
[Unit]
Description=Skupper Controller Service
Requires=network.target
After=network.target
	
[Service]
User=skupper-controller
Group=skupper-controller
Type=simple
ExecStart=/usr/local/bin/controller

[Install]
WantedBy=multi-user.target
`

	err = ioutil.WriteFile("/etc/systemd/system/skupper-controller.service", []byte(controllerService), 0777)
	if err != nil {
		return err
	}

	cmd = exec.Command("groupadd", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("useradd", "-g", "skupper-controller", "-r", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "daemon-reload")
	err = cmd.Run()
	if err != nil {
		return err
	}

	// not sure why this is needed but for now
	cmd = exec.Command("chmod", "-R", "0777", "/var/tmp/skupper/")
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "start", "qdrouterd")
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "enable", "qdrouterd")
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "start", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("systemctl", "enable", "skupper-controller")
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
