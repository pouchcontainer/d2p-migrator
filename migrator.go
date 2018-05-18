package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/alibaba/d2p-migrator/utils"
	"github.com/alibaba/pouch/storage/quota"
	"golang.org/x/net/context"
)

// Migrator is an interface to migrate docker containers to other containers
type Migrator interface {
	// PreMigrate do something before migration
	PreMigrate() error

	// Migrate does migrate action
	Migrate() error

	// PostMigrate do something after migration
	PostMigrate() error

	// RevertMigration reverts migration
	RevertMigration() error

	// Cleanup does some clean works when migrator exited
	Cleanup() error
}

// Actions that PouchMigrator migration does.
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

// PouchMigrator is a tool to migrate docker containers to pouch containers
type PouchMigrator struct {
	debug         bool
	containerd    *Ctrd
	dockerd       *Dockerd
	pouchHomeDir  string
	dockerHomeDir string
	pouchPkgPath  string
	dockerPkg     string

	upperDirMappingList []*UpperDirMapping
	runningContainers   []string
}

// UpperDirMapping stores overlayfs upperDir map for docker and pouch.
type UpperDirMapping struct {
	// specify docker UpperDir
	srcDir string
	// specify pouch UpperDir
	dstDir string
}

// NewPouchMigrator creates a migrator tool instance.
func NewPouchMigrator(dockerPkg, pouchPkgPath string, debug bool) (Migrator, error) {
	dockerCli, err := NewDockerd()
	if err != nil {
		return nil, err
	}

	// Only support overlayfs storage driver
	info, err := dockerCli.Info()
	if err != nil {
		return nil, err
	}

	homeDir := ""

	// Specify PouchRootDir, ensure new PouchRootDir should be in the same disk
	// with DockerRootDir
	if info.DockerRootDir == "" {
		return nil, fmt.Errorf("failed to get DockerRootDir")
	}
	rootDir := strings.TrimSuffix(info.DockerRootDir, "docker")
	homeDir = path.Join(rootDir, "pouch")

	if _, err := os.Stat(homeDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(homeDir, 0666); err != nil {
			return nil, fmt.Errorf("failed to mkdir: %v", err)
		}
	}

	// Check if we can migrate docker to pouch
	// if storage driver is not overlay, cannot do migration
	if info.Driver != "overlay" && info.Driver != "overlay2" {
		return nil, fmt.Errorf("d2p-migrator only support overlayfs Storage Driver")
	}

	// if host has remote disk, cannot do migration
	volumes, err := dockerCli.VolumeList()
	if err != nil {
		return nil, err
	}

	hasRemoteDisk := false
	for _, v := range volumes.Volumes {
		if utils.StringInSlice([]string{"ultron"}, v.Driver) {
			hasRemoteDisk = true
		}
	}
	if hasRemoteDisk {
		return nil, fmt.Errorf("d2p-migrate not support migrate remote dik")
	}

	ctrd, err := NewCtrd(homeDir, debug)
	if err != nil {
		return nil, err
	}

	migrator := &PouchMigrator{
		debug:         debug,
		containerd:    ctrd,
		dockerd:       dockerCli,
		pouchHomeDir:  homeDir,
		dockerHomeDir: info.DockerRootDir,
		dockerPkg:     dockerPkg,
		pouchPkgPath:  pouchPkgPath,
	}

	return migrator, nil
}

// PreMigrate prepares things for migration
// * pull image to pouch
// * create snapshot for container
// * set snapshot upperDir, workDir diskquota
// * convert docker container metaJSON to pouch container metaJSON
func (p *PouchMigrator) PreMigrate() error {
	// Get all docker containers on host.
	containers, err := p.dockerd.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	if len(containers) == 0 {
		logrus.Info(" === No containers on host, no need migrations === ")
		return nil
	}

	var (
		containersDir = path.Join(p.pouchHomeDir, "containers")
	)

	for _, c := range containers {
		p.runningContainers = append(p.runningContainers, c.ID)

		meta, err := p.dockerd.ContainerInspect(c.ID)
		if err != nil {
			return err
		}

		pouchMeta, err := ToPouchContainerMeta(&meta)
		if err != nil {
			return err
		}

		// meta.Image maybe a digest, we need image name.
		image, err := p.dockerd.ImageInspect(meta.Image)
		if err != nil {
			return err
		}
		if len(image.RepoTags) == 0 {
			return fmt.Errorf("failed to get image %s: repoTags is empty", meta.Image)
		}

		pouchMeta.Image = image.RepoTags[0]
		pouchMeta.Config.Image = image.RepoTags[0]
		pouchMeta.BaseFS = path.Join(p.pouchHomeDir, "containerd/state/io.containerd.runtime.v1.linux/default", meta.ID, "rootfs")

		if err := p.doPrepare(pouchMeta); err != nil {
			return err
		}

		// Save container meta json to disk.
		if err := p.save2Disk(containersDir, pouchMeta); err != nil {
			return err
		}

		// store upperDir mapping
		p.upperDirMappingList = append(p.upperDirMappingList, &UpperDirMapping{
			srcDir: meta.GraphDriver.Data["UpperDir"],
			dstDir: pouchMeta.Snapshotter.Data["UpperDir"],
		})
	}

	return nil
}

func (p *PouchMigrator) save2Disk(homeDir string, meta *PouchContainer) error {
	dir := path.Join(homeDir, meta.ID)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0744); err != nil {
				return fmt.Errorf("failed to mkdir %s: %v", dir, err)
			}
		}
	}

	value, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to encode meta data: %v", err)
	}

	fileName := path.Join(dir, "meta.json")
	f, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", fileName, err)
	}
	defer f.Close()

	if _, err := f.Write(value); err != nil {
		return fmt.Errorf("failed to write file %s: %v", fileName, err)
	}
	f.Sync()

	return nil
}

func (p *PouchMigrator) getOverlayFsDir(ctx context.Context, snapID string) (string, string, error) {
	var (
		upperDir string
		workDir  string
	)

	mounts, err := p.containerd.GetMounts(ctx, snapID)
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

// doPrepare prepares image and snapshot by using old container info.
func (p *PouchMigrator) doPrepare(meta *PouchContainer) error {
	ctx := context.Background()

	// Pull image
	logrus.Infof("Start pull image: %s", meta.Image)
	if err := p.containerd.PullImage(ctx, meta.Image); err != nil {
		logrus.Errorf("failed to pull image %s: %v\n", meta.Image, err)
		return err
	}
	logrus.Infof("End pull image: %s", meta.Image)

	logrus.Infof("Start prepare snapshot %s", meta.ID)
	_, err := p.containerd.GetSnapshot(ctx, meta.ID)
	if err == nil {
		logrus.Infof("Snapshot %s already exists, delete it", meta.ID)
		p.containerd.RemoveSnapshot(ctx, meta.ID)
	}
	// CreateSnapshot for new pouch container
	if err := p.containerd.CreateSnapshot(ctx, meta.ID, meta.Image); err != nil {
		return err
	}

	upperDir, workDir, err := p.getOverlayFsDir(ctx, meta.ID)
	if err != nil {
		return err
	}
	if upperDir == "" || workDir == "" {
		return fmt.Errorf("snapshot mounts occurred an error: upperDir=%s, workDir=%s", upperDir, workDir)
	}

	if meta.Snapshotter.Data == nil {
		meta.Snapshotter.Data = map[string]string{}
	}
	meta.Snapshotter.Data["UpperDir"] = upperDir

	// Set diskquota for UpperDir and WorkDir.
	diskQuota := ""
	if v, exists := meta.Config.Labels["DiskQuota"]; exists {
		diskQuota = v
	}

	for _, dir := range []string{upperDir, workDir} {
		if err := p.setDirDiskQuota(diskQuota, meta.Config.QuotaID, dir); err != nil {
			return err
		}
	}

	logrus.Infof("Set diskquota for snapshot %s done", meta.ID)
	return nil
}

func (p *PouchMigrator) setDirDiskQuota(defaultQuota, quotaID, dir string) error {
	if quotaID == "" || defaultQuota == "" {
		return nil
	}

	var qid uint32
	id, err := strconv.Atoi(quotaID)
	if err != nil {
		return fmt.Errorf("invalid argument, QuotaID: %s", quotaID)
	}

	// not set QuotaID
	if id <= 0 {
		return nil
	}

	qid = uint32(id)
	if qid > 0 && defaultQuota == "" {
		return fmt.Errorf("set quota id but have no set default quota size")
	}

	_, err = quota.StartQuotaDriver(dir)
	if err != nil {
		return fmt.Errorf("failed to start quota driver: %v", err)
	}

	qid, err = quota.SetSubtree(dir, qid)
	if err != nil {
		return fmt.Errorf("failed to set subtree: %v", err)
	}

	if err := quota.SetDiskQuota(dir, defaultQuota, qid); err != nil {
		return fmt.Errorf("failed to set disk quota: %v", err)
	}

	qotaSetFunc := func(path string, fd os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to set diskquota for walk dir %s: %v", path, err)
		}

		quota.SetFileAttrNoOutput(path, qid)

		return nil
	}

	if err := filepath.Walk(dir, qotaSetFunc); err != nil {
		return err
	}

	return nil
}

// Migrate migrates docker containers to pouch containers:
// * stop all docker containers
// * mv oldUpperDir/* newUpperDir/
func (p *PouchMigrator) Migrate() error {

	// Copy network db file
	dbName := "local-kv.db"
	dstNetDBDir := path.Join(p.pouchHomeDir, "network/files")
	dstNetDBFile := path.Join(dstNetDBDir, dbName)
	srcNetDBDir := path.Join(p.dockerHomeDir, "network/files")
	if _, err := os.Stat(path.Join(srcNetDBDir, dbName)); err != nil {
		return err
	}

	if _, err := os.Stat(dstNetDBDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(dstNetDBDir, 0666); err != nil {
			return fmt.Errorf("failed to mkdir: %v", err)
		}
	}
	if _, err := os.Stat(dstNetDBFile); err == nil {
		if err := os.RemoveAll(dstNetDBFile); err != nil {
			return fmt.Errorf("failed to delelte old network db file: %v", err)
		}
	}

	if err := utils.ExecCommand("cp", path.Join(srcNetDBDir, dbName), dstNetDBDir); err != nil {
		return fmt.Errorf("failed to prepare network db file: %v", err)
	}

	// Stop all running containers
	timeout := time.Duration(1) * time.Second
	for _, c := range p.runningContainers {
		if err := p.dockerd.ContainerStop(c, &timeout); err != nil {
			return fmt.Errorf("failed to stop container: %v", err)
		}
	}

	// mv oldUpperDir/* newUpperDir/
	for _, dirMapping := range p.upperDirMappingList {
		isEmpty, err := utils.IsDirEmpty(dirMapping.srcDir)
		if err != nil {
			return err
		}
		if isEmpty {
			continue
		}

		if err := utils.MoveDir(dirMapping.srcDir, dirMapping.dstDir); err != nil {
			logrus.Errorf("failed to mv upperDir: %v", err)
			return err
		}
	}

	return nil
}

// PostMigrate does something after migration.
func (p *PouchMigrator) PostMigrate() error {
	// stop containerd instance
	p.containerd.Cleanup()

	// Uninstall docker
	// TODO backup two config files: /etc/sysconfig/docker, /etc/docker/daemon.jon
	// In case we revert migration.
	for _, f := range []string{"/etc/sysconfig/docker", "/etc/docker/daemon.json"} {
		if err := utils.ExecCommand("cp", f, f+".bk"); err != nil {
			return err
		}
	}

	logrus.Infof("Start to uninstall docker: %s", p.dockerPkg)
	if err := utils.ExecCommand("yum", "remove", "-y", p.dockerPkg); err != nil {
		return fmt.Errorf("failed to uninstall docker: %v", err)
	}

	// Install pouch
	logrus.Infof("Start install pouch: %s", p.pouchPkgPath)
	// time.Sleep(20 * time.Second)
	if err := utils.ExecCommand("yum", "install", "-y", p.pouchPkgPath); err != nil {
		logrus.Errorf("failed to install pouch: %v", err)
		return err
	}

	// Change pouch config file
	if err := utils.ExecCommand("sed", "-i", fmt.Sprintf(`s|\("home-dir": "\).*|\1%s",|`, p.pouchHomeDir), "/etc/pouch/config.json"); err != nil {
		return fmt.Errorf("failed to change pouch config file: %v", err)
	}

	// Restart pouch.service
	if err := utils.ExecCommand("systemctl", "restart", "pouch"); err != nil {
		return fmt.Errorf("failed to restart pouch: %v", err)
	}

	logrus.Info("Start start containers")
	// TODO should specify pouchd socket path
	pouchCli, err := NewPouchClient("")
	if err != nil {
		logrus.Errorf("failed to create a pouch client: %v, need start container by manual", err)
		return err
	}

	// Start all containers need being running
	for _, c := range p.runningContainers {
		if err := pouchCli.ContainerStart(context.Background(), c, ""); err != nil {
			logrus.Errorf("failed to start container %s: %v", c, err)

			return err
		}
	}

	logrus.Info("PostMigrate done!!!")
	return nil
}

// RevertMigration reverts migration.
func (p *PouchMigrator) RevertMigration() error {
	// Then, move all upperDir back
	for _, dirMapping := range p.upperDirMappingList {
		if err := utils.MoveDir(dirMapping.dstDir, dirMapping.srcDir); err != nil {
			logrus.Errorf("%v\n", err)

			return err
		}
	}

	// Start all running containers
	for _, c := range p.runningContainers {
		if err := p.dockerd.ContainerStart(c); err != nil {
			return fmt.Errorf("failed start container: %v", err)
		}
	}

	return nil
}

// Cleanup does some clean works when migrator exited.
func (p *PouchMigrator) Cleanup() error {
	return p.containerd.Cleanup()
}
