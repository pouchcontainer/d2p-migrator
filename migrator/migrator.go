package migrator

import (
	"context"
	"fmt"

	"github.com/pouchcontainer/d2p-migrator/ctrd"
	"github.com/pouchcontainer/d2p-migrator/docker"

	"github.com/sirupsen/logrus"
)

var (
	// DefaultDockerRpmName specify the default docker rpm name
	DefaultDockerRpmName = "docker"
	// DefaultPouchRpmPath specify the default pouch rpm file location
	DefaultPouchRpmPath = "pouch"
)

// Config contains the configurations of Migrator
type Config struct {
	// Type specifies the type of Migrator, now support living and cold two types.
	Type string
	// DockerRpmName represents the docker rpm name.
	DockerRpmName string
	// PouchRpmPath represents the path where pouch rpm location.
	PouchRpmPath string
	// DockerHomeDir represents the docker service home dir, we also can
	// get pouch service home dir by the DockerHomeDir
	DockerHomeDir string
	// d2p-migrator only download the image manifest file or not.
	ImageManifestOnly bool
}

// D2pMigrator is the core component to do migrate.
type D2pMigrator struct {
	config    Config
	migrator  Migrator
	dockerCli *docker.Dockerd
	ctrdCli   *ctrd.Client
	ctrdPid   int
}

// NewD2pMigrator create a migrator
func NewD2pMigrator(cfg Config) (*D2pMigrator, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("must specify migrator type")
	}
	if cfg.DockerRpmName == "" {
		cfg.DockerRpmName = DefaultDockerRpmName
	}
	if cfg.PouchRpmPath == "" {
		cfg.PouchRpmPath = DefaultPouchRpmPath
	}

	if _, ok := migratorFactory[cfg.Type]; !ok {
		return nil, fmt.Errorf("migrator type %s not found", cfg.Type)
	}

	// validate the docker environment
	dockerCli, err := docker.NewDockerd()
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %v", err)
	}
	info, err := dockerCli.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get docker info: %v", err)
	}
	if err := validateDocker(info); err != nil {
		return nil, err
	}
	cfg.DockerHomeDir = info.DockerRootDir

	migrator, err := migratorFactory[cfg.Type](cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %v", err)
	}

	// start containerd for migrate
	ctrdPid, err := ctrd.StartContainerd(getPouchHomeDir(cfg.DockerHomeDir), true)
	if err != nil {
		return nil, fmt.Errorf("failed to start containerd instance: %v", err)
	}

	// create containerd client
	ctrdCli, err := ctrd.NewCtrdClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get containerd client: %v", err)
	}

	d2pMigrator := &D2pMigrator{
		config:    cfg,
		migrator:  migrator,
		dockerCli: dockerCli,
		ctrdPid:   ctrdPid,
		ctrdCli:   ctrdCli,
	}

	return d2pMigrator, nil
}

// initPouchEnv initialize environment for pouch,
// it will be called in PreMigrate.
func (d *D2pMigrator) initPouchEnv() error {
	// prepare environment for pouch
	pouchHomeDir := getPouchHomeDir(d.config.DockerHomeDir)
	if err := prepareConfigForPouch(pouchHomeDir); err != nil {
		return fmt.Errorf("failed to prepare config for pouch: %v", err)
	}

	return nil
}

// PreMigrate prepares things for the migration
func (d *D2pMigrator) PreMigrate(ctx context.Context) error {
	if err := d.initPouchEnv(); err != nil {
		return err
	}

	return d.migrator.PreMigrate(ctx, d.dockerCli, d.ctrdCli)
}

// Migrate  does migrate procedures
func (d *D2pMigrator) Migrate(ctx context.Context) error {
	if err := d.migrator.Migrate(ctx, d.dockerCli, d.ctrdCli); err != nil {
		return err
	}

	// after migrate finished, stop containerd instance
	if err := ctrd.Cleanup(d.ctrdPid); err != nil {
		return fmt.Errorf("failed to kill containerd instance: %v", err)
	}

	return nil
}

// PostMigrate do something after migration
func (d *D2pMigrator) PostMigrate(ctx context.Context) error {
	return d.migrator.PostMigrate(ctx, d.dockerCli, d.ctrdCli, d.config.DockerRpmName, d.config.PouchRpmPath)
}

// RevertMigration reverts the migration
func (d *D2pMigrator) RevertMigration(ctx context.Context) error {
	return d.migrator.RevertMigration(ctx, d.dockerCli, d.ctrdCli)
}

// Cleanup does some clean works when migrator exited
func (d *D2pMigrator) Cleanup() error {
	return d.migrator.Cleanup()
}

// PrepareImages just pull images for containers
func (d *D2pMigrator) PrepareImages(ctx context.Context) error {
	// Get all docker containers on host.
	containers, err := d.dockerCli.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	for _, c := range containers {
		meta, err := d.dockerCli.ContainerInspect(c.ID)
		if err != nil {
			return fmt.Errorf("failed to inspect container %s: %v", c.ID, err)
		}

		image, err := d.dockerCli.ImageInspect(meta.Image)
		if err != nil {
			return fmt.Errorf("failed to inspect image %s: %v", meta.Image, err)
		}

		var imageName string
		if len(image.RepoTags) > 0 {
			imageName = image.RepoTags[0]
		} else if len(image.RepoDigests) > 0 {
			imageName = image.RepoDigests[0]
		} else {
			return fmt.Errorf("failed to get image %s: repoTags is empty", meta.Image)
		}

		// check image existence
		_, err = d.ctrdCli.GetImage(ctx, imageName)
		if err == nil {
			logrus.Infof("image %s has been downloaded, skip pull image", imageName)
			continue
		}

		logrus.Infof("Start pull image: %s", imageName)
		if err := d.ctrdCli.PullImage(ctx, imageName, d.config.ImageManifestOnly); err != nil {
			return fmt.Errorf("failed to pull image %s: %v", imageName, err)
		}
		logrus.Infof("End pull image: %s", imageName)
	}

	return nil
}
