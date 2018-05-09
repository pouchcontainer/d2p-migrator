package utils

import (
	"fmt"
	"io"
	"os"
	"path"
)

// MoveDir moves all files in srcDir to dstDir
func MoveDir(srcDir, dstDir string) error {
	if srcDir == "" || dstDir == "" {
		return fmt.Errorf("mv: srcDir %s and dstDir %s not correct", srcDir, dstDir)
	}

	return ExecCommand("bash", "-c", fmt.Sprintf("mv %s %s", path.Join(srcDir, "*"), dstDir))
}

// IsDirEmpty is to check if a directory is empty
func IsDirEmpty(dir string) (bool, error) {
	if _, err := os.Stat(dir); err != nil {
		return false, err
	}

	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, nil
}
