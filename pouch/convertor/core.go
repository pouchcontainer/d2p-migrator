package convertor

import (
	"fmt"
	"strings"
	"time"

	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
	"github.com/pouchcontainer/d2p-migrator/utils"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/cri/annotations"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/blkiodev"
	containertypes "github.com/docker/engine-api/types/container"
	networktypes "github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/go-openapi/strfmt"
)

// ToPouchContainerMeta coverts docker container config to pouch container config.
func ToPouchContainerMeta(meta *dockertypes.ContainerJSON) (*localtypes.Container, error) {
	if meta == nil {
		return nil, nil
	}

	// Convert Base Parameters
	pouchMeta := &localtypes.Container{
		// SeccompProfile
		// NoNewPrivileges
		// TODO: should change ExecIds to ExecIDs
		// ExecIds:         meta.ExecIDs,
		// DetachKeys
		// Node
		AppArmorProfile: meta.AppArmorProfile,
		Args:            meta.Args,
		Created:         convertTime(meta.Created),
		// TODO: default convert overlay2 container
		Driver:         "overlay2",
		HostnamePath:   meta.HostnamePath,
		HostsPath:      meta.HostsPath,
		ResolvConfPath: meta.ResolvConfPath,
		ID:             meta.ID,
		Image:          meta.Image,
		LogPath:        meta.LogPath,
		MountLabel:     meta.MountLabel,
		Path:           meta.Path,
		ProcessLabel:   meta.ProcessLabel,
		RestartCount:   int64(meta.RestartCount),
		RootFSProvided: true,
	}

	// Name
	pouchMeta.Name = strings.TrimPrefix(meta.Name, "/")

	// BaseFS
	pouchMeta.BaseFS = meta.GraphDriver.Data["MergedDir"]

	// SizeRootFs, SizeRw
	if meta.SizeRootFs != nil {
		pouchMeta.SizeRootFs = *meta.SizeRootFs
	}
	if meta.SizeRw != nil {
		pouchMeta.SizeRw = *meta.SizeRw
	}

	// Snapshotter
	// GraphDriver
	pouchMeta.Snapshotter = &pouchtypes.SnapshotterData{
		Name: "overlayfs",
		Data: meta.GraphDriver.Data,
	}

	// State: converted container status
	pouchMeta.State = &pouchtypes.ContainerState{
		Dead:       meta.State.Dead,
		Error:      meta.State.Error,
		ExitCode:   int64(meta.State.ExitCode),
		FinishedAt: convertTime(meta.State.FinishedAt),
		OOMKilled:  false,
		Paused:     meta.State.Paused,
		Pid:        int64(meta.State.Pid),
		Restarting: false,
		Running:    meta.State.Running,
		StartedAt:  convertTime(meta.State.StartedAt),
		Status:     toContainerStatus(meta.State.Status),
	}

	// Config
	config, err := toContainerConfig(meta.Config)
	if err != nil || config == nil {
		if err == nil {
			err = fmt.Errorf("got an empty ContainerConfig")
		}
		return nil, err
	}

	// check if the container is a sandbox
	if SandboxNameRegex.Match([]byte(pouchMeta.Name)) {
		config.Labels[PouchContainerTypeLabelKey] = PouchContainerTypeLabelSandbox
	}

	// check if the container is a container of sandbox
	containerType, ok := config.Labels[DockerContainerTypeLabelKey]
	if ok && containerType == PouchContainerTypeLabelContainer {
		// completion CRI labels and annotaions for container of sandbox
		config.Labels[PouchContainerTypeLabelKey] = PouchContainerTypeLabelContainer

		if config.SpecAnnotation == nil {
			config.SpecAnnotation = map[string]string{}
			config.SpecAnnotation[annotations.ContainerType] = annotations.ContainerTypeContainer
		}

		if v, ok := config.Labels[SandboxIDLabelKey]; ok {
			config.SpecAnnotation[annotations.SandboxName] = v
			config.SpecAnnotation[annotations.SandboxID] = v
		}
	}
	pouchMeta.Config = config

	envs, err := parseEnv(config.Env)
	if err != nil {
		return nil, err
	}

	// Only works for alidocker
	if mode, exists := envs["ali_run_mode"]; exists {
		if mode != "vm" {
			oldRunMode := fmt.Sprintf("ali_run_mode=%s", mode)
			newRunMode := "ali_run_mode=vm"

			for i, v := range config.Env {
				if v == oldRunMode {
					config.Env[i] = newRunMode
					break
				}
			}
		}
	}

	// HostConfig
	hostconfig, err := toHostConfig(meta.HostConfig)
	if err != nil {
		return nil, err
	}

	// If the LogConfig.LogDriver is empty but the LogPath is not empty,
	// We should set the LogDriver to json-file
	if hostconfig.LogConfig.LogDriver == "" && pouchMeta.LogPath != "" {
		hostconfig.LogConfig.LogDriver = "json-file"
		// TODO: LogOpts passed but not used right now
	}

	// Mounts
	mountPoints, err := toMountPoints(meta.Mounts)
	if err != nil {
		return nil, err
	}
	pouchMeta.Mounts = mountPoints

	// TODO: Only works in alidocker
	ldBind := ""
	if path, exists := envs["LD_PRELOAD"]; exists {
		if path != "" {
			ldBind = fmt.Sprintf("%s:%s:ro", path, path)
		}
	}
	if ldBind != "" && !utils.StringInSlice(hostconfig.Binds, ldBind) {
		hostconfig.Binds = append(hostconfig.Binds, ldBind)
	}
	pouchMeta.HostConfig = hostconfig

	// NetworkSettings
	networks, err := toNetworkSettings(meta.NetworkSettings)
	if err != nil {
		return nil, err
	}
	pouchMeta.NetworkSettings = networks

	return pouchMeta, nil
}

func toContainerStatus(status string) pouchtypes.Status {
	var containerStatus pouchtypes.Status

	switch status {
	case "running":
		containerStatus = pouchtypes.StatusRunning
	case "exited":
		containerStatus = pouchtypes.StatusStopped
	case "created":
		containerStatus = pouchtypes.StatusCreated
	case "paused":
		containerStatus = pouchtypes.StatusPaused
	case "dead":
		containerStatus = pouchtypes.StatusDead
	}

	return containerStatus
}

func toContainerConfig(config *containertypes.Config) (*pouchtypes.ContainerConfig, error) {
	if config == nil {
		return nil, nil
	}

	pouchConfig := &pouchtypes.ContainerConfig{
		Hostname:        strfmt.Hostname(config.Hostname),
		Domainname:      config.Domainname,
		User:            config.User,
		AttachStdin:     config.AttachStdin,
		AttachStdout:    config.AttachStdout,
		AttachStderr:    config.AttachStderr,
		Tty:             config.Tty,
		Cmd:             []string(config.Cmd),
		Entrypoint:      []string(config.Entrypoint),
		Env:             config.Env,
		Image:           config.Image,
		Labels:          config.Labels,
		MacAddress:      config.MacAddress,
		NetworkDisabled: config.NetworkDisabled,
		OnBuild:         config.OnBuild,
		OpenStdin:       config.OpenStdin,
		Shell:           []string(config.Shell),
		StopSignal:      config.StopSignal,
		WorkingDir:      config.WorkingDir,
	}

	// Convert *int to *int64
	// pouchConfig.StopTimeout = &(int64(*config.StopTimeout)),

	// ArgsEscaped
	// DiskQuota
	// ExposedPorts
	// InitScript

	// Volumes
	volumes := map[string]interface{}{}
	for vol := range config.Volumes {
		volumes[vol] = struct{}{}
	}
	pouchConfig.Volumes = volumes

	// initialize label if it is nil
	if pouchConfig.Labels == nil {
		pouchConfig.Labels = map[string]string{}
	}

	// QuotaID
	if v, exists := pouchConfig.Labels["QuotaId"]; exists {
		pouchConfig.QuotaID = v
	}
	// Rich
	// RichMode
	// SpecAnnotation
	// StdinOnce

	return pouchConfig, nil
}

func toHostConfig(hostconfig *containertypes.HostConfig) (*pouchtypes.HostConfig, error) {
	if hostconfig == nil {
		return nil, nil
	}

	pouchHostConfig := &pouchtypes.HostConfig{
		AutoRemove: hostconfig.AutoRemove,
		CapAdd:     []string(hostconfig.CapAdd),
		CapDrop:    []string(hostconfig.CapDrop),
		Cgroup:     string(hostconfig.Cgroup),
		// ConsoleSize:
		ContainerIDFile: hostconfig.ContainerIDFile,
		DNS:             hostconfig.DNS,
		DNSOptions:      hostconfig.DNSOptions,
		DNSSearch:       hostconfig.DNSSearch,
		// EnableLxcfs:
		ExtraHosts: hostconfig.ExtraHosts,
		GroupAdd:   hostconfig.GroupAdd,
		// InitScript
		IpcMode:   string(hostconfig.IpcMode),
		Isolation: string(hostconfig.Isolation),
		Links:     hostconfig.Links,
		LogConfig: &pouchtypes.LogConfig{
			LogDriver: hostconfig.LogConfig.Type,
			LogOpts:   hostconfig.LogConfig.Config,
		},
		NetworkMode: string(hostconfig.NetworkMode),
		OomScoreAdj: int64(hostconfig.OomScoreAdj),
		PidMode:     string(hostconfig.PidMode),
		// PortBinding
		Privileged:      hostconfig.Privileged,
		PublishAllPorts: hostconfig.PublishAllPorts,
		ReadonlyRootfs:  hostconfig.ReadonlyRootfs,
		// Rich
		// RichMode

		// Only support "runc" runtime
		Runtime:      "runc",
		SecurityOpt:  hostconfig.SecurityOpt,
		ShmSize:      &hostconfig.ShmSize,
		StorageOpt:   hostconfig.StorageOpt,
		Sysctls:      hostconfig.Sysctls,
		Tmpfs:        hostconfig.Tmpfs,
		UTSMode:      string(hostconfig.UTSMode),
		VolumeDriver: hostconfig.VolumeDriver,
		VolumesFrom:  nil,
		Binds:        hostconfig.Binds,
	}

	// add capbilities to containers
	for _, cap := range []string{"SYS_RESOURCE", "SYS_MODULE", "SYS_PTRACE", "SYS_PACCT", "NET_ADMIN", "SYS_ADMIN"} {
		if utils.StringInSlice(pouchHostConfig.CapAdd, cap) {
			continue
		}

		pouchHostConfig.CapAdd = append(pouchHostConfig.CapAdd, cap)
	}

	// RestartPolicy
	pouchHostConfig.RestartPolicy = &pouchtypes.RestartPolicy{
		Name:              hostconfig.RestartPolicy.Name,
		MaximumRetryCount: int64(hostconfig.RestartPolicy.MaximumRetryCount),
	}

	resources, err := toResources(hostconfig.Resources)
	if err != nil {
		return nil, err
	}
	pouchHostConfig.Resources = resources

	return pouchHostConfig, nil
}

func toResources(resources containertypes.Resources) (pouchtypes.Resources, error) {
	// BlkioWeightDevice
	weightDevs, err := toWeightDevice(resources.BlkioWeightDevice)
	if err != nil {
		return pouchtypes.Resources{}, fmt.Errorf("failed to convert WeightDevices: %v", err)
	}

	// BlkioDeviceReadBps
	// BlkioDeviceReadIOps
	// BlkioDeviceWriteBps
	// BlkioDeviceWriteIOps
	pouchBlkDevs := [][]*pouchtypes.ThrottleDevice{}
	for _, devs := range [][]*blkiodev.ThrottleDevice{
		resources.BlkioDeviceReadBps,
		resources.BlkioDeviceReadIOps,
		resources.BlkioDeviceWriteBps,
		resources.BlkioDeviceWriteIOps,
	} {
		convertBlkDevs, err := toThrottleDevices(devs)
		if err != nil {
			return pouchtypes.Resources{}, fmt.Errorf("failed to convert ThrottleDevices: %v", err)
		}

		pouchBlkDevs = append(pouchBlkDevs, convertBlkDevs)
	}

	// Devices
	pouchDevices, err := toDevices(resources.Devices)
	if err != nil {
		return pouchtypes.Resources{}, fmt.Errorf("failed to convert Devices: %v", err)
	}

	// Ulimits
	var ulimits []*pouchtypes.Ulimit
	if resources.Ulimits != nil {
		for _, u := range resources.Ulimits {
			ulimit := &pouchtypes.Ulimit{
				Hard: u.Hard,
				Name: u.Name,
				Soft: u.Soft,
			}

			ulimits = append(ulimits, ulimit)
		}
	}

	pouchResources := pouchtypes.Resources{
		BlkioDeviceReadBps:   pouchBlkDevs[0],
		BlkioDeviceReadIOps:  pouchBlkDevs[1],
		BlkioDeviceWriteBps:  pouchBlkDevs[2],
		BlkioDeviceWriteIOps: pouchBlkDevs[3],
		BlkioWeight:          resources.BlkioWeight,
		BlkioWeightDevice:    weightDevs,
		CgroupParent:         resources.CgroupParent,
		CPUCount:             resources.CPUCount,
		CPUPercent:           resources.CPUPercent,
		CPUPeriod:            resources.CPUPeriod,
		CPUQuota:             resources.CPUQuota,
		// CPURealtimePeriod
		// CPURealtimeRuntime
		CPUShares:  resources.CPUShares,
		CpusetCpus: resources.CpusetCpus,
		CpusetMems: resources.CpusetMems,
		// DeviceCgroupRules
		Devices:            pouchDevices,
		IOMaximumIOps:      resources.IOMaximumIOps,
		IOMaximumBandwidth: resources.IOMaximumBandwidth,

		// TODO docker missing
		// IntelRdtL3Cbm:       resources.IntelRdtL3Cbm,
		// MemoryExtra:         &resources.MemoryExtra,
		// MemoryForceEmptyCtl: int64(resources.MemoryForceEmptyCtl),
		// MemoryWmarkRatio:  &int64(resources.MemoryWmarkRatio),
		// ScheLatSwitch:  resources.ScheLatSwitch,

		KernelMemory:      resources.KernelMemory,
		Memory:            resources.Memory,
		MemoryReservation: resources.MemoryReservation,
		MemorySwap:        resources.MemorySwap,
		MemorySwappiness:  resources.MemorySwappiness,
		// NanoCpus
		OomKillDisable: resources.OomKillDisable,
		PidsLimit:      resources.PidsLimit,
		Ulimits:        ulimits,
	}

	return pouchResources, nil
}

func toDevices(devs []containertypes.DeviceMapping) ([]*pouchtypes.DeviceMapping, error) {
	pouchDevices := []*pouchtypes.DeviceMapping{}
	for _, d := range devs {
		dev := &pouchtypes.DeviceMapping{
			CgroupPermissions: d.CgroupPermissions,
			PathInContainer:   d.PathInContainer,
			PathOnHost:        d.PathOnHost,
		}

		pouchDevices = append(pouchDevices, dev)
	}

	return pouchDevices, nil
}

// toMountPoints takeover all mounts information of container
func toMountPoints(mounts []dockertypes.MountPoint) ([]*pouchtypes.MountPoint, error) {
	pouchMounts := []*pouchtypes.MountPoint{}
	for _, m := range mounts {
		mount := &pouchtypes.MountPoint{
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Driver:      m.Driver,
			Mode:        m.Mode,
			RW:          m.RW,
			Propagation: string(m.Propagation),
		}

		if utils.StringInSlice([]string{"alilocal", "ultron"}, mount.Driver) {
			mount.Named = true
		}
		pouchMounts = append(pouchMounts, mount)
	}

	return pouchMounts, nil
}

func toNetworkSettings(networkSettings *dockertypes.NetworkSettings) (*pouchtypes.NetworkSettings, error) {
	if networkSettings == nil {
		return nil, nil
	}

	pouchNetworkSettings := &pouchtypes.NetworkSettings{
		Bridge:                 networkSettings.Bridge,
		HairpinMode:            networkSettings.HairpinMode,
		LinkLocalIPV6Address:   networkSettings.LinkLocalIPv6Address,
		LinkLocalIPV6PrefixLen: int64(networkSettings.LinkLocalIPv6PrefixLen),
		SandboxID:              networkSettings.SandboxID,
		SandboxKey:             networkSettings.SandboxKey,
		// SecondaryIPAddresses
		// SecondaryIPV6Addresses
	}

	// Networks
	networks, err := toEndpointSettings(networkSettings.Networks)
	if err != nil {
		return nil, err
	}
	pouchNetworkSettings.Networks = networks

	// PortMap
	portMap, err := toPortMap(networkSettings.Ports)
	if err != nil {
		return nil, err
	}
	pouchNetworkSettings.Ports = portMap

	return pouchNetworkSettings, nil
}

func toEndpointSettings(networks map[string]*networktypes.EndpointSettings) (map[string]*pouchtypes.EndpointSettings, error) {
	if networks == nil {
		return nil, nil
	}

	pouchNetworks := map[string]*pouchtypes.EndpointSettings{}
	for k, v := range networks {
		net := &pouchtypes.EndpointSettings{
			// Aliases
			// DriverOpts
			EndpointID:          v.EndpointID,
			Gateway:             v.Gateway,
			GlobalIPV6Address:   v.GlobalIPv6Address,
			GlobalIPV6PrefixLen: int64(v.GlobalIPv6PrefixLen),
			IPAddress:           v.IPAddress,
			IPPrefixLen:         int64(v.IPPrefixLen),
			IPV6Gateway:         v.IPv6Gateway,
			Links:               v.Links,
			MacAddress:          v.MacAddress,
			NetworkID:           v.NetworkID,
		}

		if v.IPAMConfig != nil {
			net.IPAMConfig = &pouchtypes.EndpointIPAMConfig{
				IPV4Address:  v.IPAMConfig.IPv4Address,
				IPV6Address:  v.IPAMConfig.IPv6Address,
				LinkLocalIps: v.IPAMConfig.LinkLocalIPs,
			}
		}

		pouchNetworks[k] = net
	}

	return pouchNetworks, nil
}

func toPortMap(ports nat.PortMap) (pouchtypes.PortMap, error) {
	if ports == nil {
		return nil, nil
	}

	pouchPortMap := pouchtypes.PortMap{}
	for k, v := range ports {
		portsBinding := []pouchtypes.PortBinding{}
		for _, binding := range v {
			portsBinding = append(portsBinding, pouchtypes.PortBinding{
				HostIP:   binding.HostIP,
				HostPort: binding.HostPort,
			})
		}

		pouchPortMap[string(k)] = portsBinding
	}

	return pouchPortMap, nil
}

func toThrottleDevices(devs []*blkiodev.ThrottleDevice) ([]*pouchtypes.ThrottleDevice, error) {
	if devs == nil {
		return nil, nil
	}
	throttleDevices := []*pouchtypes.ThrottleDevice{}
	for _, dev := range devs {
		throttleDevices = append(throttleDevices, &pouchtypes.ThrottleDevice{
			Path: dev.Path,
			Rate: dev.Rate,
		})
	}

	return throttleDevices, nil
}

func toWeightDevice(devs []*blkiodev.WeightDevice) ([]*pouchtypes.WeightDevice, error) {
	if devs == nil {
		return nil, nil
	}
	weightDevices := []*pouchtypes.WeightDevice{}
	for _, dev := range devs {
		weightDevices = append(weightDevices, &pouchtypes.WeightDevice{
			Path:   dev.Path,
			Weight: dev.Weight,
		})
	}

	return weightDevices, nil
}

func parseEnv(envs []string) (map[string]string, error) {
	result := map[string]string{}
	for _, e := range envs {
		mapping := strings.Split(e, "=")
		if len(mapping) != 2 {
			continue
		}

		result[mapping[0]] = mapping[1]
	}

	return result, nil
}

// convertTime convert time from Local to UTC. Because in docker time is parsed as Local, in pouch time
// is parsed as UTC
func convertTime(t string) string {
	n, err := time.Parse(time.RFC3339Nano, t)
	if err != nil {
		return t
	}

	return n.UTC().Format(time.RFC3339Nano)
}
