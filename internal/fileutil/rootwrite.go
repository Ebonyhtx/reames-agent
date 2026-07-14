package fileutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteRootFile atomically replaces name beneath root without resolving
// the operation through an ambient absolute path. The temporary file is opened
// in the target directory through the same root handle, so path-component
// replacement cannot redirect either the write or the final rename outside the
// root. Like AtomicWriteFile, it fails closed when rename is unavailable and
// never falls back to an in-place copy.
func AtomicWriteRootFile(root *os.Root, name string, data []byte, perm os.FileMode) error {
	if root == nil {
		return fmt.Errorf("atomic root write: root is nil")
	}
	name = filepath.Clean(name)
	if !filepath.IsLocal(name) || name == "." {
		return fmt.Errorf("atomic root write: path %q is not local to root", name)
	}
	if perm&^os.FileMode(0o777) != 0 {
		return fmt.Errorf("atomic root write: unsupported file mode %v", perm)
	}

	dir := filepath.Dir(name)
	dirPerm := os.FileMode(0o755)
	if perm&0o077 == 0 {
		dirPerm = 0o700
	}
	if err := root.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create root dir for %s: %w", name, err)
	}

	tmp, tmpName, err := createRootTemp(root, dir)
	if err != nil {
		return fmt.Errorf("create root tmp for %s: %w", name, err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = root.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write root tmp for %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("fsync root tmp for %s: %w", name, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod root tmp for %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = root.Remove(tmpName)
		return fmt.Errorf("close root tmp for %s: %w", name, err)
	}
	if err := root.Rename(tmpName, name); err != nil {
		_ = root.Remove(tmpName)
		return fmt.Errorf("replace root file %s: %w", name, err)
	}
	if err := syncRootDir(root, dir); err != nil {
		return fmt.Errorf("sync root dir for %s: %w", name, err)
	}
	return nil
}

func createRootTemp(root *os.Root, dir string) (*os.File, string, error) {
	for range 100 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := filepath.Join(dir, ".atomic-"+hex.EncodeToString(random[:])+".tmp")
		f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, name, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not allocate a unique temporary file")
}
