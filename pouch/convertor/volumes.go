package convertor

import (
	"fmt"
	"time"

	"github.com/pouchcontainer/d2p-migrator/utils"

	volumetypes "github.com/alibaba/pouch/storage/volume/types"
	volumemeta "github.com/alibaba/pouch/storage/volume/types/meta"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/pborman/uuid"
)

var (
	// SupportVolumeDrivers specify volume drivers d2p supported
	SupportVolumeDrivers = []string{"ultron", "local", "alilocal"}
)

// ToVolume converts docker volume information to pouch volume
func ToVolume(vol *dockertypes.Volume) (*volumetypes.Volume, error) {
	fmt.Printf("Start convert docker volume %+v\n", vol)
	now := time.Now()
	return &volumetypes.Volume{
		ObjectMeta: volumemeta.ObjectMeta{
			Name:              vol.Name,
			Claimer:           "pouch",
			Namespace:         "pouch",
			UID:               uuid.NewRandom().String(),
			Generation:        volumemeta.ObjectPhasePreCreate,
			Labels:            vol.Labels,
			CreationTimestamp: &now,
			ModifyTimestamp:   &now,
		},
		Spec: &volumetypes.VolumeSpec{
			Backend: vol.Driver,
			Extra: map[string]string{
				"mount": vol.Mountpoint,
			},
			Selector: make(volumetypes.Selector, 0),
			VolumeConfig: &volumetypes.VolumeConfig{
				Size: "",
			},
		},
		Status: &volumetypes.VolumeStatus{
			MountPoint: vol.Mountpoint,
		},
	}, nil
}

// ToVolumes convert docker volume slice to pouch volume slice.
func ToVolumes(volumes []*dockertypes.Volume) ([]*volumetypes.Volume, error) {
	pouchVolumes := []*volumetypes.Volume{}
	// check volume's driver, now only support local driver
	for _, vol := range volumes {
		if !utils.StringInSlice(SupportVolumeDrivers, vol.Driver) {
			return nil, fmt.Errorf("not support volume driver %s", vol.Driver)
		}

		// no need create remote disk
		if vol.Driver == "ultron" {
			continue
		}
		newVol, err := ToVolume(vol)
		if err != nil {
			return nil, fmt.Errorf("failed to convert volume %s: %v", vol.Name, err)
		}

		pouchVolumes = append(pouchVolumes, newVol)
	}

	return pouchVolumes, nil
}
