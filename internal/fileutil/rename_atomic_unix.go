//go:build !windows

package fileutil

import (
	"os"
	"path/filepath"
)

func renameAtomic(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	return syncDirectoryPath(filepath.Dir(newPath))
}

func syncDirectoryPath(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
