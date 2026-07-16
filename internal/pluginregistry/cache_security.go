package pluginregistry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ensurePrivateDirectory creates and validates path itself. Its ancestors are
// deliberately outside this function's trust boundary: macOS commonly exposes
// /var through a symlink to /private/var, and a configured cache below it is
// still safe as long as the cache base itself and every descendant are checked.
func ensurePrivateDirectory(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if unsafe, reason := unsafePathEntry(abs, info); unsafe {
		return "", fmt.Errorf("%s is unsafe: %s", abs, reason)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return "", fmt.Errorf("make cache directory owner-private: %w", err)
	}
	return abs, nil
}

// ensurePrivateSubdirectory walks only within base so no cache-owned relative
// path component can redirect creation through a link or reparse point.
func ensurePrivateSubdirectory(base, relative string) (string, error) {
	base, err := ensurePrivateDirectory(base)
	if err != nil {
		return "", err
	}
	current := base
	clean := filepath.Clean(relative)
	if clean == "." {
		return base, nil
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q is not a relative cache path", relative)
	}
	for _, component := range strings.Split(clean, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current, err = ensurePrivateDirectory(filepath.Join(current, component))
		if err != nil {
			return "", err
		}
	}
	return current, nil
}

// hardenCacheTree validates the complete cache after the per-registry lock is
// held. This rejects stale malicious links before go-tuf reads or overwrites
// cache entries, and tightens permissions on all cache-owned entries.
func hardenCacheTree(root string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		if unsafe, reason := unsafePathEntry(name, info); unsafe {
			return fmt.Errorf("%s is unsafe: %s", name, reason)
		}
		switch {
		case info.IsDir():
			if err := os.Chmod(name, 0o700); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := os.Chmod(name, 0o600); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s is not a regular file or directory", name)
		}
		return nil
	})
}

func secureCacheDestination(base, target string) (string, error) {
	if err := validateTargetPath(target); err != nil {
		return "", err
	}
	base, err := ensurePrivateDirectory(base)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(base, filepath.FromSlash(target))
	parentRelative, err := filepath.Rel(base, filepath.Dir(destination))
	if err != nil {
		return "", err
	}
	if _, err := ensurePrivateSubdirectory(base, parentRelative); err != nil {
		return "", err
	}
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return destination, nil
	}
	if err != nil {
		return "", err
	}
	if unsafe, reason := unsafePathEntry(destination, info); unsafe {
		return "", fmt.Errorf("%s is unsafe: %s", destination, reason)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", destination)
	}
	return destination, nil
}

func openPrivateCacheFile(path string) (*os.File, error) {
	for attempt := 0; attempt < 2; attempt++ {
		before, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			file, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if errors.Is(openErr, os.ErrExist) {
				continue
			}
			if openErr != nil {
				return nil, openErr
			}
			if err := validateOpenedCacheFile(path, file); err != nil {
				_ = file.Close()
				return nil, err
			}
			return file, nil
		}
		if err != nil {
			return nil, err
		}
		if unsafe, reason := unsafePathEntry(path, before); unsafe {
			return nil, fmt.Errorf("%s is unsafe: %s", path, reason)
		}
		if !before.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", path)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		if err := validateOpenedCacheFile(path, file); err != nil {
			_ = file.Close()
			return nil, err
		}
		return file, nil
	}
	return nil, fmt.Errorf("cache file %s changed while opening", path)
}

func validateOpenedCacheFile(path string, file *os.File) error {
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if unsafe, reason := unsafePathEntry(path, current); unsafe {
		return fmt.Errorf("%s is unsafe: %s", path, reason)
	}
	if !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return fmt.Errorf("%s changed or is not a regular file", path)
	}
	return file.Chmod(0o600)
}
