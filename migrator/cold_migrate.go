package migrator

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/pouchcontainer/d2p-migrator/ctrd"
	"github.com/pouchcontainer/d2p-migrator/docker"
	"github.com/pouchcontainer/d2p-migrator/pouch"
	"github.com/pouchcontainer/d2p-migrator/pouch/convertor"
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
	"github.com/pouchcontainer/d2p-migrator/utils"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	"github.com/containerd/containerd/errdefs"
	dockerclient "github.com/docker/engine-api/client"
	"github.com/sirupsen/logrus"
)

func init() {
	Register("cold-migrate", NewColdMigrator)
}

// Actions that coldMigrator migration does.
// 0. Install containerd1.0.3
// 1. Pull Images
// 2. Prepare Snapshots
// 3. Set QuotaID for upperDir and workDir
// 4. Stop all containers and alidocker.
// 5. mv oldUpperDir/* => upperDir/
// 6. Convert oldContainerMeta to PouchContainer container metaJSON
// 7. Stop containerd
// 8. Install pouch
// 9. Start all container

// coldMigrator is a tool to migrate docker containers to pouch containers,
// which we should first stop all containers before migration.
type coldMigrator struct {
	// dockerd home dir
	dockerHomeDir string

	// store map of old UpperDir and new UpperDir
	upperDirMappingList []*upperDirMapping
	// store all containers info
	allContainers map[string]bool
	// store all running containers
	runningContainers []string
	// store all images using by containers
	images map[string]struct{}
}

// upperDirMapping stores overlayfs upperDir map for docker and pouch.
type upperDirMapping struct {
	// specify docker UpperDir
	srcDir string
	// specify pouch UpperDir
	dstDir string
}

// NewColdMigrator creates a migrator tool instance.
func NewColdMigrator(cfg Config) (Migrator, error) {
	migrator := &coldMigrator{
		dockerHomeDir: cfg.DockerHomeDir,
		allContainers: map[string]bool{},
	}

	return migrator, nil
}

// PreMigrate prepares things for migration
// * pull image to pouch
// * create snapshot for container
// * set snapshot upperDir, workDir diskquota
// * convert docker container metaJSON to pouch container metaJSON
func (cm *coldMigrator) PreMigrate(ctx context.Context, dockerCli *docker.Dockerd, ctrdCli *ctrd.Client) error {
	// Get all docker containers on host.
	containers, err := dockerCli.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	if len(containers) == 0 {
		logrus.Info("Empty host, no need covert containers")
		return nil
	}

	// Get all volumes on host.
	dockerVolumes, err := dockerCli.VolumeList()
	if err != nil {
		return fmt.Errorf("failed to get volumes list: %v", err)
	}
	pouchVolumes, err := convertor.ToVolumes(dockerVolumes)
	if err != nil {
		return err
	}

	var (
		// volumeRefs to count volume references
		volumeRefs    = map[string]string{}
		pouchHomeDir  = getPouchHomeDir(cm.dockerHomeDir)
		containersDir = path.Join(pouchHomeDir, "containers")
	)

	for _, c := range containers {
		cm.allContainers[c.ID] = false
		// TODO: not consider status paused
		if c.State == "running" {
			cm.runningContainers = append(cm.runningContainers, c.ID)
		}

		meta, err := dockerCli.ContainerInspect(c.ID)
		if err != nil {
			return err
		}

		pouchMeta, err := convertor.ToPouchContainerMeta(&meta)
		if err != nil {
			return err
		}

		// count container volumes reference
		if err := ContainerVolumeRefsCount(pouchMeta, volumeRefs); err != nil {
			logrus.Errorf("failed to count container %s volumes reference: %v", pouchMeta.ID, err)
		}

		// prepare for migration
		if err := cm.doPrepare(ctx, ctrdCli, pouchMeta); err != nil {
			return err
		}

		// change BaseFS
		pouchMeta.BaseFS = path.Join(pouchHomeDir, "containerd/state/io.containerd.runtime.v1.linux/default", meta.ID, "rootfs")
		// RootFSProvided unset
		pouchMeta.RootFSProvided = false

		// store upperDir mapping
		cm.upperDirMappingList = append(cm.upperDirMappingList, &upperDirMapping{
			srcDir: meta.GraphDriver.Data["UpperDir"],
			dstDir: pouchMeta.Snapshotter.Data["UpperDir"],
		})

		// Save container meta json to disk.
		if err := save2Disk(containersDir, pouchMeta); err != nil {
			return err
		}
	}

	// prepare volumes for volumes
	if err := PrepareVolumes(pouchHomeDir, pouchVolumes, volumeRefs); err != nil {
		return nil
	}

	return nil
}

func (cm *coldMigrator) getOverlayFsDir(ctx context.Context, ctrdCli *ctrd.Client, snapID string) (string, string, error) {
	var (
		upperDir string
		workDir  string
	)

	mounts, err := ctrdCli.GetMounts(ctx, snapID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get snapshot %s mounts: %v", snapID, err)
	} else if len(mounts) != 1 {
		return "", "", fmt.Errorf("failed to get snapshots %s mounts: not equals one", snapID)
	}

	for _, opt := range mounts[0].Options {
		if strings.HasPrefix(opt, "upperdir=") {
			upperDir = strings.TrimPrefix(opt, "upperdir=")
		} else if strings.HasPrefix(opt, "workdir=") {
			workDir = strings.TrimPrefix(opt, "workdir=")
		}
	}

	return upperDir, workDir, nil
}

func (cm *coldMigrator) prepareCtrdContainers(ctx context.Context, ctrdCli *ctrd.Client, meta *localtypes.Container) error {
	// only prepare containerd container for running docker containers
	if meta.State.Status != pouchtypes.StatusRunning {
		return nil
	}

	logrus.Infof("auto take over running container %s, no need convert process", meta.ID)
	_, err := ctrdCli.GetContainer(ctx, meta.ID)
	if err == nil { // container already exist
		if err := ctrdCli.DeleteContainer(ctx, meta.ID); err != nil {
			return fmt.Errorf("failed to delete already existed containerd container %s: %v", meta.ID, err)
		}
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("failed to get containerd container: %v", err)
	}

	return ctrdCli.NewContainer(ctx, meta.ID)
}

// doPrepare prepares image and snapshot by using old container info.
func (cm *coldMigrator) doPrepare(ctx context.Context, ctrdCli *ctrd.Client, meta *localtypes.Container) error {
	// check image existence
	img := meta.Config.Image
	_, imageExist := cm.images[img]
	if !imageExist {
		cm.images[img] = struct{}{}
	}

	// Pull image
	if imageExist {
		logrus.Infof("image %s has been downloaded, skip pull image", img)
	} else {
		logrus.Infof("Start pull image: %s", img)
		if err := ctrdCli.PullImage(ctx, img, true); err != nil {
			logrus.Errorf("failed to pull image %s: %v\n", img, err)
			return err
		}
		logrus.Infof("End pull image: %s", img)
	}

	logrus.Infof("Start prepare snapshot %s", meta.ID)
	_, err := ctrdCli.GetSnapshot(ctx, meta.ID)
	if err == nil {
		logrus.Infof("Snapshot %s already exists, delete it", meta.ID)
		ctrdCli.RemoveSnapshot(ctx, meta.ID)
	}
	// CreateSnapshot for new pouch container
	if err := ctrdCli.CreateSnapshot(ctx, meta.ID, img); err != nil {
		return err
	}

	upperDir, workDir, err := cm.getOverlayFsDir(ctx, ctrdCli, meta.ID)
	if err != nil {
		return err
	}
	if upperDir == "" || workDir == "" {
		return fmt.Errorf("snapshot mounts occurred an error: upperDir=%s, workDir=%s", upperDir, workDir)
	}

	// If need convert docker container to pouch container,
	// we should also convert Snapshotter Data
	meta.Snapshotter.Data = map[string]string{}
	meta.Snapshotter.Data["UpperDir"] = upperDir

	// Set diskquota for UpperDir and WorkDir.
	diskQuota := ""
	if v, exists := meta.Config.Labels["DiskQuota"]; exists {
		diskQuota = v
	}

	for _, dir := range []string{upperDir, workDir} {
		if err := SetDirDiskQuota(diskQuota, meta.Config.QuotaID, dir); err != nil {
			return err
		}
	}
	logrus.Infof("Set diskquota for snapshot %s done", meta.ID)

	return nil
}

// Migrate migrates docker containers to pouch containers:
// * stop all docker containers
// * mv oldUpperDir/* newUpperDir/
func (cm *coldMigrator) Migrate(ctx context.Context, dockerCli *docker.Dockerd, ctrdCli *ctrd.Client) error {
	pouchHomeDir := getPouchHomeDir(cm.dockerHomeDir)
	if err := migrateNetworkFile(cm.dockerHomeDir, pouchHomeDir); err != nil {
		return err
	}
	// Stop all running containers
	timeout := time.Duration(1) * time.Second
	for _, c := range cm.runningContainers {
		logrus.Infof("Start stop container %s", c)
		if err := dockerCli.ContainerStop(c, &timeout); err != nil {
			if !dockerclient.IsErrNotFound(err) {
				return fmt.Errorf("failed to stop container: %v", err)
			}
		}
	}

	// Only mv stopped containers' upperDir
	// mv oldUpperDir/* newUpperDir/
	for _, dirMapping := range cm.upperDirMappingList {
		// TODO(ziren): need more reasonable method
		if err := utils.ExecCommand("touch", dirMapping.srcDir+"/d2p-migrator.txt"); err != nil {
			logrus.Errorf("failed to touch d2p-migrator.txt file: %v", err)
		}

		if err := utils.MoveDir(dirMapping.srcDir, dirMapping.dstDir); err != nil {
			logrus.Errorf("failed to mv upperDir: %v", err)
			return err
		}
	}

	return nil
}

// PostMigrate does something after migration.
func (cm *coldMigrator) PostMigrate(ctx context.Context, dockerCli *docker.Dockerd, ctrdCli *ctrd.Client, dockerRpmName, pouchRpmPath string) error {
	// Get all docker containers on host again,
	// In case, there will have containers be deleted
	// Notes: we will lock host first, so there will have no
	// new containers created
	containers, err := dockerCli.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	for _, c := range containers {
		if _, exists := cm.allContainers[c.ID]; exists {
			cm.allContainers[c.ID] = true
		}
	}

	deletedContainers := []string{}
	for id, exists := range cm.allContainers {
		if !exists {
			deletedContainers = append(deletedContainers, id)
		}
	}

	// uninstall docker
	if err := docker.UninstallDockerService(dockerRpmName); err != nil {
		return err
	}

	// install pouch
	logrus.Infof("Start install pouch: %s", pouchRpmPath)
	if err := pouch.InstallPouchService(pouchRpmPath); err != nil {
		return err
	}

	// TODO should specify pouchd socket path
	pouchCli, err := pouch.NewPouchClient("")
	if err != nil {
		logrus.Errorf("failed to create a pouch client: %v, need start container by manual", err)
		return err
	}

	logrus.Infof("%d containers have been deleted", len(deletedContainers))
	for _, c := range deletedContainers {
		if err := pouchCli.ContainerRemove(ctx, c, &pouchtypes.ContainerRemoveOptions{Force: true}); err != nil {
			if !strings.Contains(err.Error(), "not found") {
				return err
			}
		}
	}

	// after start pouch we should clean docker0 bridge, if not take over
	// old containers
	if err := utils.ExecCommand("ip", "link", "del", "docker0"); err != nil {
		logrus.Errorf("failed to delete docker0 bridge: %v", err)
	}

	// Start all containers need being running
	for _, c := range cm.runningContainers {
		if utils.StringInSlice(deletedContainers, c) {
			continue
		}

		logrus.Infof("Start starting container %s", c)
		if err := pouchCli.ContainerStart(ctx, c, ""); err != nil {
			logrus.Errorf("failed to start container %s: %v", c, err)
			return err
		}
	}

	logrus.Info("PostMigrate done!!!")
	return nil
}

// RevertMigration reverts migration.
func (cm *coldMigrator) RevertMigration(ctx context.Context, dockerCli *docker.Dockerd, ctrdCli *ctrd.Client) error {
	// Then, move all upperDir back
	for _, dirMapping := range cm.upperDirMappingList {
		if err := utils.MoveDir(dirMapping.dstDir, dirMapping.srcDir); err != nil {
			logrus.Errorf("%v\n", err)

			return err
		}
	}

	// Start all running containers
	for _, c := range cm.runningContainers {
		if err := dockerCli.ContainerStart(c); err != nil {
			return fmt.Errorf("failed start container: %v", err)
		}
	}

	return nil
}

// Cleanup does some clean works when migrator exited.
func (cm *coldMigrator) Cleanup() error {
	return nil
}
