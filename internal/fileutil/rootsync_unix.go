//go:build !windows

package fileutil

import (
	"os"
)

func syncRootDir(root *os.Root, dir string) error {
	handle, err := root.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
