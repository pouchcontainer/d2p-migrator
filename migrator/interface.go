package migrator

import "context"

// Migrator is an interface to migrate docker containers to other containers
type Migrator interface {
	// PreMigrate do something before migration
	PreMigrate(ctx context.Context) error

	// Migrate does migrate action
	Migrate(ctx context.Context) error

	// PostMigrate do something after migration
	PostMigrate(ctx context.Context) error

	// RevertMigration reverts migration
	RevertMigration() error

	// Cleanup does some clean works when migrator exited
	Cleanup() error
}
