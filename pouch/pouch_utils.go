package pouch

import (
	"fmt"
	"strings"

	"github.com/pouchcontainer/d2p-migrator/utils"

	pouchtypes "github.com/alibaba/pouch/apis/types"
	dockertypes "github.com/docker/engine-api/types"
	containertypes "github.com/docker/engine-api/types/container"
	networktypes "github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/go-openapi/strfmt"
)

var (
	// RemoteDrivers specify remote disk drivers
	RemoteDrivers = []string{"ultron"}
)

// ToPouchContainerMeta coverts docker container config to pouch container config.
func ToPouchContainerMeta(meta *dockertypes.ContainerJSON) (*PouchContainer, error) {
	if meta == nil {
		return nil, nil
	}

	// Convert Base Parameters
	pouchMeta := &PouchContainer{
		// SeccompProfile
		// NoNewPrivileges
		// TODO: should change ExecIds to ExecIDs
		// ExecIds:         meta.ExecIDs,
		// DetachKeys
		// Node
		AppArmorProfile: meta.AppArmorProfile,
		Args:            meta.Args,
		Created:         meta.Created,
		// TODO: default convert overlay2 container
		Driver: "overlay2",

		// TODO: This three path not be set
		HostnamePath:   "",
		HostsPath:      "",
		ResolvConfPath: "",

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

	// State: converted container is stopped

	pouchMeta.State = &pouchtypes.ContainerState{
		Dead:       meta.State.Dead,
		Error:      meta.State.Error,
		ExitCode:   int64(meta.State.ExitCode),
		FinishedAt: meta.State.FinishedAt,
		OOMKilled:  false,
		Paused:     meta.State.Paused,
		Pid:        int64(meta.State.Pid),
		Restarting: false,
		Running:    meta.State.Running,
		StartedAt:  meta.State.StartedAt,
		Status:     toContainerStatus(meta.State.Status),
	}

	// Config
	config, err := toContainerConfig(meta.Config)
	if err != nil {
		return nil, err
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

	// Mounts
	mountPoints, err := toMountPoints(meta.Mounts)
	if err != nil {
		return nil, err
	}
	pouchMeta.Mounts = mountPoints

	// Convert all mountpoint to bind
	for _, mount := range mountPoints {
		var bind string
		if utils.StringInSlice(RemoteDrivers, mount.Driver) {
			bind = fmt.Sprintf("%s:%s", mount.Name, mount.Destination)
		} else {
			bind = fmt.Sprintf("%s:%s", mount.Source, mount.Destination)
		}
		if !mount.RW {
			bind += ":ro"
		}

		exist := false
		for _, v := range hostconfig.Binds {
			if strings.HasPrefix(v, bind) {
				exist = true
				break
			}
		}

		if !exist {
			hostconfig.Binds = append(hostconfig.Binds, bind)
		}
	}

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

	// Volumes are all set empty
	pouchConfig.Volumes = map[string]interface{}{}

	// Convert *int to *int64
	// pouchConfig.StopTimeout = &(int64(*config.StopTimeout)),

	// ArgsEscaped
	// DiskQuota
	// ExposedPorts
	// InitScript

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
		// LogConfig
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
		VolumesFrom:  []string{},
	}

	// Binds
	for _, bind := range hostconfig.Binds {
		if strings.HasPrefix(bind, "/") {
			pouchHostConfig.Binds = append(pouchHostConfig.Binds, bind)
		}
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
	pouchResources := pouchtypes.Resources{
		// BlkioDeviceReadBps
		// BlkioDeviceReadBps
		// BlkioDeviceWriteBps
		// BlkioDeviceWriteBps
		BlkioWeight: resources.BlkioWeight,
		//BlkioWeightDevice
		CgroupParent: resources.CgroupParent,
		CPUCount:     resources.CPUCount,
		CPUPercent:   resources.CPUPercent,
		CPUPeriod:    resources.CPUPeriod,
		CPUQuota:     resources.CPUQuota,
		// CPURealtimePeriod
		// CPURealtimeRuntime
		CPUShares:  resources.CPUShares,
		CpusetCpus: resources.CpusetCpus,
		CpusetMems: resources.CpusetMems,
		// DeviceCgroupRules
		// Devices
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
		// Ulimits
	}

	return pouchResources, nil
}

// TODO How to let pouch manager docker's volumes
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

		if !utils.StringInSlice(RemoteDrivers, mount.Driver) {
			// change volume to bind, unset volume info
			mount.Name = ""
			mount.Driver = ""
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
