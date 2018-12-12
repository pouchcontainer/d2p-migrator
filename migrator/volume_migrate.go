package migrator

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/pouchcontainer/d2p-migrator/docker"
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"

	"github.com/alibaba/pouch/pkg/meta"
	volumetypes "github.com/alibaba/pouch/storage/volume/types"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/sirupsen/logrus"
)

// PrepareVolumes put volumes info into pouch volume store.
func PrepareVolumes(homeDir string, volumes []*volumetypes.Volume, volumeRefs map[string]string) error {
	// init a docker client
	dockerCli, err := docker.NewDockerd()
	if err != nil {
		return err
	}

	store, err := newVolumeStore(path.Join(homeDir, "volume"))
	if err != nil {
		return err
	}

	// need to close the boltdb after created volumes,
	// otherwise, pouchd cannot start because the volume
	// store initialize failed
	defer store.Shutdown()

	// update volumes references
	for _, vol := range volumes {
		refs, ok := volumeRefs[vol.Name]
		if !ok || refs == "" {
			continue
		}
		vol.Spec.Extra["ref"] = refs

		// since docker volume list api not return alilocal size,
		// we should use volume inspect api.
		if vol.Driver() == "alilocal" {
			volume, err := dockerCli.VolumeInspect(vol.Name)
			if err != nil {
				logrus.Errorf("failed to inspect volume %s: %v", vol.Name, err)
				continue
			}
			vol.Spec.Size = getVolumeSize(volume)
		}
	}

	return store.CreateVolumes(volumes)
}

// volumeStore is a store of volume
type volumeStore struct {
	baseDir string
	store   *meta.Store
}

// newVolumeStore initializes a boltdb store for volume store.
func newVolumeStore(baseDir string) (*volumeStore, error) {
	// prepare volume dir if not exist
	if _, err := os.Stat(baseDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(baseDir, 0666); err != nil {
			return nil, fmt.Errorf("failed to prepare volume store dir %s: %v", baseDir, err)
		}
	}

	boltdbCfg := meta.Config{
		Driver:  "boltdb",
		BaseDir: path.Join(baseDir, "volume.db"),
		Buckets: []meta.Bucket{
			{
				Name: "volume",
				Type: reflect.TypeOf(volumetypes.Volume{}),
			},
		},
	}
	boltStore, err := meta.NewStore(boltdbCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize a new boltdb store: %v", err)
	}

	return &volumeStore{baseDir: baseDir, store: boltStore}, nil
}

// CreateVolumes put all volumes information to volume boltdb
func (s *volumeStore) CreateVolumes(volumes []*volumetypes.Volume) error {
	for _, vol := range volumes {
		if err := s.store.Put(vol); err != nil {
			return fmt.Errorf("failed to create volume %s: %v", vol.Name, err)
		}
	}

	return nil
}

// Shutdown close the store's boltdb
func (s *volumeStore) Shutdown() error {
	return s.store.Shutdown()
}

// ContainerVolumeRefsCount count a container's reference to volumes
func ContainerVolumeRefsCount(c *localtypes.Container, volumeRefs map[string]string) error {
	for _, mount := range c.Mounts {
		if mount.Driver == "" {
			continue
		}

		refs, ok := volumeRefs[mount.Name]
		if !ok || refs == "" {
			volumeRefs[mount.Name] = c.ID
		} else if !strings.Contains(refs, c.ID) {
			volumeRefs[mount.Name] = strings.Join([]string{refs, c.ID}, ",")
		}
	}

	return nil
}

func getVolumeSize(volume dockertypes.Volume) string {
	// get volume size
	var (
		optSize interface{}
		size    string
	)

	for _, k := range []string{"size", "opt.size", "Size", "opt.Size"} {
		var ok bool
		optSize, ok = volume.Status[k]
		if ok {
			fmt.Printf("get volume %s size %s\n", volume.Name, optSize.(string))
			break
		}
	}

	if optSize != nil {
		size = optSize.(string)
	}

	return size
}
