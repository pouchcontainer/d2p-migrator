package containerplugin

import (
	"github.com/pouchcontainer/d2p-migrator/hookplugins"
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
)

type contPlugin struct{}

func init() {
	hookplugins.RegisterContainerPlugin(&contPlugin{})
}

// PostCovert will be called after convert container successful
func (c *contPlugin) PostConvert(dockerHomeDir string, cont *localtypes.Container) error {
	// implement by developers
	return nil
}
