// Package homebackup creates, verifies, and restores portable Reames Agent
// home/state backups. Known credential stores are excluded, but conversations,
// memory, and custom config can still contain sensitive user-provided text, so
// every archive must be handled as sensitive data.
package homebackup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

const (
	Format                 = "reames-agent-backup"
	SchemaVersion          = 1
	manifestName           = "manifest.json"
	maxManifestBytes       = 8 << 20
	maxEntries             = 200_000
	maxFileBytes     int64 = 8 << 30
	maxTotalBytes          = 50 << 30
	maxPathBytes           = 4096
)

var (
	publishBackupPath = renameNoReplace
	syncCreateParent  = syncParent
)

type Root struct {
	ID   string
	Path string
}

type ManifestRoot struct {
	ID string `json:"id"`
}

type Entry struct {
	Root       string `json:"root"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Mode       uint32 `json:"mode"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

type Exclusion struct {
	Root   string `json:"root"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type External struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type Manifest struct {
	Format           string         `json:"format"`
	SchemaVersion    int            `json:"schema_version"`
	CreatedAt        string         `json:"created_at"`
	CreatedByVersion string         `json:"created_by_version"`
	SourceOS         string         `json:"source_os"`
	SourceArch       string         `json:"source_arch"`
	Secrets          string         `json:"secrets"`
	Roots            []ManifestRoot `json:"roots"`
	Entries          []Entry        `json:"entries"`
	Excluded         []Exclusion    `json:"excluded,omitempty"`
	External         []External     `json:"external,omitempty"`
}

type CreateOptions struct {
	Roots            []Root
	Destination      string
	CreatedByVersion string
	Now              func() time.Time
}

type RestoreOptions struct {
	Archive string
	Targets map[string]string
	DryRun  bool
}

type Summary struct {
	ArchiveSHA256 string
	Files         int
	Directories   int
	Bytes         int64
	Manifest      Manifest
}

type scannedEntry struct {
	Entry
	sourcePath string
	sourceInfo os.FileInfo
}

// Create writes a self-consistent backup to a new restricted-mode archive. Known
// credential stores are excluded, but the archive remains sensitive.
func Create(opts CreateOptions) (Summary, error) {
	roots, err := normalizeRoots(opts.Roots)
	if err != nil {
		return Summary{}, err
	}
	destination, err := filepath.Abs(strings.TrimSpace(opts.Destination))
	if err != nil || strings.TrimSpace(opts.Destination) == "" {
		return Summary{}, errors.New("backup destination is required")
	}
	if err := rejectSymlinkAncestors(destination); err != nil {
		return Summary{}, fmt.Errorf("unsafe backup destination: %w", err)
	}
	for _, root := range roots {
		if pathWithin(destination, root.Path) {
			return Summary{}, fmt.Errorf("backup destination %s must be outside source root %s", destination, root.ID)
		}
	}
	if _, err := os.Lstat(destination); err == nil {
		return Summary{}, fmt.Errorf("backup destination already exists: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Summary{}, fmt.Errorf("inspect backup destination: %w", err)
	}

	entries, excluded, err := scanRoots(roots)
	if err != nil {
		return Summary{}, err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	manifest := Manifest{
		Format:           Format,
		SchemaVersion:    SchemaVersion,
		CreatedAt:        now().UTC().Format(time.RFC3339Nano),
		CreatedByVersion: strings.TrimSpace(opts.CreatedByVersion),
		SourceOS:         runtime.GOOS,
		SourceArch:       runtime.GOARCH,
		Secrets:          "known-stores-excluded",
		Excluded:         excluded,
		External: []External{
			{Kind: "credentials", Status: "known-stores-excluded-reconfigure-after-restore"},
			{Kind: "os-keyring", Status: "not-exported"},
		},
	}
	for _, root := range roots {
		manifest.Roots = append(manifest.Roots, ManifestRoot{ID: root.ID})
	}
	for _, entry := range entries {
		manifest.Entries = append(manifest.Entries, entry.Entry)
	}
	if err := validateManifest(manifest); err != nil {
		return Summary{}, err
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return Summary{}, fmt.Errorf("create backup directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".reames-backup-*.tmp")
	if err != nil {
		return Summary{}, fmt.Errorf("create backup temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		_ = tmp.Close()
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return Summary{}, fmt.Errorf("protect backup temp file: %w", err)
	}
	zw := zip.NewWriter(tmp)
	for _, entry := range entries {
		if err := writeScannedEntry(zw, entry); err != nil {
			_ = zw.Close()
			return Summary{}, err
		}
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = zw.Close()
		return Summary{}, err
	}
	manifestData = append(manifestData, '\n')
	header := &zip.FileHeader{Name: manifestName, Method: zip.Deflate}
	header.SetMode(0o600)
	header.Modified = zipEpoch()
	w, err := zw.CreateHeader(header)
	if err != nil {
		_ = zw.Close()
		return Summary{}, fmt.Errorf("create backup manifest: %w", err)
	}
	if _, err := w.Write(manifestData); err != nil {
		_ = zw.Close()
		return Summary{}, fmt.Errorf("write backup manifest: %w", err)
	}
	if err := zw.Close(); err != nil {
		return Summary{}, fmt.Errorf("close backup archive: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return Summary{}, fmt.Errorf("sync backup archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Summary{}, fmt.Errorf("close backup file: %w", err)
	}
	summary, err := Verify(tmpPath)
	if err != nil {
		return Summary{}, fmt.Errorf("verify created backup: %w", err)
	}
	if err := rejectSymlinkAncestors(destination); err != nil {
		return Summary{}, fmt.Errorf("backup destination changed before publish: %w", err)
	}
	if err := publishBackupPath(tmpPath, destination); err != nil {
		return Summary{}, fmt.Errorf("publish backup archive: %w", err)
	}
	removeTmp = false
	if err := syncCreateParent(destination); err != nil {
		removeErr := os.Remove(destination)
		if removeErr == nil {
			_ = syncCreateParent(destination)
			return Summary{}, err
		}
		return Summary{}, errors.Join(err, fmt.Errorf("remove incompletely published backup %s: %w", destination, removeErr))
	}
	return summary, nil
}

func normalizeRoots(in []Root) ([]Root, error) {
	if len(in) == 0 {
		return nil, errors.New("at least one backup root is required")
	}
	seenIDs := map[string]bool{}
	var roots []Root
	for _, root := range in {
		root.ID = strings.TrimSpace(root.ID)
		if !validRootID(root.ID) || seenIDs[root.ID] {
			return nil, fmt.Errorf("invalid or duplicate backup root id %q", root.ID)
		}
		seenIDs[root.ID] = true
		path := strings.TrimSpace(root.Path)
		if path == "" {
			return nil, fmt.Errorf("backup root %s has an empty path", root.ID)
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve backup root %s: %w", root.ID, err)
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, fmt.Errorf("inspect backup root %s: %w", root.ID, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("backup root %s must be a real directory, not a symlink or special file", root.ID)
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("resolve backup root %s symlinks: %w", root.ID, err)
		}
		roots = append(roots, Root{ID: root.ID, Path: filepath.Clean(resolved)})
	}
	for i := range roots {
		for j := i + 1; j < len(roots); j++ {
			if pathWithin(roots[i].Path, roots[j].Path) || pathWithin(roots[j].Path, roots[i].Path) {
				return nil, fmt.Errorf("backup roots %s and %s overlap; use one root for shared home/state", roots[i].ID, roots[j].ID)
			}
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	return roots, nil
}

func scanRoots(roots []Root) ([]scannedEntry, []Exclusion, error) {
	var entries []scannedEntry
	var excluded []Exclusion
	for _, root := range roots {
		err := filepath.WalkDir(root.Path, func(current string, dirEntry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if current == root.Path {
				return nil
			}
			rel, err := filepath.Rel(root.Path, current)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if err := validateRelativePath(rel); err != nil {
				return fmt.Errorf("unsafe source path %q: %w", rel, err)
			}
			if strings.HasSuffix(strings.ToLower(rel), ".jsonl.lease.json") {
				return fmt.Errorf("active or stale session lease found at %s:%s; close Reames Agent runtimes and reconcile leases before backup", root.ID, rel)
			}
			if reason := excludedReason(rel); reason != "" {
				excluded = append(excluded, Exclusion{Root: root.ID, Path: rel, Reason: reason})
				if dirEntry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := dirEntry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("backup refuses symlink %s:%s", root.ID, rel)
			}
			entry := scannedEntry{
				Entry: Entry{
					Root:       root.ID,
					Path:       rel,
					Mode:       uint32(info.Mode().Perm()),
					ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
				},
				sourcePath: current,
				sourceInfo: info,
			}
			switch {
			case info.IsDir():
				entry.Type = "directory"
			case info.Mode().IsRegular():
				entry.Type = "file"
				size, digest, err := hashFile(current, info)
				if err != nil {
					return err
				}
				entry.Size = size
				entry.SHA256 = digest
			default:
				return fmt.Errorf("backup refuses special file %s:%s", root.ID, rel)
			}
			entries = append(entries, entry)
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("scan backup root %s: %w", root.ID, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Root != entries[j].Root {
			return entries[i].Root < entries[j].Root
		}
		return entries[i].Path < entries[j].Path
	})
	sort.Slice(excluded, func(i, j int) bool {
		if excluded[i].Root != excluded[j].Root {
			return excluded[i].Root < excluded[j].Root
		}
		return excluded[i].Path < excluded[j].Path
	})
	return entries, excluded, nil
}

func writeScannedEntry(zw *zip.Writer, entry scannedEntry) error {
	name := dataName(entry.Entry)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.Modified = zipEpoch()
	if entry.Type == "directory" {
		header.Name += "/"
		header.SetMode(os.FileMode(entry.Mode) | os.ModeDir)
		_, err := zw.CreateHeader(header)
		return err
	}
	header.SetMode(os.FileMode(entry.Mode))
	w, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create archive entry %s:%s: %w", entry.Root, entry.Path, err)
	}
	f, err := openStableSource(entry.sourcePath, entry.sourceInfo)
	if err != nil {
		return fmt.Errorf("open backup source %s:%s: %w", entry.Root, entry.Path, err)
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(w, h), f)
	stableErr := stableSourcePath(entry.sourcePath, entry.sourceInfo)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("read backup source %s:%s: %w", entry.Root, entry.Path, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if stableErr != nil {
		return fmt.Errorf("backup source changed while being archived: %s:%s: %w", entry.Root, entry.Path, stableErr)
	}
	if n != entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
		return fmt.Errorf("backup source changed while being archived: %s:%s", entry.Root, entry.Path)
	}
	return nil
}

func hashFile(path string, expected os.FileInfo) (int64, string, error) {
	f, err := openStableSource(path, expected)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, copyErr := io.Copy(h, f)
	stableErr := stableSourcePath(path, expected)
	closeErr := f.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if stableErr != nil {
		return 0, "", stableErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func openStableSource(path string, expected os.FileInfo) (*os.File, error) {
	if err := stableSourcePath(path, expected); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(expected, opened) {
		f.Close()
		return nil, errors.New("source file identity changed before open")
	}
	return f, nil
}

func stableSourcePath(path string, expected os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(expected, current) {
		return errors.New("source path no longer names the scanned regular file")
	}
	return nil
}

func excludedReason(rel string) string {
	lower := strings.ToLower(filepath.ToSlash(rel))
	base := pathpkg.Base(lower)
	if lower == "cache" || strings.HasPrefix(lower, "cache/") {
		return "regenerable-cache"
	}
	if base == ".env" || base == "credentials" || base == "credentials.enc" {
		return "credential-secret"
	}
	if lower == "metrics-pending.json" || lower == "crash-pending.json" {
		return "ephemeral-runtime-file"
	}
	if lower == "weixin/accounts" || strings.HasPrefix(lower, "weixin/accounts/") {
		return "channel-secret"
	}
	if lower == "bot/pairing.json" {
		return "ephemeral-pairing-secret"
	}
	if strings.HasPrefix(base, ".atomic-") || strings.HasSuffix(base, ".tmp") || strings.HasSuffix(base, ".lock") || strings.HasSuffix(base, ".lease.json") {
		return "ephemeral-runtime-file"
	}
	return ""
}

func validRootID(id string) bool {
	if id == "" || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validateRelativePath(value string) error {
	if value == "" || len(value) > maxPathBytes || strings.ContainsRune(value, 0) || strings.Contains(value, `\`) {
		return errors.New("path is empty, too long, or contains a forbidden character")
	}
	if strings.HasPrefix(value, "/") || pathpkg.IsAbs(value) || pathpkg.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return errors.New("path is not a canonical relative path")
	}
	for _, component := range strings.Split(value, "/") {
		trimmed := strings.TrimRight(component, ". ")
		if trimmed != component || strings.Contains(component, ":") || windowsReservedName(trimmed) {
			return fmt.Errorf("path component %q is not portable", component)
		}
	}
	return nil
}

func windowsReservedName(component string) bool {
	name, _, _ := strings.Cut(strings.ToLower(component), ".")
	switch name {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(name) == 4 && (strings.HasPrefix(name, "com") || strings.HasPrefix(name, "lpt")) && name[3] >= '1' && name[3] <= '9' {
		return true
	}
	return false
}

func portablePathKey(value string) string {
	return strings.ToLower(norm.NFC.String(value))
}

func pathWithin(candidate, root string) bool {
	candidate, _ = filepath.Abs(candidate)
	root, _ = filepath.Abs(root)
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func dataName(entry Entry) string {
	return "data/" + entry.Root + "/" + entry.Path
}

func zipEpoch() time.Time {
	return time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
}

func syncParent(path string) error {
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync backup parent: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
