//go:build windows

package fileutil

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// renameCrossesDevice identifies rename failures that cannot become atomic by
// retrying. ReplaceFile surfaces them immediately and keeps dest unchanged.
func renameCrossesDevice(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) || errors.Is(err, syscall.EXDEV)
}
