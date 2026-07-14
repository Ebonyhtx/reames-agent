//go:build windows

package fileutil

import (
	"os"
	"path/filepath"
)

func syncRootDir(root *os.Root, dir string) error {
	return syncDirectoryPath(filepath.Join(root.Name(), dir))
}
