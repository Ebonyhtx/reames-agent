//go:build windows

package pluginregistry

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

type cacheLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func takeCacheLock(ctx context.Context, path string) (*cacheLock, error) {
	file, err := openPrivateCacheFile(path)
	if err != nil {
		return nil, err
	}
	lock := &cacheLock{file: file}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &lock.overlapped)
		if err == nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &lock.overlapped)
				_ = file.Close()
				return nil, ctxErr
			}
			return lock, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			_ = file.Close()
			return nil, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *cacheLock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlapped)
	_ = l.file.Close()
}
