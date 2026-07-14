//go:build windows

package pluginpkg

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func acquireStateFileLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open plugin lifecycle lock: %w", err)
	}
	handle := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock plugin lifecycle: %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
		_ = f.Close()
	}, nil
}
