//go:build !windows

package pluginregistry

import "os"

func unsafePathEntry(_ string, info os.FileInfo) (bool, string) {
	if info.Mode()&os.ModeSymlink != 0 {
		return true, "symbolic links are not allowed"
	}
	return false, ""
}
