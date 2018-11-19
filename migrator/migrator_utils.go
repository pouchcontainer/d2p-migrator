package migrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pouchcontainer/d2p-migrator/pouch"
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
	"github.com/pouchcontainer/d2p-migrator/utils"

	"github.com/alibaba/pouch/storage/quota"
	dockertypes "github.com/docker/engine-api/types"
)

// save2Disk save container metadata file to disk
func save2Disk(homeDir string, meta *localtypes.Container) error {
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

// SetDirDiskQuota set disk quota for the dir.
func SetDirDiskQuota(defaultQuota, quotaID, dir string) error {
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

	return filepath.Walk(dir, qotaSetFunc)
}

func migrateNetworkFile(dockerHomeDir, pouchHomeDir string) error {
	// Copy network db file
	var (
		dbName    = "local-kv.db"
		dbFile    = path.Join(dockerHomeDir, "network", "files", dbName)
		newDBDir  = path.Join(pouchHomeDir, "network", "files")
		newDBFile = path.Join(newDBDir, dbName)
	)

	if _, err := os.Stat(dbFile); err != nil {
		return err
	}

	// prepare network directory
	if _, err := os.Stat(newDBDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(newDBDir, 0666); err != nil {
			return fmt.Errorf("failed to prepare network dir: %v", err)
		}
	}

	// clear the network directory, in case dirty data remained.
	// should never occur errors here
	if err := os.RemoveAll(newDBFile); err != nil {
		return fmt.Errorf("failed to delete old network db file: %v", err)
	}

	if err := utils.ExecCommand("cp", dbFile, newDBDir); err != nil {
		return fmt.Errorf("failed to prepare network db file: %v", err)
	}

	return nil
}

func prepareConfigForPouch(homeDir string) error {
	if _, err := os.Stat(homeDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(homeDir, 0666); err != nil {
			return fmt.Errorf("failed to mkdir: %v", err)
		}
	}

	// Change pouch config file
	if err := pouch.ChangeHomeDir(homeDir); err != nil {
		return fmt.Errorf("failed to change the pouchd home dir: %v", err)
	}

	return nil
}

// validateDocker check if the docker can migration
func validateDocker(info dockertypes.Info) error {
	// Check if we can migrate docker to pouch
	if info.DockerRootDir == "" {
		return fmt.Errorf("docker root dir is empty")
	}

	// if storage driver is not overlay, cannot do migration
	if info.Driver != "overlay" && info.Driver != "overlay2" {
		return fmt.Errorf("d2p-migrator only support overlayfs Storage Driver")
	}

	return nil
}

func getPouchHomeDir(dockerHomeDir string) string {
	rootDir := strings.TrimSuffix(dockerHomeDir, "docker")
	return path.Join(rootDir, "pouch")
}
