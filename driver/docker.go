package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	//	"strings"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerfilters "github.com/docker/docker/api/types/filters"
	dockermounttypes "github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	dockernetworktypes "github.com/docker/docker/api/types/network"
	dockerapi "github.com/docker/docker/client"
	dockermessage "github.com/docker/docker/pkg/jsonmessage"
	dockerstdcopy "github.com/docker/docker/pkg/stdcopy"

	//	"github.com/ajssmith/skupper-exp/driver"
	skupperutils "github.com/skupperproject/skupper/pkg/utils"
)

const (
	// defaultTimeout is the default timeout of short running docker operations.
	// Value is slightly offset from 2 minutes to make timeouts due to this
	// constant recognizable.
	defaultTimeout = 2*time.Minute - 1*time.Second

	// defaultShmSize is the default ShmSize to use (in bytes) if not specified.
	defaultShmSize = int64(1024 * 1024 * 64)

	// defaultImagePullingProgressReportInterval is the default interval of image pulling progress reporting.
	defaultImagePullingProgressReportInterval = 10 * time.Second
)

type dockerClient struct {
	client                   *dockerapi.Client
	timeout                  time.Duration
	imagePullProgessDeadline time.Duration
}

type ImageNotFoundError struct {
	ID string
}

var DockerDriver dockerClient

func getTimeoutContext(d *dockerClient) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d.timeout)
}

func newDockerContainerSpec(options ContainerCreateOptions) *dockertypes.ContainerCreateConfig {

	var mounts []dockermounttypes.Mount
	for _, mount := range options.HostConfig.Mounts {
		mounts = append(mounts, dockermounttypes.Mount{
			Type:   dockermounttypes.TypeBind,
			Source: mount.Source,
			Target: mount.Destination,
		})
	}

	endpoints := make(map[string]*dockernetworktypes.EndpointSettings)
	for i := range options.NetworkingConfig.EndpointsConfig {
		endpoints[i] = &network.EndpointSettings{}
	}

	envVars := []string{}
	for key, value := range options.ContainerConfig.Env {
		envVars = append(envVars, key+"="+value)
	}
	opts := &dockertypes.ContainerCreateConfig{
		Name: options.Name,
		Config: &dockercontainer.Config{
			Hostname: options.ContainerConfig.Hostname,
			Image:    options.ContainerConfig.Image,
			Env:      envVars,
			Cmd:      options.ContainerConfig.Cmd,
			Healthcheck: &dockercontainer.HealthConfig{
				Test:        options.ContainerConfig.HealthCheck.Test,
				StartPeriod: options.ContainerConfig.HealthCheck.StartPeriod,
			},
			Labels:       options.ContainerConfig.Labels,
			ExposedPorts: options.ContainerConfig.ExposedPorts,
		},
		HostConfig: &dockercontainer.HostConfig{
			Mounts:     mounts,
			Privileged: options.HostConfig.Privileged,
		},
		NetworkingConfig: &dockernetworktypes.NetworkingConfig{
			EndpointsConfig: endpoints,
		},
	}
	return opts
}

func (c *dockerClient) New() error {
	fmt.Println("Inside docker plugin new")
	client, err := dockerapi.NewClientWithOpts(dockerapi.FromEnv, dockerapi.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("Couldn't connect to docker: %w", err)
	}

	DockerDriver.client = client
	DockerDriver.timeout = DefaultTimeout
	DockerDriver.imagePullProgessDeadline = DefaultImagePullingProgressReportInterval

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()
	DockerDriver.client.NegotiateAPIVersion(ctx)

	return nil
}

func getCancelableContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func contextError(ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return operationTimeout{err: ctx.Err()}
	}
	return ctx.Err()
}

type operationTimeout struct {
	err error
}

func (e operationTimeout) Error() string {
	return fmt.Sprintf("operation timeout: %v", e.err)
}

func base64EncodeAuth(auth dockertypes.AuthConfig) (string, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(auth); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf.Bytes()), nil
}

// progress is a wrapper of dockermessage.JSONMessage with a lock protecting it.
type progress struct {
	sync.RWMutex
	// message stores the latest docker json message.
	message *dockermessage.JSONMessage
	// timestamp of the latest update.
	timestamp time.Time
}

func newProgress() *progress {
	return &progress{timestamp: time.Now()}
}

func (p *progress) set(msg *dockermessage.JSONMessage) {
	p.Lock()
	defer p.Unlock()
	p.message = msg
	p.timestamp = time.Now()
}

func (p *progress) get() (string, time.Time) {
	p.RLock()
	defer p.RUnlock()
	if p.message == nil {
		return "No progress", p.timestamp
	}
	// The following code is based on JSONMessage.Display
	var prefix string
	if p.message.ID != "" {
		prefix = fmt.Sprintf("%s: ", p.message.ID)
	}
	if p.message.Progress == nil {
		return fmt.Sprintf("%s%s", prefix, p.message.Status), p.timestamp
	}
	return fmt.Sprintf("%s%s %s", prefix, p.message.Status, p.message.Progress.String()), p.timestamp
}

type progressReporter struct {
	*progress
	image                     string
	cancel                    context.CancelFunc
	stopCh                    chan struct{}
	imagePullProgressDeadline time.Duration
}

func newProgressReporter(image string, cancel context.CancelFunc, imagePullProgressDeadline time.Duration) *progressReporter {
	return &progressReporter{
		progress:                  newProgress(),
		image:                     image,
		cancel:                    cancel,
		stopCh:                    make(chan struct{}),
		imagePullProgressDeadline: imagePullProgressDeadline,
	}
}

func (p *progressReporter) start() {
	go func() {
		ticker := time.NewTicker(defaultImagePullingProgressReportInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, timestamp := p.progress.get()
				// If there is no progress for p.imagePullProgressDeadline, cancel the operation.
				if time.Since(timestamp) > p.imagePullProgressDeadline {
					//log.Printf("Cancel pulling image %q because of no progress for %v, latest progress: %q", p.image, p.imagePullProgressDeadline, progress)
					//log.Println()
					p.cancel()
					return
				}
				//log.Printf("Pulling image %q: %q", p.image, progress)
				//log.Println()
			case <-p.stopCh:
				//progress, _ := p.progress.get()
				//log.Printf("Stop pulling image %q: %q", p.image, progress)
				//log.Println()
				return
			}
		}
	}()
}

func (p *progressReporter) stop() {
	close(p.stopCh)
}

func (c *dockerClient) ImagesPull(refStr string, options ImagePullOptions) ([]string, error) {
	// TODO: return common []string
	fmt.Println("In docker pull images")
	// RegistryAuth is the base64 encoded credentials for the registry
	auth := dockertypes.AuthConfig{}
	base64Auth, err := base64EncodeAuth(auth)
	if err != nil {
		return nil, err
	}
	opts := dockertypes.ImagePullOptions{}
	opts.RegistryAuth = base64Auth

	ctx, cancel := getCancelableContext()
	defer cancel()
	resp, err := c.client.ImagePull(ctx, refStr, opts)
	if err != nil {
		return nil, err
	}
	defer resp.Close()
	reporter := newProgressReporter(refStr, cancel, 10*time.Second)
	reporter.start()
	defer reporter.stop()
	decoder := json.NewDecoder(resp)
	for {
		var msg dockermessage.JSONMessage
		err := decoder.Decode(&msg)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if msg.Error != nil {
			return nil, msg.Error
		}
		reporter.set(&msg)
	}
	return nil, nil
}

func (c *dockerClient) ImageInspect(id string) (*ImageInspect, error) {
	fmt.Println("In docker inspect image")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	data, _, err := c.client.ImageInspectWithRaw(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}

	image := &ImageInspect{
		ID:       data.ID,
		Size:     data.Size,
		RepoTags: data.RepoTags,
	}
	return image, nil
}

func (c *dockerClient) ImagesList(options ImageListOptions) ([]ImageSummary, error) {
	fmt.Println("In docker list images")
	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	images, err := c.client.ImageList(ctx, dockertypes.ImageListOptions{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	var summary []ImageSummary
	for _, image := range images {
		summary = append(summary, ImageSummary{
			ID:          image.ID,
			Labels:      image.Labels,
			RepoTags:    image.RepoTags,
			RepoDigests: image.RepoDigests,
		})
	}
	return summary, nil
}

func (c *dockerClient) ImageVersion(id string) (string, error) {
	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	iibd, _, err := c.client.ImageInspectWithRaw(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return "", ctxErr
	}
	if err != nil {
		//		if dockerapi.IsErrNotFound(err) {
		//			err = ImageNotFoundError{ID: ref}
		//		}
		return "", err
	}

	digest := iibd.RepoDigests[0]
	parts := strings.Split(digest, "@")
	if len(parts) > 1 && len(parts[1]) >= 19 {
		return fmt.Sprintf("%s (%s)", id, parts[1][:19]), nil
	} else {
		return fmt.Sprintf("%s", digest), nil
	}
}

func (c *dockerClient) ContainerCreate(options ContainerCreateOptions) (ContainerCreateResponse, error) {
	fmt.Println("Inside docker container create")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	opts := newDockerContainerSpec(options)

	ccb, err := c.client.ContainerCreate(ctx, opts.Config, opts.HostConfig, opts.NetworkingConfig, nil, opts.Name)
	if err != nil {
		return ContainerCreateResponse{}, err
	}
	return ContainerCreateResponse{ID: ccb.ID}, nil
}

func (c *dockerClient) ContainerStart(id string) error {
	fmt.Println("Inside docker start container")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	err := c.client.ContainerStart(ctx, id, dockertypes.ContainerStartOptions{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}

	return err
}

func (c *dockerClient) ContainerWait(id string, status string, timeout time.Duration, interval time.Duration) error {
	fmt.Println("Inside docker container wait")
	var container dockertypes.ContainerJSON
	var err error

	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	err = skupperutils.RetryWithContext(ctx, interval, func() (bool, error) {
		container, err = c.client.ContainerInspect(ctx, id)
		if err != nil {
			return false, nil
		}
		return container.State.Status == status, nil
	})
	return err
}

func (c *dockerClient) ContainerList(opts ContainerListOptions) ([]ContainerSummary, error) {
	fmt.Println("Inside docker container list")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	//TODO is this correct conversion of map[string][]string to filters
	filters := dockerfilters.NewArgs()
	for i, j := range opts.Filters {
		filters.Add(i, j[0])
		//filters.Add(i, strings.Join(j,","))
	}

	containers, err := c.client.ContainerList(ctx, dockertypes.ContainerListOptions{
		Filters: filters,
		All:     opts.All,
	})
	var dc []ContainerSummary
	if ctxErr := contextError(ctx); ctxErr != nil {
		return dc, ctxErr
	}
	if err != nil {
		return dc, err
	}
	for _, container := range containers {
		// TODO all fields
		dc = append(dc, ContainerSummary{
			ID:      container.ID,
			Names:   container.Names,
			Image:   container.Image,
			ImageID: container.ImageID,
			Command: container.Command,
			//Ports:   container.Ports,
			Labels: container.Labels,
			State:  container.State,
			Status: container.Status,
			//Mounts:  container.Mounts,
		})
	}
	return dc, nil
}

func (c *dockerClient) ContainerInspect(id string) (*ContainerInspect, error) {
	fmt.Println("Inside docker container inspect")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	container, err := c.client.ContainerInspect(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	mounts := []MountPoint{}
	for _, mount := range container.Mounts {
		mounts = append(mounts, MountPoint{
			Type:        TypeBind,
			Name:        mount.Name,
			Source:      mount.Source,
			Destination: mount.Destination,
			Driver:      mount.Driver,
			Mode:        mount.Mode,
			RW:          mount.RW,
		})
	}
	envVars := make(map[string]string)
	for _, env := range container.Config.Env {
		pos := strings.Index(env, "=")
		name := env[:pos]
		value := env[pos+1:]
		if value != "" {
			envVars[name] = value
		}
	}
	icd := &ContainerInspect{
		ID: container.ID,
		//Created: container.Created,
		Path: container.Path,
		Args: container.Args,
		State: &ContainerState{
			Status: container.State.Status,
		},
		Image: container.Image,
		//ImageName: container.ImageName,
		Name:   container.Name,
		Mounts: mounts,
		Config: ContainerConfig{
			Hostname:     container.Config.Hostname,
			ExposedPorts: container.Config.ExposedPorts,
			Env:          envVars,
			Healthcheck: &HealthConfig{
				Test:        container.Config.Healthcheck.Test,
				Interval:    container.Config.Healthcheck.Interval,
				Timeout:     container.Config.Healthcheck.Timeout,
				StartPeriod: container.Config.Healthcheck.StartPeriod,
				Retries:     container.Config.Healthcheck.Retries,
			},
			Image:  container.Config.Image,
			Labels: container.Config.Labels,
		},
		NetworkSettings: ContainerNetworkConfig{
			Gateway:     container.NetworkSettings.DefaultNetworkSettings.Gateway,
			IPAddress:   container.NetworkSettings.DefaultNetworkSettings.IPAddress,
			IPPrefixLen: container.NetworkSettings.DefaultNetworkSettings.IPPrefixLen,
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

func (c *dockerClient) ContainerRestart(id string) error {
	fmt.Println("Inside docker restart container")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	timeout := 10 * time.Second
	err := c.client.ContainerRestart(ctx, id, &timeout)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (c *dockerClient) ContainerStop(id string) error {
	fmt.Println("Inside docker stop container")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	err := c.client.ContainerStop(ctx, id, nil)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (c *dockerClient) ContainerRemove(id string) error {
	fmt.Println("Inside docker container remove")
	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	err := c.client.ContainerRemove(ctx, id, dockertypes.ContainerRemoveOptions{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (c *dockerClient) NetworkCreate(name string, options NetworkCreateOptions) (NetworkCreateResponse, error) {
	fmt.Println("Inside docker network create")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	ncr, err := c.client.NetworkCreate(ctx, name, dockertypes.NetworkCreate{
		CheckDuplicate: true,
		Driver:         "bridge",
		Options: map[string]string{
			"com.docker.network.bridge.name":                 "skupper0",
			"com.docker.network.bridge.enable_icc":           "true",
			"com.docker.network.bridge.enable_ip_masquerade": "true",
		},
	})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return NetworkCreateResponse{}, ctxErr
	}
	return NetworkCreateResponse{ID: ncr.ID, Warning: ncr.Warning}, err
}

func (c *dockerClient) NetworkInspect(id string) (NetworkInspect, error) {
	var netResource NetworkInspect

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	nr, err := c.client.NetworkInspect(ctx, id, dockertypes.NetworkInspectOptions{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return netResource, ctxErr
	}
	if len(nr.Containers) > 0 {
		netResource.Containers = make(map[string]EndpointResource)
		for container, endPoint := range nr.Containers {
			netResource.Containers[container] = EndpointResource{
				Name:       endPoint.Name,
				EndpointID: endPoint.EndpointID,
			}
		}
	}
	return netResource, err
}

func (c *dockerClient) NetworkRemove(id string) error {
	//	force := true
	fmt.Println("Inside docker network remove for: ", id)
	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	err := c.client.NetworkRemove(ctx, id)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	return err
}

func (c *dockerClient) NetworkConnect(id string, container string, aliases []string) error {
	fmt.Println("Inside docker network connect: ", id, container)

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	err := c.client.NetworkConnect(ctx, id, container, &dockernetworktypes.EndpointSettings{})
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *dockerClient) NetworkDisconnect(id string, container string, force bool) error {
	fmt.Println("Inside docker network disconnect: ", id, container)

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	err := c.client.NetworkDisconnect(ctx, id, container, force)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *dockerClient) ContainerExec(id string, cmd []string) (ExecResult, error) {
	fmt.Println("Inside docker container exec")
	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	execConfig := dockertypes.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	createResponse, err := c.client.ContainerExecCreate(ctx, id, execConfig)
	if err != nil {
		return ExecResult{}, err
	}
	execID := createResponse.ID

	// run with stdout and stderr attached
	attachResponse, err := c.client.ContainerExecAttach(ctx, execID, dockertypes.ExecStartCheck{})
	if err != nil {
		return ExecResult{}, err
	}
	defer attachResponse.Close()

	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error, 1)

	go func() {
		_, err = dockerstdcopy.StdCopy(&outBuf, &errBuf, attachResponse.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return ExecResult{}, err
		}
		break
	}

	inspectResponse, err := c.client.ContainerExecInspect(ctx, execID)
	if err != nil {
		return ExecResult{}, err
	}

	return ExecResult{ExitCode: inspectResponse.ExitCode, OutBuffer: &outBuf, ErrBuffer: &errBuf}, nil
}

func (c *dockerClient) Info() (Info, error) {
	fmt.Println("Inside docker info")

	ctx, cancel := getTimeoutContext(&DockerDriver)
	defer cancel()

	driverInfo := Info{}

	info, err := c.client.Info(ctx)
	if ctxErr := contextError(ctx); ctxErr != nil {
		return driverInfo, ctxErr
	}
	if err != nil {
		return driverInfo, err
	}

	driverInfo.ID = info.ID
	driverInfo.Name = info.Name
	driverInfo.KernelVersion = info.KernelVersion
	driverInfo.OperatingSystem = info.OperatingSystem
	driverInfo.OSVersion = info.OSVersion
	driverInfo.OSType = info.OSType
	driverInfo.Architecture = info.Architecture
	driverInfo.ServerVersion = info.ServerVersion

	return driverInfo, nil
}
