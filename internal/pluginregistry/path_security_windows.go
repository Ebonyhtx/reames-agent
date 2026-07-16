//go:build windows

package pluginregistry

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func unsafePathEntry(path string, info os.FileInfo) (bool, string) {
	if info.Mode()&os.ModeSymlink != 0 {
		return true, "symbolic links are not allowed"
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true, fmt.Sprintf("inspect reparse attributes: %v", err)
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return true, fmt.Sprintf("inspect reparse attributes: %v", err)
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return true, "reparse points are not allowed"
	}
	return false, ""
}
