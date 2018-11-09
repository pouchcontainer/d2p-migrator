package ctrd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"syscall"

	"github.com/pouchcontainer/d2p-migrator/ctrd/image"
	"github.com/pouchcontainer/d2p-migrator/utils"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/pkg/reference"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/linux/runctypes"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/image-spec/identity"
)

const (
	defaultSnapshotterName = "overlayfs"
	socketAddr             = "/tmp/containerd-migrator.sock"
	defaultRegistry        = "registry.hub.docker.com"
	defaultNamespace       = "library"
)

// StartContainerd create a new containerd instance.
func StartContainerd(homeDir string, debug bool) (int, error) {
	// if socket file exists, delete it.
	if _, err := os.Stat(socketAddr); err == nil {
		os.RemoveAll(socketAddr)
	}

	containerdPath, err := exec.LookPath("containerd")
	if err != nil {
		return -1, fmt.Errorf("failed to find containerd binary %v", err)
	}

	// Start a new containerd instance.
	cmd, err := newContainerdCmd(homeDir, containerdPath, debug)
	if err != nil {
		return -1, err
	}

	go cmd.Wait()
	return cmd.Process.Pid, nil
}

func newContainerdCmd(homeDir, containerdPath string, debug bool) (*exec.Cmd, error) {
	args := []string{
		"-a", socketAddr,
		"--root", path.Join(homeDir, "containerd/root"),
		"--state", path.Join(homeDir, "containerd/state"),
		"-l", utils.IfThenElse(debug, "debug", "info").(string),
	}

	cmd := exec.Command(containerdPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Env = nil
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "NOTIFY_SOCKET") {
			cmd.Env = append(cmd.Env, e)
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}

// Cleanup kill containerd instance process
func Cleanup(pid int) error {
	if pid != -1 {
		// delete containerd process forcely.
		syscall.Kill(pid, syscall.SIGKILL)
	}

	os.Remove(socketAddr)
	return nil
}

// Client is the wrapper of a containerd client.
type Client struct {
	client         *containerd.Client
	lease          *containerd.Lease
	RePullImageSet map[string]struct{}
}

// NewCtrdClient create a client of containerd
func NewCtrdClient(imageSet map[string]struct{}) (*Client, error) {
	client, err := containerd.New(socketAddr, containerd.WithDefaultNamespace("default"))
	if err != nil {
		return nil, err
	}

	// create a new lease or reuse the existed.
	var lease containerd.Lease
	leases, err := client.ListLeases(context.TODO())
	if err != nil {
		return nil, err
	}
	if len(leases) != 0 {
		lease = leases[0]
	} else {
		if lease, err = client.CreateLease(context.TODO()); err != nil {
			return nil, err
		}
	}

	return &Client{
		client:         client,
		lease:          &lease,
		RePullImageSet: imageSet,
	}, nil
}

// CreateSnapshot creates an active snapshot with image's name and id.
func (cli *Client) CreateSnapshot(ctx context.Context, id, ref string) error {
	ctx = leases.WithLease(ctx, cli.lease.ID())

	image, err := cli.client.ImageService().Get(ctx, ref)
	if err != nil {
		return err
	}

	diffIDs, err := image.RootFS(ctx, cli.client.ContentStore(), platforms.Default())
	if err != nil {
		return err
	}

	parent := identity.ChainID(diffIDs).String()
	if _, err := cli.client.SnapshotService(defaultSnapshotterName).Prepare(ctx, id, parent); err != nil {
		return err
	}

	return nil
}

// GetSnapshot returns the snapshot's info by id.
func (cli *Client) GetSnapshot(ctx context.Context, id string) (snapshots.Info, error) {
	service := cli.client.SnapshotService(defaultSnapshotterName)
	defer service.Close()

	return service.Stat(ctx, id)
}

// GetMounts returns the mounts for the active snapshot transaction identified by key.
func (cli *Client) GetMounts(ctx context.Context, id string) ([]mount.Mount, error) {
	service := cli.client.SnapshotService(defaultSnapshotterName)
	defer service.Close()

	return service.Mounts(ctx, id)
}

// RemoveSnapshot remove the snapshot by id.
func (cli *Client) RemoveSnapshot(ctx context.Context, id string) error {
	service := cli.client.SnapshotService(defaultSnapshotterName)
	defer service.Close()

	return service.Remove(ctx, id)

}

// NewContainer just create a very simple container for migration
// just load container id to boltdb
func (cli *Client) NewContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, namespaces.Default)

	spec, err := oci.GenerateSpec(ctx, nil, &containers.Container{ID: id})
	if err != nil {
		return fmt.Errorf("fail to generate spec for container %s: %v", id, err)
	}

	options := []containerd.NewContainerOpts{
		containerd.WithSpec(spec),
		containerd.WithRuntime(fmt.Sprintf("io.containerd.runtime.v1.%s", runtime.GOOS), &runctypes.RuncOptions{
			Runtime: "docker-runc",
		}),
	}

	if _, err := cli.client.NewContainer(ctx, id, options...); err != nil {
		return fmt.Errorf("failed to create new containerd container %s: %v", id, err)
	}

	return nil
}

// GetContainer is to fetch a container info from containerd
func (cli *Client) GetContainer(ctx context.Context, id string) (containers.Container, error) {
	ctx = namespaces.WithNamespace(ctx, namespaces.Default)

	return cli.client.ContainerService().Get(ctx, id)
}

// DeleteContainer deletes a containerd container
func (cli *Client) DeleteContainer(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, namespaces.Default)

	return cli.client.ContainerService().Delete(ctx, id)
}

// GetImage inspect a containerd image
func (cli *Client) GetImage(ctx context.Context, imageName string) (containerd.Image, error) {
	ctx = namespaces.WithNamespace(ctx, namespaces.Default)
	return cli.client.GetImage(ctx, imageName)
}

// PullImage prepares all images using by docker containers,
// that will be used to create new pouch container.
func (cli *Client) PullImage(ctx context.Context, ref string, rePullAll bool) error {
	var err error
	newRef := image.AddDefaultRegistryIfMissing(ref, defaultRegistry, defaultNamespace)
	namedRef, err := reference.Parse(newRef)
	if err != nil {
		return err
	}

	namedRef = reference.TrimTagForDigest(reference.WithDefaultTagIfMissing(namedRef))
	resolver, err := resolver(&pouchtypes.AuthConfig{})
	if err != nil {
		return err
	}

	options := []containerd.RemoteOpt{
		containerd.WithSchema1Conversion,
		containerd.WithResolver(resolver),
	}

	// d2p-migrator will actually download the layer data if includeLayer is true
	includeLayer := false
	if rePullAll {
		includeLayer = true
	} else if _, ok := cli.RePullImageSet[ref]; ok {
		includeLayer = true
	}

	if includeLayer {
		options = append(options, containerd.WithPullUnpack)
		_, err = cli.client.Pull(ctx, namedRef.String(), options...)
	} else {
		err = image.PullManifest(ctx, cli.client, namedRef.String(), options...)
	}

	return err
}
