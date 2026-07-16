//go:build !windows

package pluginregistry

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

type cacheLock struct {
	file *os.File
}

func takeCacheLock(ctx context.Context, path string) (*cacheLock, error) {
	file, err := openPrivateCacheFile(path)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
				return nil, ctxErr
			}
			return &cacheLock{file: file}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
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
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
