package homebackup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	renameRestorePath = renameNoReplace
	syncRestoreParent = syncParent
)

type restoreStage struct {
	id        string
	target    string
	stage     string
	committed bool
}

// ReadManifest parses and validates manifest metadata without trusting or
// extracting any payload file. Use Verify before restore or evidence claims.
func ReadManifest(archive string) (Manifest, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return Manifest{}, fmt.Errorf("open backup archive: %w", err)
	}
	defer zr.Close()
	files, err := indexZipFiles(zr.File)
	if err != nil {
		return Manifest{}, err
	}
	manifestFile := files[manifestName]
	if manifestFile == nil {
		return Manifest{}, errors.New("backup archive is missing manifest.json")
	}
	if manifestFile.UncompressedSize64 > maxManifestBytes {
		return Manifest{}, errors.New("backup manifest is too large")
	}
	r, err := manifestFile.Open()
	if err != nil {
		return Manifest{}, err
	}
	defer r.Close()
	return decodeManifest(r)
}

// Verify checks archive self-consistency against the embedded manifest hashes
// and rejects extra, duplicate, non-portable, or special-file entries. It does
// not replace verification of an externally trusted checksum or signature.
func Verify(archive string) (Summary, error) {
	archiveFile, zr, digest, err := openArchive(archive)
	if err != nil {
		return Summary{}, err
	}
	defer archiveFile.Close()
	files, err := indexZipFiles(zr.File)
	if err != nil {
		return Summary{}, err
	}
	manifest, err := readManifestFromIndex(files)
	if err != nil {
		return Summary{}, err
	}
	expected := map[string]Entry{manifestName: {}}
	summary := Summary{Manifest: manifest}
	for _, entry := range manifest.Entries {
		name := dataName(entry)
		if entry.Type == "directory" {
			name += "/"
			summary.Directories++
		} else {
			summary.Files++
			summary.Bytes += entry.Size
		}
		expected[name] = entry
		file := files[name]
		if file == nil {
			return Summary{}, fmt.Errorf("backup payload is missing %s:%s", entry.Root, entry.Path)
		}
		if err := verifyZipEntry(file, entry); err != nil {
			return Summary{}, err
		}
	}
	for name := range files {
		if _, ok := expected[name]; !ok {
			return Summary{}, fmt.Errorf("backup archive contains undeclared entry %q", name)
		}
	}
	finalDigest, err := digestOpenFile(archiveFile)
	if err != nil {
		return Summary{}, fmt.Errorf("hash backup archive: %w", err)
	}
	if finalDigest != digest {
		return Summary{}, errors.New("backup archive changed during verification")
	}
	summary.ArchiveSHA256 = digest
	return summary, nil
}

// Restore verifies the full archive, extracts each logical root to a sibling
// staging directory, then renames the staged roots into previously absent
// targets. A failed later rename attempts to roll back earlier root commits.
func Restore(opts RestoreOptions) (Summary, error) {
	summary, err := Verify(opts.Archive)
	if err != nil {
		return Summary{}, err
	}
	targets, err := normalizeTargets(summary.Manifest, opts.Targets)
	if err != nil {
		return Summary{}, err
	}
	if opts.DryRun {
		return summary, nil
	}

	staged := make([]restoreStage, 0, len(targets))
	cleanupStages := func() {
		for _, root := range staged {
			if !root.committed && root.stage != "" {
				_ = os.RemoveAll(root.stage)
			}
		}
	}
	defer cleanupStages()
	for _, root := range targets {
		if err := os.MkdirAll(filepath.Dir(root.Path), 0o700); err != nil {
			return Summary{}, fmt.Errorf("create restore parent for %s: %w", root.ID, err)
		}
		stage, err := os.MkdirTemp(filepath.Dir(root.Path), ".reames-restore-*")
		if err != nil {
			return Summary{}, fmt.Errorf("create restore staging root for %s: %w", root.ID, err)
		}
		if err := os.Chmod(stage, 0o700); err != nil {
			_ = os.RemoveAll(stage)
			return Summary{}, err
		}
		staged = append(staged, restoreStage{id: root.ID, target: root.Path, stage: stage})
	}
	stageByID := map[string]string{}
	for _, root := range staged {
		stageByID[root.id] = root.stage
	}
	if err := extractVerified(opts.Archive, summary.Manifest, summary.ArchiveSHA256, stageByID); err != nil {
		return Summary{}, err
	}

	committed := 0
	for i := range staged {
		root := &staged[i]
		if _, err := os.Lstat(root.target); err == nil {
			rollbackErrs := rollbackCommittedRoots(staged[:committed])
			return Summary{}, errors.Join(append([]error{fmt.Errorf("restore target appeared during extraction: %s", root.target)}, rollbackErrs...)...)
		} else if !errors.Is(err, os.ErrNotExist) {
			rollbackErrs := rollbackCommittedRoots(staged[:committed])
			return Summary{}, errors.Join(append([]error{fmt.Errorf("recheck restore target %s: %w", root.target, err)}, rollbackErrs...)...)
		}
		if err := rejectSymlinkAncestors(root.target); err != nil {
			rollbackErrs := rollbackCommittedRoots(staged[:committed])
			return Summary{}, errors.Join(append([]error{fmt.Errorf("restore target changed before publish: %w", err)}, rollbackErrs...)...)
		}
		if err := renameRestorePath(root.stage, root.target); err != nil {
			rollbackErrs := rollbackCommittedRoots(staged[:committed])
			return Summary{}, errors.Join(append([]error{fmt.Errorf("publish restored root %s: %w", root.id, err)}, rollbackErrs...)...)
		}
		root.committed = true
		committed++
	}
	for _, root := range staged {
		if err := syncRestoreParent(root.target); err != nil {
			rollbackErrs := rollbackCommittedRoots(staged)
			return Summary{}, errors.Join(append([]error{err}, rollbackErrs...)...)
		}
	}
	return summary, nil
}

func rollbackCommittedRoots(staged []restoreStage) []error {
	var rollbackErrs []error
	for i := len(staged) - 1; i >= 0; i-- {
		if !staged[i].committed {
			continue
		}
		if err := renameRestorePath(staged[i].target, staged[i].stage); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback restored root %s: %w", staged[i].id, err))
			continue
		}
		staged[i].committed = false
	}
	return rollbackErrs
}

func readManifestFromIndex(files map[string]*zip.File) (Manifest, error) {
	manifestFile := files[manifestName]
	if manifestFile == nil {
		return Manifest{}, errors.New("backup archive is missing manifest.json")
	}
	if manifestFile.UncompressedSize64 > maxManifestBytes {
		return Manifest{}, errors.New("backup manifest is too large")
	}
	r, err := manifestFile.Open()
	if err != nil {
		return Manifest{}, err
	}
	defer r.Close()
	return decodeManifest(r)
}

func decodeManifest(r io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(io.LimitReader(r, maxManifestBytes+1))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode backup manifest: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("backup manifest contains trailing data")
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func indexZipFiles(in []*zip.File) (map[string]*zip.File, error) {
	if len(in) > maxEntries+1 {
		return nil, fmt.Errorf("backup archive has too many entries: %d", len(in))
	}
	files := make(map[string]*zip.File, len(in))
	folded := make(map[string]string, len(in))
	for _, file := range in {
		name := file.Name
		if name != manifestName {
			trimmed := strings.TrimSuffix(name, "/")
			if !strings.HasPrefix(trimmed, "data/") {
				return nil, fmt.Errorf("backup archive contains invalid entry %q", name)
			}
			if err := validateRelativePath(strings.TrimPrefix(trimmed, "data/")); err != nil {
				return nil, fmt.Errorf("unsafe archive entry %q: %w", name, err)
			}
		}
		key := portablePathKey(name)
		if prior, ok := folded[key]; ok {
			return nil, fmt.Errorf("backup archive has duplicate or case-colliding entries %q and %q", prior, name)
		}
		folded[key] = name
		files[name] = file
	}
	return files, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Format != Format || manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported backup format %q schema %d", manifest.Format, manifest.SchemaVersion)
	}
	if _, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt); err != nil {
		return fmt.Errorf("invalid backup creation time: %w", err)
	}
	if manifest.Secrets != "known-stores-excluded" {
		return fmt.Errorf("backup secret policy %q is unsupported; plaintext secret restore is forbidden", manifest.Secrets)
	}
	if len(manifest.Roots) == 0 || len(manifest.Entries) > maxEntries {
		return errors.New("backup manifest has an invalid root or entry count")
	}
	roots := map[string]bool{}
	previousRoot := ""
	for _, root := range manifest.Roots {
		if !validRootID(root.ID) || roots[root.ID] || previousRoot > root.ID {
			return fmt.Errorf("backup manifest has invalid, duplicate, or unsorted root %q", root.ID)
		}
		roots[root.ID] = true
		previousRoot = root.ID
	}
	seen := map[string]string{}
	entryTypes := map[string]string{}
	var total int64
	previous := ""
	for _, entry := range manifest.Entries {
		if !roots[entry.Root] {
			return fmt.Errorf("backup entry references unknown root %q", entry.Root)
		}
		if err := validateRelativePath(entry.Path); err != nil {
			return fmt.Errorf("invalid backup entry %s:%s: %w", entry.Root, entry.Path, err)
		}
		key := entry.Root + "/" + entry.Path
		fold := portablePathKey(key)
		if prior, ok := seen[fold]; ok {
			return fmt.Errorf("backup entries collide across platforms: %q and %q", prior, key)
		}
		seen[fold] = key
		entryTypes[fold] = entry.Type
		if previous > key {
			return errors.New("backup manifest entries are not sorted")
		}
		previous = key
		switch entry.Type {
		case "directory":
			if entry.Size != 0 || entry.SHA256 != "" {
				return fmt.Errorf("directory entry %s has file metadata", key)
			}
		case "file":
			if entry.Size < 0 || entry.Size > maxFileBytes || len(entry.SHA256) != 64 {
				return fmt.Errorf("file entry %s has invalid size or hash", key)
			}
			if _, err := hex.DecodeString(entry.SHA256); err != nil {
				return fmt.Errorf("file entry %s has invalid hash: %w", key, err)
			}
			total += entry.Size
			if total > maxTotalBytes {
				return errors.New("backup manifest exceeds the restore size limit")
			}
		default:
			return fmt.Errorf("backup entry %s has invalid type %q", key, entry.Type)
		}
		if _, err := time.Parse(time.RFC3339Nano, entry.ModifiedAt); err != nil {
			return fmt.Errorf("backup entry %s has invalid modification time", key)
		}
		if entry.Mode&^0o777 != 0 {
			return fmt.Errorf("backup entry %s has invalid permission bits", key)
		}
	}
	for key, kind := range seen {
		parts := strings.Split(key, "/")
		for i := 2; i < len(parts); i++ {
			prefix := strings.Join(parts[:i], "/")
			if prefixKind, ok := seen[prefix]; ok && entryTypes[prefix] == "file" {
				return fmt.Errorf("backup file %q conflicts with descendant %q", prefixKind, kind)
			}
		}
	}
	return nil
}

func verifyZipEntry(file *zip.File, entry Entry) error {
	mode := file.Mode()
	if mode&os.ModeSymlink != 0 || mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
		return fmt.Errorf("backup payload %s:%s is a special file", entry.Root, entry.Path)
	}
	if entry.Type == "directory" {
		if !strings.HasSuffix(file.Name, "/") {
			return fmt.Errorf("backup directory payload %s:%s is not a directory entry", entry.Root, entry.Path)
		}
		return nil
	}
	if file.UncompressedSize64 != uint64(entry.Size) {
		return fmt.Errorf("backup payload size mismatch for %s:%s", entry.Root, entry.Path)
	}
	if file.UncompressedSize64 > 1<<20 && (file.CompressedSize64 == 0 || file.UncompressedSize64/file.CompressedSize64 > 1000) {
		return fmt.Errorf("backup payload compression ratio is unsafe for %s:%s", entry.Root, entry.Path)
	}
	r, err := file.Open()
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(h, io.LimitReader(r, entry.Size+1))
	closeErr := r.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
		return fmt.Errorf("backup payload hash mismatch for %s:%s", entry.Root, entry.Path)
	}
	return nil
}

func normalizeTargets(manifest Manifest, targets map[string]string) ([]Root, error) {
	if len(targets) != len(manifest.Roots) {
		return nil, fmt.Errorf("restore needs exactly %d target roots", len(manifest.Roots))
	}
	var roots []Root
	for _, manifestRoot := range manifest.Roots {
		raw := strings.TrimSpace(targets[manifestRoot.ID])
		if raw == "" {
			return nil, fmt.Errorf("restore target for root %s is required", manifestRoot.ID)
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			return nil, err
		}
		if _, err := os.Lstat(abs); err == nil {
			return nil, fmt.Errorf("restore target must not exist: %s", abs)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := rejectSymlinkAncestors(abs); err != nil {
			return nil, err
		}
		roots = append(roots, Root{ID: manifestRoot.ID, Path: filepath.Clean(abs)})
	}
	for id := range targets {
		found := false
		for _, root := range manifest.Roots {
			found = found || root.ID == id
		}
		if !found {
			return nil, fmt.Errorf("restore target includes unknown root %q", id)
		}
	}
	for i := range roots {
		for j := i + 1; j < len(roots); j++ {
			if pathWithin(roots[i].Path, roots[j].Path) || pathWithin(roots[j].Path, roots[i].Path) {
				return nil, fmt.Errorf("restore targets %s and %s overlap", roots[i].ID, roots[j].ID)
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	return roots, nil
}

func rejectSymlinkAncestors(target string) error {
	current := filepath.Dir(target)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("restore target ancestor is not a real directory: %s", current)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect restore target ancestor %s: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func extractVerified(archive string, manifest Manifest, expectedDigest string, stages map[string]string) error {
	archiveFile, zr, digest, err := openArchive(archive)
	if err != nil {
		return err
	}
	defer archiveFile.Close()
	if digest != expectedDigest {
		return errors.New("backup archive changed after verification")
	}
	files, err := indexZipFiles(zr.File)
	if err != nil {
		return err
	}
	currentManifest, err := readManifestFromIndex(files)
	if err != nil {
		return err
	}
	expectedJSON, _ := json.Marshal(manifest)
	currentJSON, _ := json.Marshal(currentManifest)
	if string(expectedJSON) != string(currentJSON) {
		return errors.New("backup archive changed after verification")
	}
	var dirs []Entry
	for _, entry := range manifest.Entries {
		stage := stages[entry.Root]
		target := filepath.Join(stage, filepath.FromSlash(entry.Path))
		if !pathWithin(target, stage) {
			return fmt.Errorf("restore path escaped staging root: %s:%s", entry.Root, entry.Path)
		}
		if entry.Type == "directory" {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			dirs = append(dirs, entry)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		archiveEntry := files[dataName(entry)]
		if archiveEntry == nil {
			return fmt.Errorf("backup payload disappeared after verification: %s:%s", entry.Root, entry.Path)
		}
		if err := verifyZipEntry(archiveEntry, entry); err != nil {
			return err
		}
		in, err := archiveEntry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			in.Close()
			return err
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, entry.Size+1))
		syncErr := out.Sync()
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil || syncErr != nil || closeOutErr != nil || closeInErr != nil {
			return errors.Join(copyErr, syncErr, closeOutErr, closeInErr)
		}
		if n != entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
			return fmt.Errorf("restored payload hash mismatch for %s:%s", entry.Root, entry.Path)
		}
		if err := os.Chmod(target, restoreFileMode(entry.Mode)); err != nil {
			return err
		}
		if modified, err := time.Parse(time.RFC3339Nano, entry.ModifiedAt); err == nil {
			if err := os.Chtimes(target, modified, modified); err != nil {
				return err
			}
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i].Path, "/") > strings.Count(dirs[j].Path, "/")
	})
	for _, entry := range dirs {
		target := filepath.Join(stages[entry.Root], filepath.FromSlash(entry.Path))
		if err := os.Chmod(target, 0o700); err != nil {
			return err
		}
		if err := syncDirectory(target); err != nil {
			return fmt.Errorf("sync restored directory %s:%s: %w", entry.Root, entry.Path, err)
		}
	}
	for id, stage := range stages {
		if err := syncDirectory(stage); err != nil {
			return fmt.Errorf("sync restored root %s: %w", id, err)
		}
	}
	finalDigest, err := digestOpenFile(archiveFile)
	if err != nil {
		return err
	}
	if finalDigest != digest {
		return errors.New("backup archive changed during restore")
	}
	return nil
}

func restoreFileMode(mode uint32) os.FileMode {
	owner := os.FileMode(mode) & 0o700
	return owner | 0o600
}

func openArchive(path string) (*os.File, *zip.Reader, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open backup archive: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, "", err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, "", errors.New("backup archive must be a regular file")
	}
	digest, err := digestOpenFile(f)
	if err != nil {
		f.Close()
		return nil, nil, "", err
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		f.Close()
		return nil, nil, "", fmt.Errorf("open backup zip: %w", err)
	}
	return f, zr, digest, nil
}

func digestOpenFile(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
