package utils

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestMoveDir(t *testing.T) {
	// prepare dirs
	dirs := []string{}
	for _, name := range []string{"testMoveDirSrc", "testMoveDirDst"} {
		tmpDir, err := ioutil.TempDir("", name)
		if err != nil {
			t.Errorf("failed to create a tempDir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		dirs = append(dirs, tmpDir)
	}

	// prepare a file
	tmpFile := path.Join(dirs[0], "test")
	if err := ioutil.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Errorf("failed to write content to file %s: %v", tmpFile, err)
	}

	// call MoveDir function
	if err := MoveDir(dirs[0], dirs[1]); err != nil {
		t.Errorf("failed to call MoveDir function: %v", err)
	}

	// test MoveDir function results
	// 1. check if soruce dir is empty
	isEmpty, err := IsDirEmpty(dirs[0])
	if err != nil || !isEmpty {
		t.Errorf("MoveDir got wrong result, expect source dir %s is empty dir, got (isEmpty %v, err: %v)", dirs[0], isEmpty, err)
	}

	// 2. check destination dir
	isEmpty, err = IsDirEmpty(dirs[1])
	if err != nil || isEmpty {
		t.Errorf("MoveDir got wrong result, expecte dstDir %s is not empty dir, got (isEmpty %v, err: %v)", dirs[1], isEmpty, err)
	}

	// 3. check file content
	movedTmpFile := path.Join(dirs[1], "test")
	rawData, err := ioutil.ReadFile(movedTmpFile)
	if err != nil {
		t.Errorf("failed to read file %s: %v", movedTmpFile, err)
	}

	if !strings.Contains(string(rawData), "test") {
		t.Errorf("moved temp file got unexpected, got %s, expected test", string(rawData))
	}
}

func TestIsDirEmpty(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "testIsDirEmpty")
	if err != nil {
		t.Errorf("failed to create a tempDir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// check empty dir
	isEmpty, err := IsDirEmpty(tmpDir)
	if err != nil || !isEmpty {
		t.Errorf("call IsDirEmpty function which pass empty Dir  got unexpected result: (isEmpty %v, err %v)", isEmpty, err)
	}

	// check not empty dir
	tmpFile := path.Join(tmpDir, "test")
	if err := ioutil.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Errorf("failed to write content to file %s: %v", tmpFile, err)
	}
	isEmpty, err = IsDirEmpty(tmpDir)
	if err != nil || isEmpty {
		t.Errorf("call IsDirEmpty function which pass not empty Dir got unexpected result: (isEmpty %v, err %v)", isEmpty, err)
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "testCopyFile")
	if err != nil {
		t.Errorf("failed to create a tempDir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	var (
		srcFile = path.Join(tmpDir, "src")
		dstFile = path.Join(tmpDir, "dst")
	)

	if err := ioutil.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Errorf("failed to write content to file %s: %v", srcFile, err)
	}

	// call CopyFile
	if err := CopyFile(dstFile, srcFile, 0644); err != nil {
		t.Errorf("failed to call CopyFile: %v", err)
	}

	// check result
	rawData, err := ioutil.ReadFile(dstFile)
	if err != nil {
		t.Errorf("failed to read file %s: %v", dstFile, err)
	}

	if !strings.Contains(string(rawData), "test") {
		t.Errorf("moved temp file got unexpected, got %s, expected test", string(rawData))
	}

}
