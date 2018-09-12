package migrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pouchcontainer/d2p-migrator/ctrd"
	"github.com/pouchcontainer/d2p-migrator/docker"
	"github.com/pouchcontainer/d2p-migrator/pouch"
	"github.com/pouchcontainer/d2p-migrator/utils"

	"github.com/Sirupsen/logrus"
	pouchtypes "github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/storage/quota"
	"github.com/containerd/containerd/errdefs"
	dockerCli "github.com/docker/engine-api/client"
)

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
	// debug switch
	debug bool
	// if this switch is on, we just run the code, not uninstall docker rpm
	dryRun bool

	// containerd config
	containerd *ctrd.Ctrd

	// pouchd home dir
	pouchHomeDir string
	// pouch rpm package path, the relative path or absolute path
	pouchPkgPath string

	// docker config
	dockerd *docker.Dockerd
	// dockerd home dir
	dockerHomeDir string
	// pouch rpm package name
	dockerPkg string

	// store map of old UpperDir and new UpperDir
	upperDirMappingList []*UpperDirMapping
	// store all containers info
	allContainers map[string]bool
	// store all running containers
	runningContainers []string
	// store all images using by containers
	images map[string]struct{}
}

// UpperDirMapping stores overlayfs upperDir map for docker and pouch.
type UpperDirMapping struct {
	// specify docker UpperDir
	srcDir string
	// specify pouch UpperDir
	dstDir string
}

// NewPouchMigrator creates a migrator tool instance.
func NewPouchMigrator(dockerPkg, pouchPkgPath string, debug, dryRun bool) (Migrator, error) {
	dockerCli, err := docker.NewDockerd()
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

	ctrd, err := ctrd.NewCtrd(homeDir, debug)
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
		allContainers: map[string]bool{},
		dryRun:        dryRun,
		images:        map[string]struct{}{},
	}

	return migrator, nil
}

// PreMigrate prepares things for migration
// * pull image to pouch
// * create snapshot for container
// * set snapshot upperDir, workDir diskquota
// * convert docker container metaJSON to pouch container metaJSON
func (p *PouchMigrator) PreMigrate(ctx context.Context, takeOverContainer bool) error {
	// Change pouch config file
	if _, err := os.Stat("/etc/pouch/config.json"); err == nil {
		if err := utils.ExecCommand("sed", "-i", fmt.Sprintf(`s|\("home-dir": "\).*|\1%s",|`, p.pouchHomeDir), "/etc/pouch/config.json"); err != nil {
			logrus.Errorf("failed to change pouch config file: %v", err)
		}
	}

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

	containersDir := path.Join(p.pouchHomeDir, "containers")

	for _, c := range containers {
		p.allContainers[c.ID] = false

		// TODO: not consider status paused
		if c.State == "running" {
			p.runningContainers = append(p.runningContainers, c.ID)
		}

		meta, err := p.dockerd.ContainerInspect(c.ID)
		if err != nil {
			return err
		}

		pouchMeta, err := pouch.ToPouchContainerMeta(&meta)
		if err != nil {
			return err
		}

		// prepare for migration
		if err := p.doPrepare(ctx, pouchMeta, takeOverContainer); err != nil {
			return err
		}

		if !takeOverContainer {
			// change BaseFS
			pouchMeta.BaseFS = path.Join(p.pouchHomeDir, "containerd/state/io.containerd.runtime.v1.linux/default", meta.ID, "rootfs")

			// RootFSProvided unset
			pouchMeta.RootFSProvided = false

			// store upperDir mapping
			p.upperDirMappingList = append(p.upperDirMappingList, &UpperDirMapping{
				srcDir: meta.GraphDriver.Data["UpperDir"],
				dstDir: pouchMeta.Snapshotter.Data["UpperDir"],
			})
		}

		// Save container meta json to disk.
		if err := p.save2Disk(containersDir, pouchMeta); err != nil {
			return err
		}
	}

	logrus.Infof("running containers: %v", p.runningContainers)
	return nil
}

func (p *PouchMigrator) save2Disk(homeDir string, meta *pouch.PouchContainer) error {
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

func (p *PouchMigrator) prepareCtrdContainers(ctx context.Context, meta *pouch.PouchContainer) error {
	// only prepare containerd container for running docker containers
	if meta.State.Status != pouchtypes.StatusRunning {
		return nil
	}

	logrus.Infof("auto take over running container %s, no need convert process", meta.ID)
	_, err := p.containerd.GetContainer(ctx, meta.ID)
	if err == nil { // container already exist
		if err := p.containerd.DeleteContainer(ctx, meta.ID); err != nil {
			return fmt.Errorf("failed to delete already existed containerd container %s: %v", meta.ID, err)
		}
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("failed to get containerd container: %v", err)
	}

	return p.containerd.NewContainer(ctx, meta.ID)
}

// doPrepare prepares image and snapshot by using old container info.
func (p *PouchMigrator) doPrepare(ctx context.Context, meta *pouch.PouchContainer, takeOverContainer bool) error {
	// check image existance
	img := meta.Config.Image
	_, imageExist := p.images[img]
	if !imageExist {
		p.images[img] = struct{}{}
	}

	// if takeOverContainer set, just prepare containerd containers for running containers
	if takeOverContainer {
		return p.prepareCtrdContainers(ctx, meta)
	}

	// Pull image
	if imageExist {
		logrus.Infof("image %s has been downloaded, skip pull image", img)
	} else {
		logrus.Infof("Start pull image: %s", img)
		if err := p.containerd.PullImage(ctx, img); err != nil {
			logrus.Errorf("failed to pull image %s: %v\n", img, err)
			return err
		}
		logrus.Infof("End pull image: %s", img)
	}

	logrus.Infof("Start prepare snapshot %s", meta.ID)
	_, err := p.containerd.GetSnapshot(ctx, meta.ID)
	if err == nil {
		logrus.Infof("Snapshot %s already exists, delete it", meta.ID)
		p.containerd.RemoveSnapshot(ctx, meta.ID)
	}
	// CreateSnapshot for new pouch container
	if err := p.containerd.CreateSnapshot(ctx, meta.ID, img); err != nil {
		return err
	}

	upperDir, workDir, err := p.getOverlayFsDir(ctx, meta.ID)
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
func (p *PouchMigrator) Migrate(ctx context.Context, takeOverContainer bool) error {

	// Copy network db file
	dbName := "local-kv.db"
	srcNetDBFile := path.Join(p.dockerHomeDir, "network/files", dbName)

	dstNetDBDir := path.Join(p.pouchHomeDir, "network/files")
	dstNetDBFile := path.Join(dstNetDBDir, dbName)
	if _, err := os.Stat(srcNetDBFile); err != nil {
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

	if err := utils.ExecCommand("cp", srcNetDBFile, dstNetDBDir); err != nil {
		return fmt.Errorf("failed to prepare network db file: %v", err)
	}

	// Stop all running containers
	timeout := time.Duration(1) * time.Second
	if !takeOverContainer {
		for _, c := range p.runningContainers {
			logrus.Infof("Start stop container %s", c)
			if err := p.dockerd.ContainerStop(c, &timeout); err != nil {
				if !dockerCli.IsErrNotFound(err) {
					return fmt.Errorf("failed to stop container: %v", err)
				}
			}
		}
	}

	// Only mv stopped containers' upperDir
	// mv oldUpperDir/* newUpperDir/
	for _, dirMapping := range p.upperDirMappingList {
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
func (p *PouchMigrator) PostMigrate(ctx context.Context, takeOverContainer bool) error {
	// stop containerd instance
	p.containerd.Cleanup()

	// Get all docker containers on host again,
	// In case, there will have containers be deleted
	// Notes: we will lock host first, so there will have no
	// new containers created
	containers, err := p.dockerd.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	for _, c := range containers {
		if _, exists := p.allContainers[c.ID]; exists {
			p.allContainers[c.ID] = true
		}
	}

	deletedContainers := []string{}
	for id, exists := range p.allContainers {
		if !exists {
			deletedContainers = append(deletedContainers, id)
		}
	}

	// Uninstall docker
	// TODO backup two config files: /etc/sysconfig/docker, /etc/docker/daemon.jon
	// In case we revert migration.
	for _, f := range []string{"/etc/sysconfig/docker", "/etc/docker/daemon.json"} {
		if err := utils.ExecCommand("cp", f, f+".bk"); err != nil {
			return err
		}
	}

	// We must first stop the docker before remove it
	logrus.Infof("Start to stop docker: %s", p.dockerPkg)
	stopDocker := make(chan error, 1)
	go func() {
		var err error
		for i := 0; i < 3; i++ {
			err = utils.ExecCommand("systemctl", "stop", "docker")
			if err == nil {
				break
			}

			logrus.Errorf("failed to stop docker service: %v", err)
		}

		stopDocker <- err
	}()

	if err := <-stopDocker; err != nil {
		return fmt.Errorf("failed to stop docker: %v", err)
	}

	// if dryRun set or take over old container, just test the code, not remove the package
	if !p.dryRun && !takeOverContainer {
		// Change docker bridge mode to nat mode
		logrus.Infof("Start change docker net mode from bridge to nat")
		if err := utils.ExecCommand("setup-bridge", "stop"); err != nil {
			logrus.Errorf("failed to stop bridge nat: %v", err)
		}
		if err := utils.ExecCommand("setup-bridge", "nat"); err != nil {
			logrus.Errorf("failed to set nat mode: %v", err)
		}
	}

	if !p.dryRun {
		// Remove docker
		logrus.Infof("Start to uninstall docker: %s", p.dockerPkg)
		if err := utils.ExecCommand("yum", "remove", "-y", p.dockerPkg); err != nil {
			return fmt.Errorf("failed to uninstall docker: %v", err)
		}
	}

	// export NO_NAT=yes
	if err := utils.ExecCommand("export", "NO_NAT=yes"); err != nil {
		logrus.Errorf("failed export env: %v", err)
	}

	// Install pouch
	logrus.Infof("Start install pouch: %s", p.pouchPkgPath)
	if err := utils.ExecCommand("yum", "install", "-y", p.pouchPkgPath); err != nil {
		logrus.Errorf("failed to install pouch: %v", err)
		return err
	}

	// Starting wait pouchd to serve
	check := make(chan struct{})
	timeout := make(chan bool, 1)
	// set timeout to wait pouchd start
	go func() {
		time.Sleep(60 * time.Second)
		timeout <- true
	}()

	// check whether pouchd starts success
	go func() {
		for {
			_, err := net.Dial("unix", "/var/run/pouchd.sock")
			if err == nil {
				check <- struct{}{}
			}
		}
	}()

	select {
	case <-check:
		// pouchd has started
	case <-timeout:
		return fmt.Errorf("failed to wait pouchd start, timeout")
	}

	// TODO should specify pouchd socket path
	pouchCli, err := pouch.NewPouchClient("")
	if err != nil {
		logrus.Errorf("failed to create a pouch client: %v, need start container by manual", err)
		return err
	}

	logrus.Infof("Has %d containers being deleted", len(deletedContainers))
	for _, c := range deletedContainers {
		if err := pouchCli.ContainerRemove(ctx, c, &pouchtypes.ContainerRemoveOptions{Force: true}); err != nil {
			if !strings.Contains(err.Error(), "not found") {
				return err
			}
		}
	}

	if !takeOverContainer {
		// after start pouch we should clean docker0 bridge, if not take over
		// old containers
		if err := utils.ExecCommand("ip", "link", "del", "docker0"); err != nil {
			logrus.Errorf("failed to delete docker0 bridge: %v", err)
		}

		// Start all containers need being running
		for _, c := range p.runningContainers {
			if utils.StringInSlice(deletedContainers, c) {
				continue
			}

			logrus.Infof("Start starting container %s", c)
			if err := pouchCli.ContainerStart(ctx, c, ""); err != nil {
				logrus.Errorf("failed to start container %s: %v", c, err)
				return err
			}
		}
	}

	logrus.Info("PostMigrate done!!!")
	return nil
}

// RevertMigration reverts migration.
func (p *PouchMigrator) RevertMigration(ctx context.Context, takeOverContainer bool) error {
	// Then, move all upperDir back
	for _, dirMapping := range p.upperDirMappingList {
		if err := utils.MoveDir(dirMapping.dstDir, dirMapping.srcDir); err != nil {
			logrus.Errorf("%v\n", err)

			return err
		}
	}

	if !takeOverContainer {
		// Start all running containers
		for _, c := range p.runningContainers {
			if err := p.dockerd.ContainerStart(c); err != nil {
				return fmt.Errorf("failed start container: %v", err)
			}
		}
	}

	return nil
}

// Cleanup does some clean works when migrator exited.
func (p *PouchMigrator) Cleanup() error {
	return p.containerd.Cleanup()
}

// PrepareImages just pull images for containers
func (p *PouchMigrator) PrepareImages(ctx context.Context) error {
	// Get all docker containers on host.
	containers, err := p.dockerd.ContainerList()
	if err != nil {
		return fmt.Errorf("failed to get containers list: %v", err)
	}
	logrus.Debugf("Get %d containers", len(containers))

	for _, c := range containers {
		meta, err := p.dockerd.ContainerInspect(c.ID)
		if err != nil {
			logrus.Errorf("failed to inspect container %s: %v", c.ID, err)
			continue
		}

		image, err := p.dockerd.ImageInspect(meta.Image)
		if err != nil {
			logrus.Errorf("failed to inspect image %s: %v", meta.Image, err)
			continue
		}

		var imageName string
		if len(image.RepoTags) > 0 {
			imageName = image.RepoTags[0]
		} else if len(image.RepoDigests) > 0 {
			imageName = image.RepoDigests[0]
		} else {
			logrus.Errorf("failed to get image %s: repoTags is empty", meta.Image)
			continue
		}

		// check image existance
		_, err = p.containerd.GetImage(ctx, imageName)
		if err == nil {
			logrus.Infof("image %s has been downloaded, skip pull image", imageName)
		} else {
			logrus.Infof("Start pull image: %s", imageName)
			if err := p.containerd.PullImage(ctx, imageName); err != nil {
				logrus.Errorf("failed to pull image %s: %v", imageName, err)
				continue
			}
			logrus.Infof("End pull image: %s", imageName)
		}
	}

	return nil
}
