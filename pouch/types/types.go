package types

import (
	pouchtypes "github.com/alibaba/pouch/apis/types"
	runtime "github.com/alibaba/pouch/cri/apis/v1alpha2"
)

// Container represents the container's meta data.
type Container struct {
	// app armor profile
	AppArmorProfile string `json:"AppArmorProfile,omitempty"`

	// seccomp profile
	SeccompProfile string `json:"SeccompProfile,omitempty"`

	// no new privileges
	NoNewPrivileges bool `json:"NoNewPrivileges,omitempty"`

	// The arguments to the command being run
	Args []string `json:"Args"`

	// config
	Config *pouchtypes.ContainerConfig `json:"Config,omitempty"`

	// The time the container was created
	Created string `json:"Created,omitempty"`

	// driver
	Driver string `json:"Driver,omitempty"`

	// exec ids
	ExecIds string `json:"ExecIDs,omitempty"`

	// Snapshotter, GraphDriver is same, keep both
	// just for compatibility
	// snapshotter informations of container
	Snapshotter *pouchtypes.SnapshotterData `json:"Snapshotter,omitempty"`

	// graph driver
	GraphDriver *pouchtypes.GraphDriverData `json:"GraphDriver,omitempty"`

	// host config
	HostConfig *pouchtypes.HostConfig `json:"HostConfig,omitempty"`

	// hostname path
	HostnamePath string `json:"HostnamePath,omitempty"`

	// hosts path
	HostsPath string `json:"HostsPath,omitempty"`

	// The ID of the container
	ID string `json:"Id,omitempty"`

	// The container's image
	Image string `json:"Image,omitempty"`

	// log path
	LogPath string `json:"LogPath,omitempty"`

	// mount label
	MountLabel string `json:"MountLabel,omitempty"`

	// mounts
	Mounts []*pouchtypes.MountPoint `json:"Mounts"`

	// name
	Name string `json:"Name,omitempty"`

	// network settings
	NetworkSettings *pouchtypes.NetworkSettings `json:"NetworkSettings,omitempty"`

	Node interface{} `json:"Node,omitempty"`

	// The path to the command being run
	Path string `json:"Path,omitempty"`

	// process label
	ProcessLabel string `json:"ProcessLabel,omitempty"`

	// resolv conf path
	ResolvConfPath string `json:"ResolvConfPath,omitempty"`

	// restart count
	RestartCount int64 `json:"RestartCount,omitempty"`

	// The total size of all the files in this container.
	SizeRootFs int64 `json:"SizeRootFs,omitempty"`

	// The size of files that have been created or changed by this container.
	SizeRw int64 `json:"SizeRw,omitempty"`

	// state
	State *pouchtypes.ContainerState `json:"State,omitempty"`

	// BaseFS
	BaseFS string `json:"BaseFS,omitempty"`

	// Escape keys for detach
	DetachKeys string

	// Specify if the container is taken over by pouch,
	// or just created by pouch
	RootFSProvided bool
}

// SandboxMeta represents the sandbox's meta data.
// copy from github.com/alibaba/pouch/cri/v1alpha2/cri_types.go
type SandboxMeta struct {
	// ID is the id of sandbox.
	ID string

	// Config is CRI sandbox config.
	Config *runtime.PodSandboxConfig

	// Runtime is the runtime of sandbox
	Runtime string

	// Runtime whether to enable lxcfs for a container
	LxcfsEnabled bool

	// NetNS is the sandbox's network namespace
	NetNS string
}

// Key returns sandbox's id.
func (meta *SandboxMeta) Key() string {
	return meta.ID
}
