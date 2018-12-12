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

// CopyFile copys src file to dest file
func CopyFile(dst, src string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
