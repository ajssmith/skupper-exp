package driver

import (
	"bytes"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
)

const (
	// defaultTimeout is the default timeout of short running docker operations.
	// Value is slightly offset from 2 minutes to make timeouts due to this
	// constant recognizable.
	DefaultTimeout = 2*time.Minute - 1*time.Second

	// defaultShmSize is the default ShmSize to use (in bytes) if not specified.
	DefaultShmSize = int64(1024 * 1024 * 64)

	// defaultImagePullingProgressReportInterval is the default interval of image pulling progress reporting.
	DefaultImagePullingProgressReportInterval = 10 * time.Second
)

type ContainerStatus int

type Driver interface {
	New() error
	ImageInspect(id string) (*ImageInspect, error)
	ImagesList(options ImageListOptions) ([]ImageSummary, error)
	ImagesPull(refStr string, options ImagePullOptions) ([]string, error)
	ContainerCreate(options ContainerCreateOptions) (ContainerCreateResponse, error)
	ContainerStart(id string) error
	ContainerWait(id string, state string, timeout time.Duration, interval time.Duration) error
	ContainerList(options ContainerListOptions) ([]Container, error)
	ContainerInspect(id string) (*InspectContainerData, error)
	ContainerRestart(id string) error
	ContainerStop(id string) error
	ContainerRemove(id string) error
	ContainerExec(id string, cmd []string) (ExecResult, error)
	NetworkCreate(name string, options NetworkCreateOptions) (NetworkCreateResponse, error)
	NetworkInspect(id string) (NetworkResource, error)
	NetworkRemove(id string) error
	NetworkConnect(id string, container string, aliases []string) error
	NetworkDisconnect(id string, container string, force bool) error
}

func RecreateContainer(name string, dd Driver) error {
	current, err := dd.ContainerInspect(name)
	if err != nil {
		return err
	}

	mounts := []MountPoint{}
	for _, v := range current.Mounts {
		mounts = append(mounts, MountPoint{
			Type:        v.Type,
			Source:      v.Source,
			Destination: v.Destination,
		})
	}

	hostCfg := &ContainerHostConfig{
		Mounts:     mounts,
		Privileged: true,
	}

	containerCfg := &ContainerBaseConfig{
		Hostname:     current.Config.Hostname,
		ExposedPorts: current.Config.ExposedPorts,
		Env:          current.Config.Env,
		HealthCheck:  current.Config.Healthcheck,
		Image:        current.Config.Image,
		Labels:       current.Config.Labels,
	}

	networkCfg := &ContainerNetworkingConfig{
		EndpointsConfig: current.NetworkSettings.Networks,
	}

	// remote current and create new container
	err = dd.ContainerStop(name)
	if err != nil {
		return fmt.Errorf("Failed to stop container: %w", err)
	}

	err = dd.ContainerRemove(name)
	if err != nil {
		return fmt.Errorf("Failed to remove container: %w", err)
	}

	opts := ContainerCreateOptions{
		Name:             name,
		ContainerConfig:  containerCfg,
		HostConfig:       hostCfg,
		NetworkingConfig: networkCfg,
	}

	resp, err := dd.ContainerCreate(opts)
	if err != nil {
		return fmt.Errorf("Failed to re-create container %w", err)
	}

	err = dd.ContainerStart(resp.ID)
	if err != nil {
		return fmt.Errorf("Failed to re-start container %w", err)
	}
	return nil
}

// TODO: add Config
type ImageInspect struct {
	ID       string   `json:"Id"`
	Created  int64    `json:"Created"`
	RepoTags []string `json:",omitempty"`
	Size     int64    `json:"Size"`
}

type ImagePullOptions struct {
	Filters map[string][]string
	All     bool
}

type ImageListOptions struct {
	Filters map[string][]string
	All     bool
}

type ContainerListOptions struct {
	Filters map[string][]string
	All     bool
}

type NetworkListOptions struct {
	Filters map[string][]string
	All     bool
}

// TODO: podman Image has container config, where should this come from
type ImageSummary struct {
	ID          string            `json:"Id"`
	Created     int64             `json:"Created"`
	Labels      map[string]string `json:"Labels"`
	RepoTags    []string          `json:",omitempty"`
	RepoDigests []string          `json:",omitempty"`
	Size        int64             `json:"Size"`
}

type HealthConfig struct {
	Test        []string
	Interval    time.Duration
	Timeout     time.Duration
	StartPeriod time.Duration
	Retries     int
}

type ContainerBaseConfig struct {
	Hostname     string
	Image        string
	Env          map[string]string
	Cmd          []string
	HealthCheck  *HealthConfig
	Labels       map[string]string
	ExposedPorts nat.PortSet
}

type ContainerHostConfig struct {
	NetworkMode  string
	PortBindings nat.PortMap
	Mounts       []MountPoint
	Privileged   bool
}

type ContainerNetworkingConfig struct {
	EndpointsConfig map[string]*NetworkEndpointSetting
}

type ContainerCreateOptions struct {
	Name             string
	ContainerConfig  *ContainerBaseConfig
	HostConfig       *ContainerHostConfig
	NetworkingConfig *ContainerNetworkingConfig
}

type ContainerCreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

type Container struct {
	ID      string `json:"Id"`
	Names   []string
	Image   string
	ImageID string
	Command string
	Created int64
	Ports   []Port
	Labels  map[string]string
	State   string
	Status  string
	Mounts  []MountPoint
}

// corresponds to containers on a network
type EndpointResource struct {
	Name       string
	EndpointID string
}
type NetworkResource struct {
	Name       string
	NetworkID  string
	Containers map[string]EndpointResource
}

type NetworkEndpointSetting struct {
	Links       []string
	Aliases     []string
	NetworkID   string
	EndpointID  string
	Gateway     string
	IPAddress   string
	IPPrefixLen int
}

type ContainerNetworkConfig struct {
	Gateway              string   `json:"Gateway"`
	IPAddress            string   `json:"IPAddress"`
	IPPrefixLen          int      `json:"IPPrefixLen"`
	SecondaryIPAddresses []string `json:"SecondaryIPAddresses,omitempty"`
	Networks             map[string]*NetworkEndpointSetting
}

type ContainerConfig struct {
	Hostname     string
	ExposedPorts nat.PortSet
	Env          map[string]string
	Healthcheck  *HealthConfig
	Image        string
	Labels       map[string]string
}

// NOTE: ContainerJSONBase    for docker
//       InspectContainerData for podman
type InspectContainerData struct {
	// Base
	ID              string          `json:"Id"`
	Created         time.Time       `json:"Created"`
	Path            string          `json:"Path"`
	Args            []string        `json:"Args"`
	State           *ContainerState `json:"State"`
	Image           string          `json:"Image"`
	ImageName       string          `json:"ImageName"`
	Name            string          `json:"Name"`
	Mounts          []MountPoint
	Config          ContainerConfig        `json:"Config"`
	NetworkSettings ContainerNetworkConfig `json:"NetworkSettings`
}

type MountPoint struct {
	Type        MountType `json:",omitempty"`
	Name        string    `json:",omitempty"`
	Source      string
	Destination string
	Driver      string `json:"omitempty"`
	Mode        string
	RW          bool
	Propagation MountPropagation
}

type MountType string

const (
	TypeBind      MountType = "bind"
	TypeVolume    MountType = "volume"
	TypeTmpfs     MountType = "tmpsfs"
	TypeNamedPipe MountType = "npipe"
)

type MountPropagation string

// TODO all propagation consts
const (
	PropagationRPrivate MountPropagation = "rprivate"
	PropagationPrivate  MountPropagation = "private"
	PropagationRShared  MountPropagation = "rshared"
	PropagationShared   MountPropagation = "shared"
)

type Port struct {
	IP            string `json: "IP,omitempty"`
	ContainerPort uint16 `json:"ContainerPort"`
	HostPort      uint16 `json:"HostPort,omitempty"`
	Type          string `json:"Type"`
}

type ContainerState struct {
	Status  string
	Running bool
	Paused  bool
}

type NetworkCreateOptions struct {
	CheckDuplicate bool
	Driver         string
	Options        map[string]string
	Labels         map[string]string
}

type NetworkCreateResponse struct {
	ID      string `json:"Id"`
	Warning string
}

type ExecResult struct {
	ExitCode  int
	OutBuffer *bytes.Buffer
	ErrBuffer *bytes.Buffer
}

func (res *ExecResult) Stderr() string {
	return res.ErrBuffer.String()
}

func (res *ExecResult) Stdout() string {
	return res.OutBuffer.String()
}
