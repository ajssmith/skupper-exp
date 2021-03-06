package docker

import (
	"fmt"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/ajssmith/skupper-exp/pkg/docker/libdocker"
)

func InspectNetwork(name string, dd libdocker.Interface) (dockertypes.NetworkResource, error) {
	return dd.InspectNetwork(name)
}

func RemoveNetwork(name string, dd libdocker.Interface) error {
	tnr, err := dd.InspectNetwork(name)
	if err != nil {
		return err
	}
	for _, container := range tnr.Containers {
		err := dd.DisconnectContainerFromNetwork(name, container.Name, true)
		if err != nil {
			return err
		}
	}
	return dd.RemoveNetwork(name)
}

func NewTransportNetwork(name string, dd libdocker.Interface) (dockertypes.NetworkCreateResponse, error) {
	nw := dockertypes.NetworkCreateResponse{}

	if name != "" {
		nw, err := dd.CreateNetwork(name)
		return nw, err
	} else {
		return nw, fmt.Errorf("Unable to create network")
	}
}
