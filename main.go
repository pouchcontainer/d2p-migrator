package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pouchcontainer/d2p-migrator/ctrd"
	"github.com/pouchcontainer/d2p-migrator/migrator"
	"github.com/pouchcontainer/d2p-migrator/version"

	"github.com/docker/docker/pkg/reexec"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Config of pouch-migrator
type Config struct {
	DockerRpmName string
	PouchRpmPath  string
	MigrateAll    bool
	ImageProxy    string
	LiveMigrate   bool
	PrepareImage  bool
}

var (
	printVersion bool
	cfg          = &Config{}
	rePullImages []string
)

func main() {
	if reexec.Init() {
		return
	}

	var cmdServe = &cobra.Command{
		Use:          "d2p-migrator",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCmd()
		},
	}

	setupFlags(cmdServe)
	parseFlags(cmdServe, os.Args[1:])

	if err := cmdServe.Execute(); err != nil {
		fmt.Printf("failed to execute pouch-migrator: %v", err)
		os.Exit(1)
	}

	os.Exit(0)
}

// initLog initializes log Level and log format of daemon.
func initLog() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.Infof("start daemon at debug level")

	formatter := &logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339Nano,
	}
	logrus.SetFormatter(formatter)
}

func setupFlags(cmd *cobra.Command) {
	flagSet := cmd.Flags()

	flagSet.StringVar(&cfg.DockerRpmName, "docker-pkg", "docker", "Specify docker package name")
	flagSet.StringVar(&cfg.PouchRpmPath, "pouch-pkg-path", "pouch", "Specify pouch package file path")
	flagSet.BoolVar(&cfg.MigrateAll, "migrate-all", false, "If true, do all migration things, otherwise, just prepare data for migration")
	flagSet.BoolVar(&cfg.LiveMigrate, "live-migrate", false, "Auto takeover the docker running containers when migration, which will not affect the whole containers at all")
	flagSet.StringVar(&cfg.ImageProxy, "image-proxy", "", "Http proxy to pull image")
	flagSet.BoolVar(&cfg.PrepareImage, "pull-images", false, "If this flag set, we will just pull container images")
	flagSet.StringSliceVar(&rePullImages, "repull-images", []string{}, "Images d2p-migrator will actually pull")
	flagSet.BoolVarP(&printVersion, "version", "v", false, "Print d2p-migrator version")
}

func parseFlags(cmd *cobra.Command, flags []string) {
	err := cmd.Flags().Parse(flags)
	if err == nil || err == pflag.ErrHelp {
		return
	}

	cmd.SetOutput(os.Stderr)
	cmd.Usage()
}

// runCmd prepares configs, setups essential details and runs pouch-migrator
func runCmd() error {
	// initialize log
	initLog()
	//user specifies --version or -v, print version and return.
	if printVersion {
		fmt.Printf("d2p-migrator version: %s, build: %s, build at: %s\n", version.Version, version.GitCommit, version.BuildTime)
		return nil
	}

	if cfg.ImageProxy != "" {
		ctrd.SetImageProxy(cfg.ImageProxy)
	}

	rePullImageSet := make(map[string]struct{})
	for _, image := range rePullImages {
		rePullImageSet[image] = struct{}{}
	}

	ctx := context.Background()
	migratorCfg := migrator.Config{
		Type:           "cold-migrate",
		DockerRpmName:  cfg.DockerRpmName,
		PouchRpmPath:   cfg.PouchRpmPath,
		RePullImageSet: rePullImageSet,
	}

	if cfg.LiveMigrate {
		migratorCfg.Type = "live-migrate"
	}
	migrator, err := migrator.NewD2pMigrator(migratorCfg)
	if err != nil {
		logrus.Errorf("failed to new pouch migrator: %v\n", err)
		return err
	}
	defer migrator.Cleanup()

	if cfg.PrepareImage {
		return migrator.PrepareImages(ctx)
	}

	if err := migrator.PreMigrate(ctx); err != nil {
		logrus.Errorf("failed to execute PreMigrage: %v", err)
		return err
	}

	if !cfg.MigrateAll {
		logrus.Infof("Just prepare data:\n * Pull Image \n * Prepare snapshots \n * Set disk quota \n * Convert docker container meta json file to pouch container meta json file ")
		return nil
	}

	// If migration failed, revert it.
	needRevert := false
	defer func() {
		if needRevert {
			logrus.Info("Migration has failed, start revert...")

			if err := migrator.RevertMigration(ctx); err != nil {
				logrus.Errorf("failed to revert migration: %v", err)
				return
			}

			logrus.Info("Revert migration done!\n")
		}
	}()

	if err := migrator.Migrate(ctx); err != nil {
		logrus.Errorf("failed to migrate: %v\n", err)
		needRevert = true
		return err
	}

	// If PostMigrate failed, should handle by manual.
	if err := migrator.PostMigrate(ctx); err != nil {
		logrus.Errorf("PostMigrate error: %v, need handle by manual!!!", err)
		return err
	}

	return nil
}
