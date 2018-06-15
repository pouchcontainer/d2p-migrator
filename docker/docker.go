package docker

import (
	"time"

	dockerCli "github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/filters"
	"golang.org/x/net/context"
)

// Dockerd is a wrapper client for docker daemon.
type Dockerd struct {
	client *dockerCli.Client
}

// NewDockerd creates a docker client.
func NewDockerd() (*Dockerd, error) {
	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := dockerCli.NewClient("unix:///var/run/docker.sock", "v1.24", nil, defaultHeaders)
	if err != nil {
		return nil, err
	}

	return &Dockerd{client: cli}, nil
}

// ContainerList lists all containers on host.
func (d *Dockerd) ContainerList() ([]types.Container, error) {
	options := types.ContainerListOptions{All: true}
	return d.client.ContainerList(context.Background(), options)
}

// ContainerInspect returns the container information.
func (d *Dockerd) ContainerInspect(containerID string) (types.ContainerJSON, error) {
	return d.client.ContainerInspect(context.Background(), containerID)
}

// ImageInspect returns the image information.
func (d *Dockerd) ImageInspect(imageID string) (types.ImageInspect, error) {
	imageInspect, _, err := d.client.ImageInspectWithRaw(context.Background(), imageID, false)
	return imageInspect, err
}

// Info returns dockerd information.
func (d *Dockerd) Info() (types.Info, error) {
	return d.client.Info(context.Background())
}

// VolumeList lists all volumes on host
func (d *Dockerd) VolumeList() (types.VolumesListResponse, error) {
	return d.client.VolumeList(context.Background(), filters.NewArgs())
}

// ContainerStop stops a container.
func (d *Dockerd) ContainerStop(containerID string, timeout *time.Duration) error {
	return d.client.ContainerStop(context.Background(), containerID, timeout)
}

// ContainerStart starts a container.
func (d *Dockerd) ContainerStart(containerID string) error {
	opts := types.ContainerStartOptions{}

	return d.client.ContainerStart(context.Background(), containerID, opts)

}
