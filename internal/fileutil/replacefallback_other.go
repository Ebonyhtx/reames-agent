//go:build !windows

package fileutil

import (
	"errors"
	"syscall"
)

// renameCrossesDevice identifies EXDEV, which cannot become atomic by retrying.
func renameCrossesDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
