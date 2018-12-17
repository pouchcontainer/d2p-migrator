package pouch

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/pouchcontainer/d2p-migrator/utils"
	"github.com/sirupsen/logrus"
)

var (
	// ConfigFile represents the config file of pouchd
	ConfigFile = "/etc/pouch/config.json"
	// SocketAddr represents the socket address of pouchd
	SocketAddr = "/var/run/pouchd.sock"
)

// InstallPouchService install pouch rpm and wait the poucd to serve
func InstallPouchService(pouchRpmPath string) error {
	if err := utils.ExecCommand("rpm", "-Uvh", pouchRpmPath); err != nil {
		logrus.Errorf("failed to install pouch: %v", err)
		return err
	}
	// wait pouchd to serve
	if err := waitPouchAlive(); err != nil {
		return err
	}

	return nil
}

// waitPouchAlive waits pouchd to serve
func waitPouchAlive() error {
	// Starting wait pouchd to serve
	check := make(chan struct{})
	timeout := make(chan bool, 1)
	// set timeout to wait pouchd start
	go func() {
		time.Sleep(120 * time.Second)
		timeout <- true
	}()

	// check whether pouchd starts success
	go func() {
		for {
			_, err := net.Dial("unix", SocketAddr)
			if err == nil {
				check <- struct{}{}
			}
		}
	}()

	select {
	case <-check:
		// pouchd has started
	case <-timeout:
		return fmt.Errorf("failed to wait pouchd start, 120s timeout")
	}

	return nil
}

// ChangeHomeDir change home directory of pouchd service.
func ChangeHomeDir(homeDir string) error {
	_, err := os.Stat("/etc/pouch/config.json")
	if err != nil {
		return err
	}

	replaceReg := fmt.Sprintf(`s|\("home-dir": "\).*|\1%s",|`, homeDir)
	return utils.ExecCommand("sed", "-i", replaceReg, ConfigFile)
}
