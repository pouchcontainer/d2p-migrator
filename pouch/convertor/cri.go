package convertor

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
	"github.com/pouchcontainer/d2p-migrator/utils"

	runtime "github.com/alibaba/pouch/cri/apis/v1alpha2"
)

const (
	// PouchContainerTypeLabelKey label to identify whether a container is a sandbox
	// or container
	PouchContainerTypeLabelKey = "io.kubernetes.pouch.type"
	// PouchContainerTypeLabelSandbox specify this container is a sandbox
	PouchContainerTypeLabelSandbox = "sandbox"
	// PouchContainerTypeLabelContainer specify this container is a container
	PouchContainerTypeLabelContainer = "container"

	// DockerContainerTypeLabelKey label to identify whether a container is a sandbox
	// or container
	DockerContainerTypeLabelKey = "io.kubernetes.docker.type"

	// SandboxIDLabelKey specify sandbox container id
	SandboxIDLabelKey = "io.kubernetes.sandbox.id"
)

var (
	// SandboxNameRegex is the regex of Sandbox name
	SandboxNameRegex = regexp.MustCompile("^k8s_POD_([^_]+_){3}[0-9]+$")
)

// ToCRIMetaJSON generate cri sandbox meta json from sandbox container
func ToCRIMetaJSON(c *localtypes.Container) (*localtypes.SandboxMeta, error) {
	if c == nil {
		return nil, nil
	}

	// ID
	sandbox := &localtypes.SandboxMeta{
		ID: c.ID,
	}

	// Runtime and LxcfsEnabled
	if c.HostConfig != nil {
		sandbox.Runtime = c.HostConfig.Runtime
		sandbox.LxcfsEnabled = c.HostConfig.EnableLxcfs
	}

	// NetNS
	pid := 0
	if c.State != nil {
		pid = int(c.State.Pid)
	}
	if pid > 0 {
		sandbox.NetNS = fmt.Sprintf("/proc/%v/ns/net", pid)
	}

	sandboxConfig, err := toPodSandboxConfig(c)
	if err != nil {
		return nil, err
	}
	sandbox.Config = sandboxConfig

	return sandbox, nil
}

// type PodSandboxConfig struct {
//     Metadata *PodSandboxMetadata
//     Hostname string
//     LogDirectory string
//     DnsConfig *DNSConfig
//     PortMappings []*PortMapping
//     Labels map[string]string
//     Annotations map[string]string
//     Linux *LinuxPodSandboxConfig
// }
//
func toPodSandboxConfig(c *localtypes.Container) (*runtime.PodSandboxConfig, error) {
	sandboxConfig := &runtime.PodSandboxConfig{
		Labels: map[string]string{},
	}

	meta, err := getPodSandboxMetadataBySandboxName(c.Name)
	if err != nil {
		return nil, err
	}
	sandboxConfig.Metadata = meta

	if c.Config != nil {
		labelKeyType := []string{
			PouchContainerTypeLabelKey,
			DockerContainerTypeLabelKey,
		}

		sandboxConfig.Hostname = string(c.Config.Hostname)
		sandboxConfig.Annotations = c.Config.SpecAnnotation

		for k, v := range c.Config.Labels {
			// filter annotation labels
			if !strings.HasPrefix(k, "annotation.") && !utils.StringInSlice(labelKeyType, k) {
				sandboxConfig.Labels[k] = v
			}

			// covert annotation label to annations
			if strings.HasPrefix(k, "annotation.") {
				if sandboxConfig.Annotations == nil {
					sandboxConfig.Annotations = map[string]string{}
				}

				sandboxConfig.Annotations[strings.TrimPrefix(k, "annotation.")] = v
			}
		}
	}

	// LogDirectory
	sandboxConfig.LogDirectory = fmt.Sprintf("/var/log/pods/%s", meta.Uid)

	// Linux
	linuxConfig, err := toLinuxPodSandboxConfig(c)
	if err != nil {
		return nil, err
	}
	sandboxConfig.Linux = linuxConfig

	// DnsConfig
	dnsConf, err := toDNSConfig(c.ResolvConfPath)
	if err != nil {
		return nil, err
	}
	sandboxConfig.DnsConfig = dnsConf

	return sandboxConfig, nil
}

func getPodSandboxMetadataBySandboxName(name string) (*runtime.PodSandboxMetadata, error) {
	if !SandboxNameRegex.Match([]byte(name)) {
		return nil, fmt.Errorf("name %s not match k8s sandbox name regexp %s", name, SandboxNameRegex.String())
	}

	parts := strings.Split(name, "_")
	attempt, err := strconv.ParseUint(parts[5], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the attempt times in sandbox name: %q: %v", name, err)
	}

	return &runtime.PodSandboxMetadata{
		Name:      parts[2],
		Namespace: parts[3],
		Uid:       parts[4],
		Attempt:   uint32(attempt),
	}, nil
}

func toLinuxPodSandboxConfig(c *localtypes.Container) (*runtime.LinuxPodSandboxConfig, error) {
	linuxConfig := &runtime.LinuxPodSandboxConfig{}
	if c.HostConfig != nil {
		linuxConfig.CgroupParent = c.HostConfig.CgroupParent
		linuxConfig.Sysctls = c.HostConfig.Sysctls
	}

	linuxSecurityContext, err := toLinuxSandboxSecurityContext(c)
	if err != nil {
		return nil, err
	}
	linuxConfig.SecurityContext = linuxSecurityContext

	return linuxConfig, nil
}

func toLinuxSandboxSecurityContext(c *localtypes.Container) (*runtime.LinuxSandboxSecurityContext, error) {
	linuxSecurityContext := &runtime.LinuxSandboxSecurityContext{
		NamespaceOptions: &runtime.NamespaceOption{
			Pid: runtime.NamespaceMode_CONTAINER,
		},
	}
	if c.NetworkSettings != nil {
		if _, ok := c.NetworkSettings.Networks["host"]; ok {
			linuxSecurityContext.NamespaceOptions = &runtime.NamespaceOption{
				Pid:     runtime.NamespaceMode_NODE,
				Network: runtime.NamespaceMode_NODE,
				Ipc:     runtime.NamespaceMode_NODE,
			}
		}
	}

	return linuxSecurityContext, nil
}

// toDNSConfig generates DNSConfig from /etc/resolve file content
func toDNSConfig(resolvConfPath string) (*runtime.DNSConfig, error) {
	if resolvConfPath == "" {
		return nil, nil
	}

	if _, err := os.Stat(resolvConfPath); err != nil {
		return nil, err
	}

	var (
		servers  []string
		searches []string
		options  []string
	)

	rawData, err := ioutil.ReadFile(resolvConfPath)
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(rawData), "\n") {
		resolvStr := strings.TrimSpace(line)
		resolvOptions := strings.Split(resolvStr, " ")
		if len(resolvOptions) < 2 {
			continue
		}

		switch resolvOptions[0] {
		case "search":
			searches = append(searches, resolvOptions[1:]...)
		case "nameserver":
			servers = append(servers, resolvOptions[1:]...)
		case "options":
			options = append(options, resolvOptions[1:]...)
		}
	}

	return &runtime.DNSConfig{
		Servers:  utils.SliceTrimSpace(servers),
		Searches: utils.SliceTrimSpace(searches),
		Options:  utils.SliceTrimSpace(options),
	}, nil
}
