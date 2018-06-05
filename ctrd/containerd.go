package ctrd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/pouchcontainer/d2p-migrator/utils"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/linux/runctypes"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/image-spec/identity"
)

const (
	defaultSnapshotterName = "overlayfs"
)

// Ctrd is a wrapper client for containerd grpc client.
type Ctrd struct {
	client    *containerd.Client
	lease     *containerd.Lease
	daemonPid int
	rpcAddr   string
	homeDir   string
}

// NewCtrd create a new Ctrd instance.
func NewCtrd(homeDir string, debug bool) (*Ctrd, error) {
	ctrd := &Ctrd{
		homeDir:   homeDir,
		rpcAddr:   "/tmp/containerd-migrator.socket",
		daemonPid: -1,
	}

	// if socket file exists, delete it.
	if _, err := os.Stat(ctrd.rpcAddr); err == nil {
		os.RemoveAll(ctrd.rpcAddr)
	}

	containerdPath, err := exec.LookPath("/usr/local/bin/containerd")
	if err != nil {
		return nil, fmt.Errorf("failed to find containerd binary %v", err)
	}

	// Start a new containerd instance.
	cmd, err := ctrd.newContainerdCmd(containerdPath, debug)
	go cmd.Wait()

	ctrd.daemonPid = cmd.Process.Pid

	client, err := containerd.New(ctrd.rpcAddr, containerd.WithDefaultNamespace("default"))
	if err != nil {
		syscall.Kill(ctrd.daemonPid, syscall.SIGKILL)
		return nil, err
	}

	// create a new lease or reuse the existed.
	var lease containerd.Lease
	leases, err := client.ListLeases(context.TODO())
	if err != nil {
		syscall.Kill(ctrd.daemonPid, syscall.SIGKILL)
		return nil, err
	}
	if len(leases) != 0 {
		lease = leases[0]
	} else {
		if lease, err = client.CreateLease(context.TODO()); err != nil {
			return nil, err
		}
	}

	ctrd.client = client
	ctrd.lease = &lease

	return ctrd, nil
}

func (ctrd *Ctrd) newContainerdCmd(containerdPath string, debug bool) (*exec.Cmd, error) {
	args := []string{
		"-a", ctrd.rpcAddr,
		"--root", path.Join(ctrd.homeDir, "containerd/root"),
		"--state", path.Join(ctrd.homeDir, "containerd/state"),
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
func (ctrd *Ctrd) Cleanup() error {
	if ctrd.daemonPid == -1 {
		return nil
	}

	// delete containerd process forcely.
	syscall.Kill(ctrd.daemonPid, syscall.SIGKILL)

	// clear some files.
	os.Remove(ctrd.rpcAddr)

	return nil
}

// PullImage prepares all images using by docker containers,
// that will be used to create new pouch container.
func (ctrd *Ctrd) PullImage(ctx context.Context, imageName string) error {
	ctx = leases.WithLease(ctx, ctrd.lease.ID())

	resolver, err := resolver(&pouchtypes.AuthConfig{})
	if err != nil {
		return err
	}

	options := []containerd.RemoteOpt{
		containerd.WithPullUnpack,
		containerd.WithSchema1Conversion,
		containerd.WithResolver(resolver),
	}
	_, err = ctrd.client.Pull(ctx, imageName, options...)
	return err
}

func resolver(authConfig *pouchtypes.AuthConfig) (remotes.Resolver, error) {
	var (
		// TODO
		username  = ""
		secret    = ""
		plainHTTP = false
		refresh   = ""
		insecure  = false
	)

	// FIXME
	_ = refresh

	options := docker.ResolverOptions{
		PlainHTTP: plainHTTP,
		Tracker:   docker.NewInMemoryTracker(),
	}
	options.Credentials = func(host string) (string, string, error) {
		// Only one host
		return username, secret, nil
	}

	tr := &http.Transport{
		Proxy: proxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
		ExpectContinueTimeout: 5 * time.Second,
	}

	options.Client = &http.Client{
		Transport: tr,
	}

	return docker.NewResolver(options), nil
}

// CreateSnapshot creates an active snapshot with image's name and id.
func (ctrd *Ctrd) CreateSnapshot(ctx context.Context, id, ref string) error {
	ctx = leases.WithLease(ctx, ctrd.lease.ID())

	image, err := ctrd.client.ImageService().Get(ctx, ref)
	if err != nil {
		return err
	}

	diffIDs, err := image.RootFS(ctx, ctrd.client.ContentStore(), platforms.Default())
	if err != nil {
		return err
	}

	parent := identity.ChainID(diffIDs).String()
	if _, err := ctrd.client.SnapshotService(defaultSnapshotterName).Prepare(ctx, id, parent); err != nil {
		return err
	}

	return nil
}

// GetSnapshot returns the snapshot's info by id.
func (ctrd *Ctrd) GetSnapshot(ctx context.Context, id string) (snapshots.Info, error) {
	service := ctrd.client.SnapshotService(defaultSnapshotterName)
	defer service.Close()

	return service.Stat(ctx, id)
}

// GetMounts returns the mounts for the active snapshot transaction identified by key.
func (ctrd *Ctrd) GetMounts(ctx context.Context, id string) ([]mount.Mount, error) {
	service := ctrd.client.SnapshotService(defaultSnapshotterName)
	defer service.Close()

	return service.Mounts(ctx, id)
}

// RemoveSnapshot remove the snapshot by id.
func (ctrd *Ctrd) RemoveSnapshot(ctx context.Context, id string) error {
	service := ctrd.client.SnapshotService(defaultSnapshotterName)
	defer service.Close()

	return service.Remove(ctx, id)

}

// NewContainer just create a very simple container for migration
// just load container id to boltdb
func (ctrd *Ctrd) NewContainer(ctx context.Context, id string) error {
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

	if _, err := ctrd.client.NewContainer(ctx, id, options...); err != nil {
		return fmt.Errorf("failed to create new containerd container %s: %v", id, err)
	}

	return nil
}

// GetContainer is to fetch a container info from containerd
func (ctrd *Ctrd) GetContainer(ctx context.Context, id string) (containers.Container, error) {
	ctx = namespaces.WithNamespace(ctx, namespaces.Default)

	return ctrd.client.ContainerService().Get(ctx, id)
}

// DeleteContainer deletes a containerd container
func (ctrd *Ctrd) DeleteContainer(ctx context.Context, id string) error {

	ctx = namespaces.WithNamespace(ctx, namespaces.Default)

	return ctrd.client.ContainerService().Delete(ctx, id)
}
