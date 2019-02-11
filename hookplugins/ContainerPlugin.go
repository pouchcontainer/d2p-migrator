package hookplugins

import (
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
)

// ContainerPlugin will be called when converts docker container to pouch container.
type ContainerPlugin interface {

	// PostCovert will be called after convert container successful.
	// this plugin pass two parameters:
	// `dockerHomeDir`: specify docker home-dir that we can find the container meta informations.
	// `cont`: which container we will deal with.
	PostConvert(dockerHomeDir string, cont *localtypes.Container) error
}

var containerPlugin ContainerPlugin

// RegisterContainerPlugin is used to register container plugin.
func RegisterContainerPlugin(cp ContainerPlugin) {
	containerPlugin = cp
}

// GetContainerPlugin returns the container plugin.
func GetContainerPlugin() ContainerPlugin {
	return containerPlugin
}
