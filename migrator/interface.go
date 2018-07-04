package migrator

import "context"

// Migrator is an interface to migrate docker containers to other containers
type Migrator interface {
	// PreMigrate do something before migration
	PreMigrate(ctx context.Context, takeOverContainer bool) error

	// Migrate does migrate action
	Migrate(ctx context.Context, takeOverContainer bool) error

	// PostMigrate do something after migration
	PostMigrate(ctx context.Context, takeOverContainer bool) error

	// RevertMigration reverts migration
	RevertMigration(ctx context.Context, takeOverContainer bool) error

	// Cleanup does some clean works when migrator exited
	Cleanup() error

	// PrepareImages just pull images for containers
	PrepareImages(ctx context.Context) error
}
