package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var (
	maxReplaceRetries = 12
	replaceRetryBase  = 20 * time.Millisecond

	// renameFile is a test seam: transient lock and cross-device failures
	// cannot be provoked portably on a real filesystem.
	renameFile = renameAtomic
)

// AtomicWriteFile writes data to a sibling temporary file, fsyncs it, applies
// the requested mode, and atomically replaces path. A successful call leaves
// either the complete old file or the complete new file across a crash or power
// loss; it never degrades to an in-place copy.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	dirPerm := os.FileMode(0o755)
	if perm&0o077 == 0 {
		dirPerm = 0o700
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write tmp for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("fsync tmp for %s: %w", path, err)
	}
	// Chmod the open handle before Close so another process cannot move the
	// temporary file between those operations.
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod tmp for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close tmp for %s: %w", path, err)
	}
	if err := ReplaceFile(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// ReplaceFile atomically renames tmp onto dest. Transient sharing locks are
// retried. A cross-device error, including ERROR_NOT_SAME_DEVICE from a Windows
// filter driver, fails closed immediately because no atomic fallback exists.
func ReplaceFile(tmp, dest string) error {
	var err error
	for attempt := 0; ; attempt++ {
		if err = renameFile(tmp, dest); err == nil {
			return nil
		}
		if renameCrossesDevice(err) {
			return err
		}
		if attempt >= maxReplaceRetries || !fileExists(tmp) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * replaceRetryBase)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SyncParentDir persists a newly created or renamed directory entry where the
// platform exposes that guarantee.
func SyncParentDir(path string) error {
	return syncDirectoryPath(filepath.Dir(path))
}
