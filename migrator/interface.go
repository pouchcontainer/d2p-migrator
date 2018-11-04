package migrator

import (
	"context"

	"github.com/pouchcontainer/d2p-migrator/docker"
)

var migratorFactory map[string]func(Config) (Migrator, error)

// Migrator is an interface to migrate docker containers to other containers
type Migrator interface {
	// PreMigrate do something before migration
	PreMigrate(ctx context.Context, cli *docker.Dockerd) error

	// Migrate does migrate action
	Migrate(ctx context.Context, cli *docker.Dockerd) error

	// PostMigrate do something after migration
	PostMigrate(ctx context.Context, cli *docker.Dockerd, dockerRpmName, pouchRpmPath string) error

	// RevertMigration reverts migration
	RevertMigration(ctx context.Context, cli *docker.Dockerd) error

	// Cleanup does some clean works when migrator exited
	Cleanup() error
}

// Register registers a migrator to be a d2p-migrator.
func Register(name string, create func(Config) (Migrator, error)) {
	if migratorFactory == nil {
		migratorFactory = make(map[string]func(Config) (Migrator, error))
	}
	migratorFactory[name] = create
}
