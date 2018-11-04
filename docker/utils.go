package docker

import (
	"fmt"

	"github.com/pouchcontainer/d2p-migrator/utils"

	"github.com/sirupsen/logrus"
)

// StopDockerService stops the docker service.
func StopDockerService() error {
	logrus.Debug("Start to stop docker service")
	stopDocker := make(chan error, 1)
	go func() {
		var err error
		for i := 0; i < 3; i++ {
			err = utils.ExecCommand("systemctl", "stop", "docker")
			if err == nil {
				break
			}

			logrus.Errorf("failed to stop docker service: %v", err)
		}

		stopDocker <- err
	}()

	if err := <-stopDocker; err != nil {
		return fmt.Errorf("failed to stop docker: %v", err)
	}

	logrus.Debug("Success stopped docker service")
	return nil
}

// UninstallDockerService remove docker package
func UninstallDockerService(dockerRpmName string) error {
	// first, backup some config files, in case we may revert migration.
	for _, f := range []string{"/etc/sysconfig/docker", "/etc/docker/daemon.json"} {
		if err := utils.ExecCommand("cp", f, f+".bk"); err != nil {
			return err
		}
	}

	// second, we must first stop the docker before remove it
	if err := StopDockerService(); err != nil {
		return err
	}

	// now remove docker rpm package
	logrus.Infof("Start to uninstall docker %s ", dockerRpmName)
	if err := utils.ExecCommand("yum", "remove", "-y", dockerRpmName); err != nil {
		return fmt.Errorf("failed to uninstall docker: %v", err)
	}

	return nil
}
