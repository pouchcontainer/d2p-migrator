package migrator

import (
	"fmt"
	"os"
	"path"
	"reflect"

	"github.com/pouchcontainer/d2p-migrator/pouch/convertor"
	localtypes "github.com/pouchcontainer/d2p-migrator/pouch/types"
	"github.com/pouchcontainer/d2p-migrator/utils"

	"github.com/alibaba/pouch/pkg/meta"
	"github.com/sirupsen/logrus"
)

// GenerateSandboxMetaJSON generate cri sandbox meta.
func GenerateSandboxMetaJSON(homeDir string, sandboxCons []*localtypes.Container) error {
	store, err := newCRIStore(homeDir)
	if err != nil {
		return fmt.Errorf("failed to initialize cri store: %v", err)
	}
	defer store.Shutdown()

	return store.CreateSandboxes(sandboxCons)
}

// criStore is a store of cri meta
type criStore struct {
	sandboxBaseDir string
	sandboxStore   *meta.Store
}

// newCRIStore initializes a local store for cri meta
func newCRIStore(baseDir string) (*criStore, error) {
	// prepare cri meta dir if not exist
	var (
		metaDir      = path.Join(baseDir, "sandboxes-meta")
		sandboxesDir = path.Join(baseDir, "sandboxes")
	)
	for _, dir := range []string{metaDir, sandboxesDir} {
		if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0666); err != nil {
				return nil, fmt.Errorf("failed to prepare cri dir %s: %v", dir, err)
			}
		}
	}

	store, err := meta.NewStore(meta.Config{
		Driver:  "local",
		BaseDir: metaDir,
		Buckets: []meta.Bucket{
			{
				Name: meta.MetaJSONFile,
				Type: reflect.TypeOf(localtypes.SandboxMeta{}),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return &criStore{
		sandboxBaseDir: sandboxesDir,
		sandboxStore:   store,
	}, nil
}

// CreateSandboxes put all sandbox meta to cri meta store
func (cri *criStore) CreateSandboxes(conts []*localtypes.Container) error {
	for _, c := range conts {
		sandbox, err := convertor.ToCRIMetaJSON(c)
		if err != nil {
			return err
		}

		// put sandbox meta to store
		if err := cri.sandboxStore.Put(sandbox); err != nil {
			return fmt.Errorf("failed to create sandbox %s: %v", c.ID, err)
		}

		// put resolve.conf to sandbox dir
		// if the setup action occurred an error, just log error info here
		if err := cri.setupSandboxFiles(c.ResolvConfPath, sandbox); err != nil {
			logrus.Errorf("failed to setup sandbox %s files: %v", c.ID, err)
		}
	}

	return nil
}

func (cri *criStore) setupSandboxFiles(resolvConfPath string, sandbox *localtypes.SandboxMeta) error {
	// Set DNS options. Maintain a resolv.conf for the sandbox.
	sandboxDir := path.Join(cri.sandboxBaseDir, sandbox.ID)

	// prepare sandbox dir if not exist.
	if _, err := os.Stat(sandboxDir); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(sandboxDir, 0666); err != nil {
			return fmt.Errorf("failed to prepare sandbox dir %s: %v", sandboxDir, err)
		}
	}

	if resolvConfPath == "" {
		// Copy host's resolv.conf to resolvPath.
		resolvConfPath = "/etc/resolv.conf"
	}

	// Copy resolv.conf to sandboxDir
	return utils.CopyFile(path.Join(sandboxDir, "resolv.conf"), resolvConfPath, 0644)
}

// Shutdown close the store
func (cri *criStore) Shutdown() error {
	return cri.sandboxStore.Shutdown()
}
