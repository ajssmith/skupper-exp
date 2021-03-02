package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/containers/podman/v2/libpod/define"
	"github.com/containers/podman/v2/pkg/api/handlers"
	"github.com/containers/podman/v2/pkg/bindings"
	"github.com/containers/podman/v2/pkg/bindings/containers"
	"github.com/containers/podman/v2/pkg/bindings/images"
	"github.com/containers/podman/v2/pkg/bindings/network"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/specgen"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	//	"github.com/ajssmith/skupper-exp/driver"
)

type podmanClient struct {
	ctx                      context.Context
	timeout                  time.Duration
	imagePullProgessDeadline time.Duration
}

var PodmanDriver podmanClient

func newPodmanContainerSpec(options ContainerCreateOptions) *specgen.SpecGenerator {
	//	sg := specgen.NewSpecGenerator("", true)
	sg := specgen.NewSpecGenerator(options.ContainerConfig.Image, false)

	var mounts []spec.Mount
	for _, mount := range options.HostConfig.Mounts {
		mounts = append(mounts, spec.Mount{
			Type:        "bind",
			Destination: mount.Destination,
			Source:      mount.Source,
		})
	}

	// Basic
	sg.ContainerBasicConfig.Name = options.Name
	sg.ContainerBasicConfig.Command = options.ContainerConfig.Cmd
	sg.ContainerBasicConfig.Env = options.ContainerConfig.Env

	// storage
	sg.ContainerStorageConfig.Mounts = mounts
	sg.ContainerStorageConfig.Image = options.ContainerConfig.Image

	// Security
	sg.ContainerSecurityConfig.Privileged = true

	// Neworking
	cniNetworks := make([]string, 0, len(options.NetworkingConfig.EndpointsConfig))
	for netName, _ := range options.NetworkingConfig.EndpointsConfig {
		cniNetworks = append(cniNetworks, netName)
	}
	//var networks []string
	//networks = append(networks, "skupper-network")
	//	sg.ContainerNetworkConfig.NetNS.NSMode = "Bridge"

	sg.ContainerNetworkConfig.CNINetworks = cniNetworks
	return sg
}

func (c *podmanClient) New() error {
	fmt.Println("Inside podman plugin new")

	//sock_dir := os.Getenv("XDG_RUNTIME_DIR")
	//	socket := "unix:" + sock_dir + "/podman/podman.sock"
	//	socket := "unix:/run/user/1000/podman/podman.sock"
	socket := "unix:/var/run/podman/podman.sock"

	fmt.Println("Let's connect to podman")
	ctx, err := bindings.NewConnection(context.Background(), socket)
	if err != nil {
		return fmt.Errorf("Coudnt's connect to podman: %w", err)
	}
	PodmanDriver.ctx = ctx
	PodmanDriver.timeout = DefaultTimeout
	PodmanDriver.imagePullProgessDeadline = DefaultImagePullingProgressReportInterval

	return nil
}

func (c *podmanClient) ImageInspect(id string) (*ImageInspect, error) {
	fmt.Println("In podman inspect image")

	data, err := images.GetImage(c.ctx, id, nil)
	if err != nil {
		return &ImageInspect{}, err
	}
	image := &ImageInspect{
		ID:       data.ID,
		Size:     data.Size,
		RepoTags: data.RepoTags,
	}
	return image, nil
}

func (c *podmanClient) ImagesPull(refStr string, options ImagePullOptions) ([]string, error) {
	fmt.Println("In podman pull images")
	strSlice, err := images.Pull(c.ctx, refStr, entities.ImagePullOptions{})
	if err != nil {
		return nil, fmt.Errorf("Could not pull image: %w", err)
	}
	return strSlice, nil
}

func (c *podmanClient) ImagesList(options ImageListOptions) ([]ImageSummary, error) {
	fmt.Println("In podman list images")

	images, err := images.List(c.ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	var summary []ImageSummary
	for _, image := range images {
		summary = append(summary, ImageSummary{
			ID:       image.ID,
			Created:  image.Created,
			Labels:   image.Labels,
			RepoTags: image.RepoTags,
			Size:     image.Size,
		})
	}
	return summary, nil
}

func (c *podmanClient) ImageVersion(id string) (string, error) {
	return "no-image", nil
}

func (c *podmanClient) ContainerCreate(options ContainerCreateOptions) (ContainerCreateResponse, error) {
	fmt.Println("Inside podman container create")
	//	s := specgen.NewSpecGenerator(image, false)
	spec := newPodmanContainerSpec(options)
	r, err := containers.CreateWithSpec(c.ctx, spec)
	if err != nil {
		return ContainerCreateResponse{}, err
	}

	return ContainerCreateResponse{ID: r.ID}, nil
}

func (c *podmanClient) ContainerStart(id string) error {
	fmt.Println("Inside podman start container")
	err := containers.Start(c.ctx, id, nil)
	return err
}

func (c *podmanClient) ContainerWait(id string, status string, timeout time.Duration, interval time.Duration) error {
	fmt.Println("Inside podman container wait")
	// TODO: Should we have retry with context here?
	waitState := define.ContainerStateRunning
	_, err := containers.Wait(c.ctx, id, &waitState)
	return err
}

func (c *podmanClient) ContainerList(ContainerListOptions) ([]ContainerSummary, error) {
	fmt.Println("Inside podman container list")
	// TODO convert options
	var latestContainers = 1
	cl, err := containers.List(c.ctx, nil, nil, &latestContainers, nil, nil, nil)
	var dc []ContainerSummary
	for _, container := range cl {
		// TODO all fields
		//fmt.Println("Container list name, id", container.Names[0], container.ID)
		if container.Names[0] != "skupper-service-controller" {
			dc = append(dc, ContainerSummary{
				ID:      container.ID,
				Names:   container.Names,
				Image:   container.Image,
				ImageID: container.ImageID,
				//Command: container.Command,
				//Ports:   container.Ports,
				Labels: container.Labels,
				State:  container.State,
				Status: container.Status,
				//Mounts:  container.Mounts,
			})
		}
	}
	return dc, err
}

func (c *podmanClient) ContainerInspect(id string) (*ContainerInspect, error) {
	fmt.Println("Inside podman container inspect")
	container, err := containers.Inspect(c.ctx, id, nil)
	if err != nil {
		return &ContainerInspect{}, err
	}
	icd := &ContainerInspect{
		ID:      container.ID,
		Created: container.Created,
		Path:    container.Path,
		Args:    container.Args,
		//		State: cd.State,
		Image:     container.Image,
		ImageName: container.ImageName,
		Name:      container.Name,
		//		Mounts: cd.Mounts,
		NetworkSettings: ContainerNetworkConfig{
			Gateway:              container.NetworkSettings.Gateway,
			IPAddress:            container.NetworkSettings.IPAddress,
			IPPrefixLen:          container.NetworkSettings.IPPrefixLen,
			SecondaryIPAddresses: container.NetworkSettings.SecondaryIPAddresses,
		},
	}
	if len(container.NetworkSettings.Networks) > 0 {
		icd.NetworkSettings.Networks = make(map[string]*NetworkEndpointSetting)
		for net, setting := range container.NetworkSettings.Networks {
			endpoint := new(NetworkEndpointSetting)
			endpoint.NetworkID = net
			endpoint.Gateway = setting.Gateway
			endpoint.IPAddress = setting.IPAddress
			endpoint.IPPrefixLen = setting.IPPrefixLen
			icd.NetworkSettings.Networks[net] = endpoint
		}
	}
	return icd, err
}

func (c *podmanClient) ContainerRestart(id string) error {
	fmt.Println("Inside podman restart container")
	err := containers.Restart(c.ctx, id, nil)
	return err
}

func (c *podmanClient) ContainerStop(id string) error {
	fmt.Println("Inside podman stop container")
	err := containers.Stop(c.ctx, id, nil)
	return err
}

func (c *podmanClient) ContainerRemove(id string) error {
	force := true
	fmt.Println("Inside podman container remove")
	return containers.Remove(c.ctx, id, &force, &force)
}

func (c *podmanClient) NetworkCreate(name string, options NetworkCreateOptions) (NetworkCreateResponse, error) {
	fmt.Println("Inside podman network create")
	nco := entities.NetworkCreateOptions{
		Driver: options.Driver,
		//		Options: options.Options,
		//		Labels:  options.Labels,
	}
	resp, err := network.Create(c.ctx, nco, &name)
	if err != nil {
		return NetworkCreateResponse{}, err
	}
	fmt.Printf("Network create response %+v\n", resp)
	return NetworkCreateResponse{}, err
}

func (c *podmanClient) NetworkInspect(id string) (NetworkInspect, error) {
	fmt.Println("Inside podman network inspect")
	// nir is map[string]interface
	nir, err := network.Inspect(c.ctx, id)
	//	fmt.Println("nir name: ", nir[0]["name"])
	name := fmt.Sprintf("%v", nir[0]["name"])
	//	fmt.Println("nir cniversion: ", nir[0]["cniVersion"])
	//	fmt.Println("nir plugins: ", nir[0]["plugins"])

	//	fmt.Println("NIR is:", nir)
	//	var dnr driver.NetworkResource
	//	if _, ok := nir["name"]; ok {
	//		dnr.Name = nir["name"]
	//	}
	return NetworkInspect{Name: name}, err
}

func (c *podmanClient) NetworkRemove(id string) error {
	force := true
	fmt.Println("Inside podman network remove for: ", id)
	_, err := network.Remove(c.ctx, id, &force)
	return err
}

func (c *podmanClient) NetworkConnect(id string, container string, aliases []string) error {
	fmt.Println("Inside podman network connect: ", id, container)
	err := network.Connect(c.ctx, id, entities.NetworkConnectOptions{
		Container: container,
		Aliases:   aliases,
	})
	return err
}

func (c *podmanClient) NetworkDisconnect(id string, container string, force bool) error {
	fmt.Println("Inside podman network disconnect: ", id, container)
	err := network.Disconnect(c.ctx, id, entities.NetworkDisconnectOptions{
		Container: container,
		Force:     force,
	})
	return err
}

type PmWriteCloser struct {
	*bufio.Writer
}

func (pwc *PmWriteCloser) Close() error {
	return nil
}

func (c *podmanClient) ContainerExecKeeper(id string, cmd []string) (ExecResult, error) {
	fmt.Println("Inside docker container exec")

	//TODO: there may be a better way to capture, stderr too?
	stdout := os.Stdout
	r, w, err := os.Pipe()
	os.Stdout = w

	execConfig := new(handlers.ExecCreateConfig)
	execConfig.AttachStdout = true
	execConfig.AttachStderr = true
	execConfig.Cmd = cmd

	execID, err := containers.ExecCreate(c.ctx, id, execConfig)
	if err != nil {
		return ExecResult{}, err
	}

	streams := new(define.AttachStreams)
	streams.OutputStream = os.Stdout
	streams.ErrorStream = os.Stderr
	streams.AttachOutput = true
	streams.AttachError = true

	err = containers.ExecStartAndAttach(c.ctx, execID, streams)
	if err != nil {
		return ExecResult{}, err
	}

	//TODO: channel behaviors
	var outBuf bytes.Buffer
	copyDone := make(chan struct{})

	go func() {
		_, err = io.Copy(&outBuf, r)
		r.Close()
		close(copyDone)
	}()

	defer func() {
		w.Close()
		os.Stdout = stdout
		<-copyDone
	}()

	inspectOut, err := containers.ExecInspect(c.ctx, execID)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{ExitCode: inspectOut.ExitCode, OutBuffer: &outBuf, ErrBuffer: nil}, nil
}

func (c *podmanClient) ContainerExec(id string, cmd []string) (ExecResult, error) {
	fmt.Println("Inside docker container exec")

	//TODO: there may be a better way to capture, stderr too?
	stdout := os.Stdout
	r, w, err := os.Pipe()
	os.Stdout = w

	execConfig := new(handlers.ExecCreateConfig)
	execConfig.AttachStdout = true
	execConfig.AttachStderr = true
	execConfig.Cmd = cmd

	execID, err := containers.ExecCreate(c.ctx, id, execConfig)
	if err != nil {
		return ExecResult{}, err
	}

	streams := new(define.AttachStreams)
	streams.OutputStream = os.Stdout
	streams.ErrorStream = os.Stderr
	streams.AttachOutput = true
	streams.AttachError = true

	err = containers.ExecStartAndAttach(c.ctx, execID, streams)
	if err != nil {
		return ExecResult{}, err
	}

	var outBuf, errBuf bytes.Buffer
	copyDone := make(chan struct{})

	go func() {
		_, err = io.Copy(&outBuf, r)
		r.Close()
		copyDone <- struct{}{}
	}()

	defer func() {
		w.Close()
		os.Stdout = stdout
		<-copyDone
	}()

	inspectOut, err := containers.ExecInspect(c.ctx, execID)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{ExitCode: inspectOut.ExitCode, OutBuffer: &outBuf, ErrBuffer: &errBuf}, nil
}

func (c *podmanClient) Info() (Info, error) {
	return Info{}, nil
}
