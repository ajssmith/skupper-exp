package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ajssmith/skupper-exp/api/types"
	"github.com/ajssmith/skupper-exp/container"
	"github.com/ajssmith/skupper-exp/host"
	skupperutils "github.com/skupperproject/skupper/pkg/utils"	

//	"github.com/ajssmith/skupper-exp/client"
)

func describe(i interface{}) {
	fmt.Printf("(%v, %T)\n", i, i)
	fmt.Println()
}

var onlyOneSignalHandler = make(chan struct{})
var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

func SetupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}

func getTlsConfig(verify bool, cert, key, ca string) (*tls.Config, error) {
	var config tls.Config
	config.InsecureSkipVerify = true
	if verify {
		certPool := x509.NewCertPool()
		file, err := ioutil.ReadFile(ca)
		if err != nil {
			return nil, err
		}
		certPool.AppendCertsFromPEM(file)
		config.RootCAs = certPool
		config.InsecureSkipVerify = false
	}

	_, errCert := os.Stat(cert)
	_, errKey := os.Stat(key)
	if errCert == nil || errKey == nil {
		tlsCert, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			log.Fatal("Could not load x509 key pair", err.Error())
		}
		config.Certificates = []tls.Certificate{tlsCert}
	}
	config.MinVersion = tls.VersionTLS10

	return &config, nil
}

func WaitForServiceStatus(name string, status string, timeout time.Duration, interval time.Duration) error {
	var err error

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	cmd := exec.Command("systemctl", "check", name)

	err = skupperutils.RetryWithContext(ctx, interval, func() (bool, error) {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, nil
		}
		current := strings.TrimSuffix(string(out), "\n")
		return current == status, nil
	})
	return err
}

var mode string
var ce string
var cli types.VanClientInterface

func main() {
    mode = "host"

	siteId := os.Getenv("SKUPPER_SITE_ID")
	if os.Getenv("SKUPPER_CONTAINER_ENGINE") != "" {
		ce = os.Getenv("SKUPPER_CONTAINER_ENGINE")
		fmt.Printf("Container engine is: ", ce)
	} else {
		ce = "docker"
	}

	if mode == "container-engine" {
		cli = &container.ContainerClient
	} else if mode == "host" {
		cli = &host.HostClient
	} else {
		fmt.Printf("Mode %s note recognized, must be one of host or container-engine \n", mode)
	}	

	stopCh := SetupSignalHandler()

//    tlsConfig, err := getTlsConfig(true, types.ControllerConfigPath+"tls.crt", types.ControllerConfigPath+"tls.key", types.ControllerConfigPath+"ca.crt")
	tlsConfig, err := getTlsConfig(true, "/var/tmp/skupper/qpid-dispatch-certs/skupper-amqps/tls.crt", "/var/tmp/skupper/qpid-dispatch-certs/skupper-amqps/tls.key", "/var/tmp/skupper/qpid-dispatch-certs/skupper-amqps/ca.crt")
	if err != nil {
		log.Fatal("Error getting tls config: ", err.Error())
	}

	controller, err := NewController(siteId, tlsConfig)
	if err != nil {
		log.Fatal("Error getting new controller: ", err.Error())
	}

	log.Println("Waiting for the Skupper router component to start")
	err = WaitForServiceStatus("qdrouterd", "active", time.Second*180, time.Second*5)	
	//	_, err = docker.WaitForContainerStatus("skupper-router", "running", time.Second*180, time.Second*5, cli.DockerInterface)
	if err != nil {
			log.Fatal("Failed waiting for router to be running", err.Error())
	}

	// start the controller workers
	if err = controller.Run(stopCh); err != nil {
		log.Fatal("Error running controller: ", err.Error())
	}

}
