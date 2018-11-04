package migrator

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/pouchcontainer/d2p-migrator/ctrd"
	"github.com/pouchcontainer/d2p-migrator/docker"
	"github.com/pouchcontainer/d2p-migrator/pouch"
	"github.com/pouchcontainer/d2p-migrator/pouch/convertor"
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	"github.com/containerd/containerd/errdefs"
	"github.com/sirupsen/logrus"
)

func init() {
	Register("live-migrate", NewLiveMigrator)
}

// liveMigrator is a tool to migrate docker containers to pouch containers,
// which can auto take over all running containers.
type liveMigrator struct {
	// dockerd home dir
	dockerHomeDir string
	// store all containers info
	allContainers map[string]bool
}

// NewLiveMigrator creates a migrator tool instance.
func NewLiveMigrator(cfg Config) (Migrator, error) {
	migrator := &liveMigrator{
		dockerHomeDir: cfg.DockerHomeDir,
		allContainers: map[string]bool{},
	}

	return migrator, nil
}

// PreMigrate prepares things for migration
// TODO: need to add more details
func (lm *liveMigrator) PreMigrate(ctx context.Context, dockerCli *docker.Dockerd) error {
	// Get all docker containers on host.
	containers, err := dockerCli.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	if len(containers) == 0 {
		logrus.Info("Empty host, no need covert containers")
		return nil
	}
	logrus.Debugf("Get %d containers", len(containers))

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
		pouchHomeDir  = getPouchHomeDir(lm.dockerHomeDir)
		containersDir = path.Join(pouchHomeDir, "containers")
	)

	// get container client
	ctrdCli, err := ctrd.NewCtrdClient()
	if err != nil {
		return fmt.Errorf("failed to get containerd client: %v", err)
	}

	for _, c := range containers {
		lm.allContainers[c.ID] = false

		meta, err := dockerCli.ContainerInspect(c.ID)
		if err != nil {
			return err
		}

		// convert docker meta information to pouch meta
		pouchMeta, err := convertor.ToPouchContainerMeta(&meta)
		if err != nil {
			return err
		}

		// count container volumes reference
		if err := ContainerVolumeRefsCount(pouchMeta, volumeRefs); err != nil {
			logrus.Errorf("failed to count container %s volumes reference: %v", pouchMeta.ID, err)
		}

		// prepare for migration
		if err := lm.doPrepare(ctx, ctrdCli, pouchMeta); err != nil {
			return err
		}

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

func (lm *liveMigrator) prepareCtrdContainers(ctx context.Context, ctrdCli *ctrd.Client, meta *localtypes.Container) error {
	// only prepare containerd container for running docker containers
	if meta.State.Status != pouchtypes.StatusRunning {
		return nil
	}

	logrus.Infof("auto take over running container %s", meta.ID)
	if _, err := ctrdCli.GetContainer(ctx, meta.ID); err == nil { // container already exist
		if err := ctrdCli.DeleteContainer(ctx, meta.ID); err != nil {
			return fmt.Errorf("failed to delete already existed containerd container %s: %v", meta.ID, err)
		}
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("failed to get containerd container: %v", err)
	}

	return ctrdCli.NewContainer(ctx, meta.ID)
}

// doPrepare prepares image and snapshot by using old container info.
func (lm *liveMigrator) doPrepare(ctx context.Context, ctrdCli *ctrd.Client, meta *localtypes.Container) error {
	return lm.prepareCtrdContainers(ctx, ctrdCli, meta)
}

// Migrate just migrates network files here when living migration.
func (lm *liveMigrator) Migrate(ctx context.Context, dockerCli *docker.Dockerd) error {
	pouchHomeDir := getPouchHomeDir(lm.dockerHomeDir)
	return migrateNetworkFile(lm.dockerHomeDir, pouchHomeDir)
}

// PostMigrate does something after migration.
func (lm *liveMigrator) PostMigrate(ctx context.Context, dockerCli *docker.Dockerd, dockerRpmName, pouchRpmPath string) error {
	// Get all docker containers on host again,
	// In case, it may have containers being deleted
	// Notes: we will lock host first, so there will have no
	// new containers created
	containers, err := dockerCli.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	for _, c := range containers {
		if _, exists := lm.allContainers[c.ID]; exists {
			lm.allContainers[c.ID] = true
		}
	}

	deletedContainers := []string{}
	for id, exists := range lm.allContainers {
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

	logrus.Info("PostMigrate done!!!")
	return nil
}

// RevertMigration reverts migration.
func (lm *liveMigrator) RevertMigration(ctx context.Context, dockerCli *docker.Dockerd) error {
	return nil
}

// Cleanup does some clean works when migrator exited.
func (lm *liveMigrator) Cleanup() error {
	return nil
}
