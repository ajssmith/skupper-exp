package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
//	"os"
	"strings"
	"strconv"
	"time"

	amqp "github.com/interconnectedcloud/go-amqp"

	"github.com/ajssmith/skupper-exp/api/types"
//	"github.com/ajssmith/skupper-exp/client"
	"github.com/ajssmith/skupper-exp/driver"
	"github.com/ajssmith/skupper-exp/pkg/docker"
	"github.com/fsnotify/fsnotify"
)

type Controller struct {
	origin    string
//	vanClient *client.VanClient

	// controller loop state
	bindings map[string]*ServiceBindings

	// service_sync statue
	tlsConfig       *tls.Config
	amqpClient      *amqp.Client
	amqpSession     *amqp.Session
	byOrigin        map[string]map[string]types.ServiceInterface
	localServices   map[string]types.ServiceInterface
	byName          map[string]types.ServiceInterface
	desiredServices map[string]types.ServiceInterface
	heardFrom       map[string]time.Time
}

func equivalentProxyConfig(desired types.ServiceInterface, env []string) bool {
	envVar := docker.FindEnvVar(env, "SKUPPER_PROXY_CONFIG")
	encodedDesired, _ := json.Marshal(desired)
	return string(encodedDesired) == envVar
}

func NewController(origin string, tlsConfig *tls.Config) (*Controller, error) {
//func NewController(cli *client.VanClient, origin string, tlsConfig *tls.Config) (*Controller, error) {
	controller := &Controller{
//		vanClient: cli,
		origin:    origin,
		tlsConfig: tlsConfig,
	}

	// Organize service definitions
	controller.bindings = make(map[string]*ServiceBindings)
	controller.byOrigin = make(map[string]map[string]types.ServiceInterface)
	controller.localServices = make(map[string]types.ServiceInterface)
	controller.byName = make(map[string]types.ServiceInterface)
	controller.desiredServices = make(map[string]types.ServiceInterface)
	controller.heardFrom = make(map[string]time.Time)

	// could setup watchers here

	return controller, nil
}

func (c *Controller) Run(stopCh <-chan struct{}) error {
	log.Println("Starting the Skupper controller")

//	var imageName string
//	if os.Getenv("QDROUTERD_IMAGE") != "" {
//		imageName = os.Getenv("QDROUTERD_IMAGE")
//	} else {
//		imageName = types.DefaultTransportImage
//	}

//	log.Println("Pulling proxy image", c.vanClient.CeDriver)
//	_, err := c.vanClient.CeDriver.ImagesPull(imageName, driver.ImagePullOptions{})
//	if err != nil {
//		log.Fatal("Failed to pull proxy image: ", err.Error())
//	}

	log.Println("Starting workers")
	go c.runServiceSync() // receives peer updates
	go c.runServiceDefsWatcher()

	log.Println("Started workers")
	<-stopCh
	log.Println("Shutting down workers")

	return nil
}

func usePort(proto, port string) bool {

	ln, err := net.Listen(proto, ":" + port)

	if err != nil {
  		fmt.Println("Can't listen on port %q: %s", port, err)
  		return false
	}

	err = ln.Close()
	if err != nil {
	  	fmt.Println("Couldn't stop listening on port %q: %s", port, err)
  		return false
	}

	fmt.Printf("TCP Port %q is available", port)
	return true
}

func updateSkupperServices(changed []types.ServiceInterface, deleted []string, origin string) error {
	if len(changed) == 0 && len(deleted) == 0 {
		return nil
	}

	current := make(map[string]types.ServiceInterface)
//	file, err := ioutil.ReadFile("/etc/messaging/services/skupper-services")
	file, err := ioutil.ReadFile("/var/tmp/skupper/services/skupper-services")

	if err != nil {
		return fmt.Errorf("Failed to retrieve skupper service definitions: %w", err)
	}
	err = json.Unmarshal([]byte(file), &current)
	if err != nil {
		return fmt.Errorf("Failed to decode json for service definitions: %w", err)
	}

	for _, def := range changed {
		current[def.Address] = def
	}

	for _, name := range deleted {
		delete(current, name)
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("Failed to encode json for service interface: %w", err)
	}

//	err = ioutil.WriteFile("/etc/messaging/services/skupper-services", encoded, 0755)
	err = ioutil.WriteFile("/var/tmp/skupper/services/skupper-services", encoded, 0755)
	if err != nil {
		return fmt.Errorf("Failed to write service file: %w", err)
	}
	return nil
}

func getServiceDefinitions() (map[string]types.ServiceInterface, error) {
	svcDefs := make(map[string]types.ServiceInterface)
//	file, err := ioutil.ReadFile("/etc/messaging/services/skupper-services")	
	file, err := ioutil.ReadFile("/var/tmp/skupper/services/skupper-services")
	if err != nil {
		return svcDefs, fmt.Errorf("Failed to retrieve skupper service definitions: %w", err)
	}
	err = json.Unmarshal([]byte(file), &svcDefs)
	if err != nil {
		return svcDefs, fmt.Errorf("Failed to decode json for service definitions: %w", err)
	}
	return svcDefs, nil
}

func (c *Controller) ensureProxyFor(bindings *ServiceBindings) error {
	port := strconv.Itoa(bindings.publicPort)
    usePort := usePort(bindings.protocol, port)
	fmt.Println("Can use port:", bindings.publicPort, usePort)

	proxies := c.getProxies()
	_, exists := proxies[bindings.address]
	serviceInterface := asServiceInterface(bindings)

	if bindings.origin == "" {
		fmt.Println("Ensure proxy for linux host")
//		attached := make(map[string]bool)
//		sn, err := c.vanClient.CeDriver.NetworkInspect(types.TransportNetworkName)
//		if err != nil {
//			return fmt.Errorf("Unable to retrieve skupper-network: %w", err)
//		}
//		for _, c := range sn.Containers {
//			attached[c.Name] = true
//		}

//		for _, t := range bindings.targets {
//			if t.selector == "internal.skupper.io/container" {
//				if _, ok := attached[t.name]; !ok {
//					fmt.Println("Attaching container to skupper network: ", t.service)
//					err := c.vanClient.CeDriver.NetworkConnect(types.TransportNetworkName, t.name, []string{})
//					if err != nil {
//						log.Println("Failed to attach target container to skupper network: ", err.Error())
//					}
//				}
//			}
//		}
	}

	// config, _ := qdr.GetRouterConfigForProxy(serviceInterface, c.origin)
	// mapToHost := false
	// if os.Getenv("SKUPPER_MAP_TO_HOST") != "" {
	// 	mapToHost = true
	// }

	if !exists {
		log.Println("Deploying proxy: ", serviceInterface.Address)
		// proxyContainer, err := docker.NewProxyContainer(serviceInterface, config, mapToHost, c.vanClient.DockerInterface)
		// if err != nil {
		// 	return fmt.Errorf("Failed to create proxy container: %w", err)
		// }
		// err = docker.StartContainer(proxyContainer.Name, c.vanClient.DockerInterface)
		// if err != nil {
		// 	return fmt.Errorf("Failed to start proxy container: %w", err)
		// }
	} else {
		log.Println("ReDeploying proxy: ", serviceInterface.Address)
		// proxyContainer, err := c.vanClient.CeDriver.ContainerInspect(serviceInterface.Address)
		// if err != nil {
		// 	return fmt.Errorf("Failed to retrieve current proxy container: %w", err)
		// }
		// actualConfig := "Please fix this"
		// //		actualConfig := docker.FindEnvVar(proxyContainer.Config.Env, "QDROUTERD_CONF")
		// if actualConfig == "" || actualConfig != config {
		// 	log.Println("Updating proxy config for: ", serviceInterface.Address)
		// 	err := c.deleteProxy(serviceInterface.Address)
		// 	if err != nil {
		// 		return fmt.Errorf("Failed to delete proxy container: %w", err)
		// 	}
		// 	newProxyContainer, err := docker.NewProxyContainer(serviceInterface, config, mapToHost, c.vanClient.DockerInterface)
		// 	if err != nil {
		// 		return fmt.Errorf("Failed to re-create proxy container: %w", err)
		// 	}
		// 	err = docker.StartContainer(newProxyContainer.Name, c.vanClient.DockerInterface)
		// 	if err != nil {
		// 		return fmt.Errorf("Failed to start proxy container: %w", err)
		// 	}
		//}
	}
	return nil

}

func (c *Controller) deleteProxy(name string) error {
	fmt.Println("Do delete proxy for linux host")
//	err := c.vanClient.CeDriver.ContainerStop(name)
//	if err != nil {
//		return err
//	}
//	err = c.vanClient.CeDriver.ContainerRemove(name)
	return nil
}

func (c *Controller) updateProxies() {
	for _, v := range c.bindings {
		err := c.ensureProxyFor(v)
		if err != nil {
			log.Println("Unable to ensure proxy container: ", err.Error())
		}
	}
	proxies := c.getProxies()
	for _, v := range proxies {
		proxyContainerName := strings.TrimPrefix(v.Names[0], "/")
		def, ok := c.bindings[proxyContainerName]
		if !ok || def == nil {
			c.deleteProxy(proxyContainerName)
		}
	}
}

func (c *Controller) getProxies() map[string]driver.ContainerSummary {
	proxies := make(map[string]driver.ContainerSummary)

	fmt.Println("Do get proxies for linux host")
//	filters := map[string][]string{
//		"label": {"skuper.io/application"},
//	}
//	opts := driver.ContainerListOptions{
//		Filters: filters,
//		All:     true,
//	}
//	containers, err := c.vanClient.CeDriver.ContainerList(opts)
//	if err == nil {
//		for _, container := range containers {
//			proxyName := strings.TrimPrefix(container.Names[0], "/")
//			proxies[proxyName] = container
//		}
//	}
	return proxies
}

func (c *Controller) updateBridgeConfig(name string) error {
	desiredBridges := requiredBridges(c.bindings, c.origin)
	fmt.Println("Desired bridges", desiredBridges)
	// read and unmarshal the current config file
//	update, err := desiredBridges.UpdateConfigFile(currentConfig)
//	fmt.Print
	return nil
}

func (c *Controller) processServiceDefs() {
	svcDefs, err := getServiceDefinitions()
	if err != nil {
		log.Println("Failed to retrieve skupper service definitions: ", err.Error())
		return
	}
	c.serviceSyncDefinitionsUpdated(svcDefs)
	if len(svcDefs) > 0 {
		for _, v := range svcDefs {
			c.updateServiceBindings(v)
		}
		for k, _ := range c.bindings {
			_, ok := svcDefs[k]
			if !ok {
				delete(c.bindings, k)
			}
		}
	} else if len(c.bindings) > 0 {
		for k, _ := range c.bindings {
			delete(c.bindings, k)
		}
	}
	//c.updateProxies()
    c.updateBridgeConfig("/var/tmp/skupper/config/qdrouterd.json")
}

func (c *Controller) runServiceDefsWatcher() {
	var watcher *fsnotify.Watcher

	fmt.Println("Inservice defs watcher")
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

//	err := watcher.Add("/etc/messaging/services/skupper-services")	
	err := watcher.Add("/var/tmp/skupper/services/skupper-services")
	if err != nil {
		log.Println("Could not add directory watcher", err.Error())
		return
	}

	c.processServiceDefs()

	for origin, _ := range c.byOrigin {
		if origin != c.origin {
			c.heardFrom[origin] = time.Now()
		}
	}

	fmt.Println("about to enter service defs watch loop")
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				c.processServiceDefs()
			}
		}
	}

}
