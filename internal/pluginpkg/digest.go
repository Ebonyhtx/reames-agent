package pluginpkg

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxPluginFiles = 4096
	maxPluginBytes = 64 << 20
	digestPrefix   = "sha256-tree-v1:"
)

type digestEntry struct {
	rel  string
	size int64
	mode os.FileMode
}

var beforeDigestFileOpen = func(string) error { return nil }

// ContentDigest hashes every regular package file except .git metadata. Paths,
// executable bits, sizes, and bytes are framed into a deterministic tree hash.
// Symlinks and special files are rejected so a package cannot redirect parsing
// or execution outside the approved tree after install.
func ContentDigest(root string) (string, error) {
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("plugin root %s must be a real directory", root)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rootHandle.Close()
	return contentDigestRoot(rootHandle)
}

func contentDigestRoot(rootHandle *os.Root) (string, error) {
	entries := make([]digestEntry, 0, 64)
	var total int64
	err := fs.WalkDir(rootHandle.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if entry.IsDir() {
			if strings.EqualFold(entry.Name(), ".git") {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin package contains symlink %s", path)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("plugin package contains special file %s", path)
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		total += fileInfo.Size()
		if total > maxPluginBytes {
			return fmt.Errorf("plugin package exceeds %d bytes", maxPluginBytes)
		}
		entries = append(entries, digestEntry{rel: path, size: fileInfo.Size(), mode: fileInfo.Mode()})
		if len(entries) > maxPluginFiles {
			return fmt.Errorf("plugin package exceeds %d files", maxPluginFiles)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	hash := sha256.New()
	for _, entry := range entries {
		executable := entry.mode.Perm() & 0o111
		_, _ = fmt.Fprintf(hash, "file\x00%s\x00%d\x00%03o\x00", entry.rel, entry.size, executable)
		if err := beforeDigestFileOpen(entry.rel); err != nil {
			return "", err
		}
		file, err := rootHandle.Open(filepath.FromSlash(entry.rel))
		if err != nil {
			return "", err
		}
		before, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return "", err
		}
		if !before.Mode().IsRegular() || before.Size() != entry.size || before.Mode().Perm()&0o111 != executable {
			_ = file.Close()
			return "", fmt.Errorf("plugin file changed while hashing: %s", entry.rel)
		}
		written, copyErr := io.Copy(hash, io.LimitReader(file, entry.size+1))
		after, statErr := file.Stat()
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if statErr != nil {
			return "", statErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if written != entry.size || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || after.Mode().Perm()&0o111 != executable {
			return "", fmt.Errorf("plugin file changed while hashing: %s", entry.rel)
		}
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%s%x", digestPrefix, hash.Sum(nil)), nil
}

func digestID(digest string) (string, error) {
	if !strings.HasPrefix(digest, digestPrefix) {
		return "", fmt.Errorf("unsupported plugin digest %q", digest)
	}
	id := strings.TrimPrefix(digest, digestPrefix)
	if len(id) != sha256.Size*2 {
		return "", fmt.Errorf("invalid plugin digest %q", digest)
	}
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return "", fmt.Errorf("invalid plugin digest %q", digest)
		}
	}
	return id, nil
}
