package utils

import (
	"os/exec"

	"github.com/sirupsen/logrus"
)

// ExecCommand is a wrapper of exec Command
func ExecCommand(name string, args ...string) error {
	argsStr := name
	for _, arg := range args {
		argsStr += " " + arg
	}
	logrus.Debugf("start exec command: [%s]", argsStr)

	cmd := exec.Command(name, args...)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	logrus.Infof("exec command [%s] output: %s", argsStr, stdoutStderr)
	return nil
}
