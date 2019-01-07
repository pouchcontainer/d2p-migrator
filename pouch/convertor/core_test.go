package convertor

import (
	"reflect"
	"testing"

	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/blkiodev"
	containertypes "github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
)

// TestToPouchContainerMeta is a test case for convert docker ContainerJSON to
// Pouch ContainerJSON.
// We should check gpu, distribution storage volume, blkio and so on.
func TestToPouchContainerMeta(t *testing.T) {
	containerJSON := &dockertypes.ContainerJSON{
		ContainerJSONBase: &dockertypes.ContainerJSONBase{
			AppArmorProfile: "",
			Args:            []string{},
			Created:         "2018-12-18T21:36:18.054277464+08:00",
			Driver:          "overlay2",
			GraphDriver: dockertypes.GraphDriverData{
				Name: "overlay2",
				Data: map[string]string{
					"LowerDir":  "/var/lib/docker/overlay2/42db2c7cf72c009f478780e08b7c3486e625d69dafe504e956e39dc600ec4ec4/diff",
					"MergedDir": "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/merged",
					"UpperDir":  "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/diff",
					"WorkDir":   "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/work",
				},
			},
			HostnamePath:   "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/hostname",
			HostsPath:      "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/hosts",
			ID:             "864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e",
			Image:          "sha256:82f911de338a57eaac9eb35f3ce7c794fed5e1e9c29f68a656d22ebb875f43e1",
			LogPath:        "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e-json.log",
			MountLabel:     "",
			Name:           "65193f33-ffef-453c-a2a4-dce94fb4240d011001067137",
			Path:           "/home/admin/start.sh",
			ProcessLabel:   "",
			ResolvConfPath: "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/resolv.conf",
			RestartCount:   3,
			SizeRootFs:     nil,
			SizeRw:         nil,
			State:          &dockertypes.ContainerState{},
			HostConfig: &containertypes.HostConfig{
				AutoRemove:      false,
				Binds:           []string{"testUltron:/var/logs", "/opt/route.tmpl:/etc/route.tmpl:ro"},
				CapAdd:          []string{"SYS_RESOURCE", "SYS_MODULE", "SYS_PTRACE", "SYS_PACCT", "NET_ADMIN", "SYS_ADMIN"},
				CapDrop:         nil,
				Cgroup:          "",
				ConsoleSize:     [2]int{0, 0},
				ContainerIDFile: "",
				DNS:             []string{},
				DNSOptions:      []string{},
				DNSSearch:       []string{},
				ExtraHosts:      nil,
				GroupAdd:        nil,
				IpcMode:         "",
				Isolation:       "",
				LogConfig: containertypes.LogConfig{
					Type:   "json-file",
					Config: nil,
				},
				NetworkMode:     "docker0_11.1.67.253.overlay",
				OomScoreAdj:     0,
				PidMode:         "",
				PortBindings:    nil,
				Privileged:      false,
				PublishAllPorts: false,
				ReadonlyRootfs:  false,
				RestartPolicy: containertypes.RestartPolicy{
					Name:              "always",
					MaximumRetryCount: 3,
				},
				Runtime:      "runc",
				SecurityOpt:  nil,
				ShmSize:      67108864,
				StorageOpt:   nil,
				Sysctls:      nil,
				Tmpfs:        nil,
				UTSMode:      "",
				UsernsMode:   "",
				VolumeDriver: "",
				VolumesFrom:  nil,
				Resources: containertypes.Resources{
					BlkioDeviceReadBps: []*blkiodev.ThrottleDevice{
						{
							Path: "/dev/vrbd6",
							Rate: 0,
						},
					},
					BlkioDeviceReadIOps: []*blkiodev.ThrottleDevice{
						{
							Path: "/dev/vrbd6",
							Rate: 0,
						},
					},
					BlkioDeviceWriteBps: []*blkiodev.ThrottleDevice{
						{
							Path: "/dev/vrbd6",
							Rate: 0,
						},
					},
					BlkioDeviceWriteIOps: []*blkiodev.ThrottleDevice{
						{
							Path: "/dev/vrbd6",
							Rate: 0,
						},
					},
					BlkioWeight:       0,
					BlkioWeightDevice: nil,
					CPUPeriod:         0,
					CPUQuota:          -1,
					CPUShares:         4096,
					CgroupParent:      "",
					CPUCount:          0,
					CPUPercent:        0,
					CpusetCpus:        "16,17,18,19",
					CpusetMems:        "",
					Devices: []containertypes.DeviceMapping{
						{
							PathOnHost:        "/dev/vrbd6",
							PathInContainer:   "/dev/vrbd6",
							CgroupPermissions: "rwm",
						},
					},
					IOMaximumBandwidth: 0,
					IOMaximumIOps:      0,
					KernelMemory:       0,
					Memory:             8589934592,
					MemoryReservation:  0,
					MemorySwap:         17179869184,
					MemorySwappiness:   int64Ptr(-1),
					PidsLimit:          0,
					Ulimits:            nil,
				},
			},
		},
		Config: &containertypes.Config{
			AttachStderr: false,
			AttachStdin:  false,
			AttachStdout: false,
			Cmd:          nil,
			Domainname:   "",
			Entrypoint:   []string{"/home/admin/start.sh"},
			Env:          []string{"test=foo", "type=container"},
			Hostname:     "test",
			Image:        "registry.hub.docker.com/library/busybox:latest",
			Labels:       map[string]string{"type": "test"},
			OnBuild:      nil,
			OpenStdin:    false,
			StdinOnce:    false,
			User:         "root",
			Tty:          false,
			WorkingDir:   "/home/admin/bin",
			Volumes: map[string]struct{}{
				"/home/admin/logs": {},
			},
		},
		Mounts: []dockertypes.MountPoint{
			{
				Source:      "/opt/route.tmpl",
				Destination: "/etc/route.tmpl",
				Mode:        "ro",
				RW:          false,
			},
			{
				Name:        "testUltron",
				Source:      "/mnt/nas/testUltron/home/admin/logs",
				Destination: "/var/logs",
				Driver:      "ultron",
				Mode:        "",
				RW:          true,
				Propagation: "rprivate",
			},
		},
		NetworkSettings: &dockertypes.NetworkSettings{
			NetworkSettingsBase: dockertypes.NetworkSettingsBase{
				Bridge:                 "",
				SandboxID:              "3e248571fe8fac159402db7d51dbe29e8ed8676143c9c7f1ddbca1d7982c54b9",
				HairpinMode:            false,
				LinkLocalIPv6Address:   "",
				LinkLocalIPv6PrefixLen: 0,
				Ports:                  nil,
				SandboxKey:             "/var/run/docker/netns/3e248571fe8f",
				SecondaryIPAddresses:   nil,
				SecondaryIPv6Addresses: nil,
			},
			DefaultNetworkSettings: dockertypes.DefaultNetworkSettings{},
			Networks: map[string]*network.EndpointSettings{
				"docker0_11.1.67.253.overlay": {
					IPAMConfig: &network.EndpointIPAMConfig{
						IPv4Address: "11.1.67.137",
					},
					NetworkID:   "37c66d262cd1d68ce45c6659d9ff8e102c6d926d5998d1e32afed5818442ae3e",
					EndpointID:  "876856d0102f80c21b55a0cf1510c3b81ef368aead954a378dc595ccc887db66",
					Gateway:     "11.1.67.253",
					IPAddress:   "11.1.67.137",
					IPPrefixLen: 22,
					IPv6Gateway: "",
					MacAddress:  "02:42:0b:01:43:89",
				},
			},
		},
	}

	wantContainerJSON := &localtypes.Container{
		AppArmorProfile: "",
		SeccompProfile:  "",
		NoNewPrivileges: false,
		Args:            []string{},
		Config: &pouchtypes.ContainerConfig{
			AttachStderr: false,
			AttachStdin:  false,
			AttachStdout: false,
			Cmd:          nil,
			Domainname:   "",
			Entrypoint:   []string{"/home/admin/start.sh"},
			Env:          []string{"test=foo", "type=container"},
			Hostname:     "test",
			Image:        "registry.hub.docker.com/library/busybox:latest",
			Labels:       map[string]string{"type": "test"},
			OnBuild:      nil,
			OpenStdin:    false,
			StdinOnce:    false,
			User:         "root",
			Tty:          false,
			WorkingDir:   "/home/admin/bin",
			Volumes: map[string]interface{}{
				"/home/admin/logs": struct{}{},
			},
		},
		Created: "2018-12-18T21:36:18.054277464+08:00",
		Driver:  "overlay2",
		ExecIds: "",
		Snapshotter: &pouchtypes.SnapshotterData{
			Name: "overlayfs",
			Data: map[string]string{
				"LowerDir":  "/var/lib/docker/overlay2/42db2c7cf72c009f478780e08b7c3486e625d69dafe504e956e39dc600ec4ec4/diff",
				"MergedDir": "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/merged",
				"UpperDir":  "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/diff",
				"WorkDir":   "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/work",
			},
		},
		HostConfig: &pouchtypes.HostConfig{
			AutoRemove: false,
			Binds:      []string{"testUltron:/var/logs", "/opt/route.tmpl:/etc/route.tmpl:ro"},
			CapAdd:     []string{"SYS_RESOURCE", "SYS_MODULE", "SYS_PTRACE", "SYS_PACCT", "NET_ADMIN", "SYS_ADMIN"},
			CapDrop:    nil,
			Cgroup:     "",
			//ConsoleSize:     []*int64{nil, nil},
			ContainerIDFile: "",
			DNS:             []string{},
			DNSOptions:      []string{},
			DNSSearch:       []string{},
			ExtraHosts:      nil,
			GroupAdd:        nil,
			IpcMode:         "",
			Isolation:       "",
			LogConfig: &pouchtypes.LogConfig{
				LogDriver: "json-file",
				LogOpts:   nil,
			},
			NetworkMode:     "docker0_11.1.67.253.overlay",
			OomScoreAdj:     0,
			PidMode:         "",
			PortBindings:    nil,
			Privileged:      false,
			PublishAllPorts: false,
			ReadonlyRootfs:  false,
			RestartPolicy: &pouchtypes.RestartPolicy{
				Name:              "always",
				MaximumRetryCount: 3,
			},
			Runtime:      "runc",
			SecurityOpt:  nil,
			ShmSize:      int64Ptr(67108864),
			StorageOpt:   nil,
			Sysctls:      nil,
			Tmpfs:        nil,
			UTSMode:      "",
			UsernsMode:   "",
			VolumeDriver: "",
			VolumesFrom:  nil,
			Resources: pouchtypes.Resources{
				BlkioDeviceReadBps: []*pouchtypes.ThrottleDevice{
					{
						Path: "/dev/vrbd6",
						Rate: 0,
					},
				},
				BlkioDeviceReadIOps: []*pouchtypes.ThrottleDevice{
					{
						Path: "/dev/vrbd6",
						Rate: 0,
					},
				},
				BlkioDeviceWriteBps: []*pouchtypes.ThrottleDevice{
					{
						Path: "/dev/vrbd6",
						Rate: 0,
					},
				},
				BlkioDeviceWriteIOps: []*pouchtypes.ThrottleDevice{
					{
						Path: "/dev/vrbd6",
						Rate: 0,
					},
				},
				BlkioWeight:       0,
				BlkioWeightDevice: nil,
				CPUPeriod:         0,
				CPUQuota:          -1,
				CPUShares:         4096,
				CgroupParent:      "",
				CPUCount:          0,
				CPUPercent:        0,
				CpusetCpus:        "16,17,18,19",
				CpusetMems:        "",
				Devices: []*pouchtypes.DeviceMapping{
					{
						PathOnHost:        "/dev/vrbd6",
						PathInContainer:   "/dev/vrbd6",
						CgroupPermissions: "rwm",
					},
				},
				IOMaximumBandwidth:  0,
				IOMaximumIOps:       0,
				KernelMemory:        0,
				Memory:              8589934592,
				MemoryExtra:         nil,
				MemoryForceEmptyCtl: 0,
				MemoryReservation:   0,
				MemorySwap:          17179869184,
				MemorySwappiness:    int64Ptr(-1),
				MemoryWmarkRatio:    nil,
				OomKillDisable:      nil,
				PidsLimit:           0,
				Ulimits:             nil,
			},
		},
		HostnamePath: "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/hostname",
		HostsPath:    "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/hosts",
		ID:           "864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e",
		Image:        "sha256:82f911de338a57eaac9eb35f3ce7c794fed5e1e9c29f68a656d22ebb875f43e1",
		LogPath:      "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e-json.log",
		MountLabel:   "",
		Mounts: []*pouchtypes.MountPoint{
			{
				Source:      "/opt/route.tmpl",
				Destination: "/etc/route.tmpl",
				Mode:        "ro",
				RW:          false,
			},
			{
				Name:        "testUltron",
				Source:      "/mnt/nas/testUltron/home/admin/logs",
				Destination: "/var/logs",
				Driver:      "ultron",
				Mode:        "",
				RW:          true,
				Propagation: "rprivate",
				Named:       true,
			},
		},
		Name: "65193f33-ffef-453c-a2a4-dce94fb4240d011001067137",
		NetworkSettings: &pouchtypes.NetworkSettings{
			Bridge:                 "",
			HairpinMode:            false,
			LinkLocalIPV6Address:   "",
			LinkLocalIPV6PrefixLen: 0,
			Networks: map[string]*pouchtypes.EndpointSettings{
				"docker0_11.1.67.253.overlay": {
					EndpointID:  "876856d0102f80c21b55a0cf1510c3b81ef368aead954a378dc595ccc887db66",
					Gateway:     "11.1.67.253",
					IPAddress:   "11.1.67.137",
					IPPrefixLen: 22,
					IPV6Gateway: "",
					MacAddress:  "02:42:0b:01:43:89",
					NetworkID:   "37c66d262cd1d68ce45c6659d9ff8e102c6d926d5998d1e32afed5818442ae3e",
					IPAMConfig: &pouchtypes.EndpointIPAMConfig{
						IPV4Address: "11.1.67.137",
					},
				},
			},
			Ports:                  nil,
			SandboxID:              "3e248571fe8fac159402db7d51dbe29e8ed8676143c9c7f1ddbca1d7982c54b9",
			SandboxKey:             "/var/run/docker/netns/3e248571fe8f",
			SecondaryIPAddresses:   nil,
			SecondaryIPV6Addresses: nil,
		},
		Path:           "/home/admin/start.sh",
		ProcessLabel:   "",
		ResolvConfPath: "/var/lib/docker/containers/864bfda43118ba323675956faec5f67a40060730f8cc8a4ee2b572e9f460023e/resolv.conf",
		RestartCount:   int64(3),
		SizeRootFs:     int64(0),
		SizeRw:         int64(0),
		State:          &pouchtypes.ContainerState{},
		BaseFS:         "/var/lib/docker/overlay2/bd94ada96dca4181a0bb81330191aac573c35c76084c03518192a8edf8e79d71/merged",
		DetachKeys:     "",
		RootFSProvided: true,
	}

	gotContainerJSON, err := ToPouchContainerMeta(containerJSON)
	if err != nil {
		t.Errorf("failed to convert docker container json to pouch container json: %v", err)
	}

	// check ContainerJSON
	if !reflect.DeepEqual(wantContainerJSON, gotContainerJSON) {
		t.Errorf("ToPouchContainerMeta got wrong result: want %#v, got %#v", wantContainerJSON, gotContainerJSON)
	}
}

func int64Ptr(i int) *int64 { u := int64(i); return &u }
