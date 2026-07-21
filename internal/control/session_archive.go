package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reames-agent/internal/fileutil"
	"reames-agent/internal/guardian"
	"reames-agent/internal/store"
)

const sessionArchiveDir = ".archive"
const sessionArchiveManifest = "archive.json"

type sessionArchiveRecord struct {
	Version    int      `json:"version"`
	Key        string   `json:"key"`
	ArchivedAt int64    `json:"archivedAt"`
	Artifacts  []string `json:"artifacts"`
}

// ArchivedSession is one restorable session bundle in the shared store.
type ArchivedSession struct {
	Path       string
	ArchivedAt time.Time
}

var archiveRename = os.Rename

// ArchiveSession moves a complete, idle session bundle under the store's
// archive directory. The caller must stop its runtime and release its writer
// lease first. Every move is rolled back if a later artifact fails.
func ArchiveSession(sessionDir, sessionPath string) (ArchivedSession, error) {
	return ArchiveSessionBundle(sessionDir, sessionPath)
}

// ArchiveSessionBundle archives primaryPath and any related transcripts as one
// rollback-capable transaction. App-Server uses this for a stable thread whose
// origin transcript redirects to an active recovery transcript.
func ArchiveSessionBundle(sessionDir, primaryPath string, relatedPaths ...string) (ArchivedSession, error) {
	absDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return ArchivedSession{}, err
	}
	primary, key, err := validateLiveSessionPath(absDir, primaryPath)
	if err != nil {
		return ArchivedSession{}, err
	}
	paths := []string{primary}
	seenPaths := map[string]struct{}{CanonicalSessionPath(primary): {}}
	for _, related := range relatedPaths {
		if strings.TrimSpace(related) == "" {
			continue
		}
		path, _, err := validateLiveSessionPath(absDir, related)
		if err != nil {
			return ArchivedSession{}, err
		}
		canonical := CanonicalSessionPath(path)
		if _, exists := seenPaths[canonical]; exists {
			continue
		}
		seenPaths[canonical] = struct{}{}
		paths = append(paths, path)
	}
	lockOrder := append([]string(nil), paths...)
	sort.Slice(lockOrder, func(i, j int) bool { return CanonicalSessionPath(lockOrder[i]) < CanonicalSessionPath(lockOrder[j]) })
	guards := make([]*SessionRemovalGuard, 0, len(lockOrder))
	for _, path := range lockOrder {
		guard, err := TryAcquireSessionRemovalGuard(path)
		if err != nil {
			for _, acquired := range guards {
				acquired.Release()
			}
			return ArchivedSession{}, err
		}
		guards = append(guards, guard)
	}
	defer func() {
		for _, guard := range guards {
			guard.Release()
		}
	}()

	itemDir, ok := archiveRelativePath(absDir, filepath.Join(sessionArchiveDir, key))
	if !ok {
		return ArchivedSession{}, fmt.Errorf("unsafe session archive path")
	}
	if _, err := os.Lstat(itemDir); err == nil {
		return ArchivedSession{}, fmt.Errorf("session archive already exists: %s", key)
	} else if !os.IsNotExist(err) {
		return ArchivedSession{}, err
	}
	artifacts, err := sessionArchiveBundleArtifacts(absDir, primary, paths)
	if err != nil {
		return ArchivedSession{}, err
	}
	if len(artifacts) == 0 || artifacts[0] != key {
		return ArchivedSession{}, fmt.Errorf("session transcript is missing: %s", key)
	}
	record := sessionArchiveRecord{Version: 1, Key: key, ArchivedAt: time.Now().UnixMilli(), Artifacts: artifacts}
	if err := writeSessionArchiveRecord(itemDir, record); err != nil {
		return ArchivedSession{}, err
	}
	moved, err := moveArchiveArtifacts(absDir, itemDir, artifacts, false)
	if err != nil {
		rollbackErr := rollbackArchiveArtifacts(absDir, itemDir, moved, false)
		if rollbackErr == nil {
			_ = os.RemoveAll(itemDir)
		}
		return ArchivedSession{}, errors.Join(err, rollbackErr)
	}
	var guardErrs []error
	for _, guard := range guards {
		if err := guard.RemoveSidecarsAndRelease(); err != nil {
			guardErrs = append(guardErrs, err)
		}
	}
	if guardErr := errors.Join(guardErrs...); guardErr != nil {
		rollbackErr := rollbackArchiveArtifacts(absDir, itemDir, moved, false)
		if rollbackErr == nil {
			_ = os.RemoveAll(itemDir)
		}
		return ArchivedSession{}, errors.Join(guardErr, rollbackErr)
	}
	return ArchivedSession{Path: filepath.Join(itemDir, "files", key), ArchivedAt: time.UnixMilli(record.ArchivedAt)}, nil
}

func sessionArchiveBundleArtifacts(sessionDir, primary string, paths []string) ([]string, error) {
	seen := make(map[string]struct{})
	var artifacts []string
	for _, path := range paths {
		items, err := sessionArchiveArtifacts(sessionDir, path)
		if err != nil {
			return nil, err
		}
		for _, rel := range items {
			if _, exists := seen[rel]; exists {
				continue
			}
			seen[rel] = struct{}{}
			artifacts = append(artifacts, rel)
		}
	}
	primaryRel, err := filepath.Rel(sessionDir, primary)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(artifacts, func(i, j int) bool {
		if artifacts[i] == primaryRel {
			return true
		}
		if artifacts[j] == primaryRel {
			return false
		}
		return artifacts[i] < artifacts[j]
	})
	return artifacts, nil
}

// UnarchiveSession restores a previously archived bundle to its original
// relative paths. Existing live artifacts fail preflight before any move.
func UnarchiveSession(sessionDir, archivedPath string) (string, error) {
	itemDir, record, err := validateArchivedSessionPath(sessionDir, archivedPath)
	if err != nil {
		return "", err
	}
	for _, rel := range record.Artifacts {
		target, ok := archiveRelativePath(sessionDir, rel)
		if !ok {
			return "", fmt.Errorf("invalid archived artifact path %q", rel)
		}
		if _, err := os.Lstat(target); err == nil {
			return "", fmt.Errorf("session artifact already exists: %s", rel)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	moved, err := moveArchiveArtifacts(sessionDir, itemDir, record.Artifacts, true)
	if err != nil {
		return "", errors.Join(err, rollbackArchiveArtifacts(sessionDir, itemDir, moved, true))
	}
	if err := os.RemoveAll(itemDir); err != nil {
		return "", err
	}
	path, ok := archiveRelativePath(sessionDir, record.Key)
	if !ok {
		return "", fmt.Errorf("invalid restored session path")
	}
	return path, nil
}

// ListArchivedSessions returns validated archived transcripts newest-first.
func ListArchivedSessions(sessionDir string) ([]ArchivedSession, error) {
	root, ok := archiveRelativePath(sessionDir, sessionArchiveDir)
	if !ok {
		return nil, fmt.Errorf("unsafe session archive directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []ArchivedSession{}, nil
		}
		return nil, err
	}
	out := make([]ArchivedSession, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		itemDir := filepath.Join(root, entry.Name())
		record, err := readSessionArchiveRecord(itemDir)
		if err != nil || record.Key != entry.Name() {
			continue
		}
		path := filepath.Join(itemDir, "files", record.Key)
		if info, err := os.Lstat(path); err != nil || info.IsDir() {
			continue
		}
		out = append(out, ArchivedSession{Path: path, ArchivedAt: time.UnixMilli(record.ArchivedAt)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArchivedAt.After(out[j].ArchivedAt) })
	return out, nil
}

// DiscardSessionArtifacts removes a newly-created, unowned session bundle.
// It is intended for transactional rollback before a runtime or client has
// observed the session identity.
func DiscardSessionArtifacts(sessionDir, sessionPath string) error {
	path, _, err := validateLiveSessionPath(sessionDir, sessionPath)
	if err != nil {
		return err
	}
	guard, err := TryAcquireSessionRemovalGuard(path)
	if err != nil {
		return err
	}
	defer guard.Release()
	artifacts, err := sessionArchiveArtifacts(sessionDir, path)
	if err != nil {
		return err
	}
	var errs []error
	for _, rel := range artifacts {
		target, ok := archiveRelativePath(sessionDir, rel)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid discard path %q", rel))
			continue
		}
		if err := os.RemoveAll(target); err != nil {
			errs = append(errs, err)
		}
	}
	if err := guard.RemoveSidecarsAndRelease(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func sessionArchiveArtifacts(sessionDir, sessionPath string) ([]string, error) {
	candidates := []string{sessionPath}
	candidates = append(candidates, store.SessionSidecarFiles(sessionPath)...)
	candidates = append(candidates, store.SessionCheckpointDir(sessionPath), store.SessionJobsDir(sessionPath), sessionPath+".telemetry.json")
	guardianPath := guardian.PathFor(sessionPath)
	candidates = append(candidates, guardianPath)
	candidates = append(candidates, store.SessionSidecarFiles(guardianPath)...)
	subagents, err := ListSessionSubagentArtifacts(sessionDir, sessionPath)
	if err != nil {
		return nil, err
	}
	for _, artifact := range subagents {
		candidates = append(candidates, artifact.SessionPath, artifact.MetaPath, artifact.EffectPath)
		candidates = append(candidates, store.SessionSidecarFiles(artifact.SessionPath)...)
	}
	seen := make(map[string]struct{}, len(candidates))
	rels := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, err := os.Lstat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		rel, err := filepath.Rel(sessionDir, candidate)
		if err != nil || !safeArchiveRelative(rel) {
			return nil, fmt.Errorf("session artifact outside session dir: %s", candidate)
		}
		if _, exists := seen[rel]; exists {
			continue
		}
		seen[rel] = struct{}{}
		rels = append(rels, rel)
	}
	// The primary transcript must move first so a failed transaction can never
	// leave a discoverable live session with only a subset of its sidecars.
	key := filepath.Base(sessionPath)
	sort.SliceStable(rels, func(i, j int) bool {
		if rels[i] == key {
			return true
		}
		if rels[j] == key {
			return false
		}
		return rels[i] < rels[j]
	})
	return rels, nil
}

func moveArchiveArtifacts(sessionDir, itemDir string, artifacts []string, restore bool) ([]string, error) {
	moved := make([]string, 0, len(artifacts))
	for _, rel := range artifacts {
		live, ok := archiveRelativePath(sessionDir, rel)
		if !ok {
			return moved, fmt.Errorf("invalid archived artifact path %q", rel)
		}
		archiveRel, err := filepath.Rel(sessionDir, filepath.Join(itemDir, "files", rel))
		if err != nil {
			return moved, err
		}
		archived, ok := archiveRelativePath(sessionDir, archiveRel)
		if !ok {
			return moved, fmt.Errorf("unsafe archived artifact path %q", rel)
		}
		src, dst := live, archived
		if restore {
			src, dst = archived, live
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return moved, err
		}
		if err := archiveRename(src, dst); err != nil {
			return moved, fmt.Errorf("move session artifact %s: %w", rel, err)
		}
		moved = append(moved, rel)
	}
	return moved, nil
}

func rollbackArchiveArtifacts(sessionDir, itemDir string, moved []string, restore bool) error {
	var errs []error
	for i := len(moved) - 1; i >= 0; i-- {
		rel := moved[i]
		live, ok := archiveRelativePath(sessionDir, rel)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid rollback path %q", rel))
			continue
		}
		archiveRel, err := filepath.Rel(sessionDir, filepath.Join(itemDir, "files", rel))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		archived, ok := archiveRelativePath(sessionDir, archiveRel)
		if !ok {
			errs = append(errs, fmt.Errorf("unsafe archived rollback path %q", rel))
			continue
		}
		src, dst := archived, live
		if restore {
			src, dst = live, archived
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("rollback session artifact %s: %w", rel, err))
		}
	}
	return errors.Join(errs...)
}

func writeSessionArchiveRecord(itemDir string, record sessionArchiveRecord) error {
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(filepath.Join(itemDir, sessionArchiveManifest), append(raw, '\n'), 0o600)
}

func readSessionArchiveRecord(itemDir string) (sessionArchiveRecord, error) {
	raw, err := os.ReadFile(filepath.Join(itemDir, sessionArchiveManifest))
	if err != nil {
		return sessionArchiveRecord{}, err
	}
	var record sessionArchiveRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return sessionArchiveRecord{}, err
	}
	if record.Version != 1 || !store.IsSessionTranscriptName(record.Key) || len(record.Artifacts) == 0 {
		return sessionArchiveRecord{}, fmt.Errorf("invalid session archive manifest")
	}
	for _, rel := range record.Artifacts {
		if !safeArchiveRelative(rel) {
			return sessionArchiveRecord{}, fmt.Errorf("invalid archived artifact path %q", rel)
		}
	}
	return record, nil
}

func validateLiveSessionPath(sessionDir, sessionPath string) (string, string, error) {
	absDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", "", err
	}
	path := sessionPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(absDir, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	key := filepath.Base(path)
	rel, relErr := filepath.Rel(absDir, path)
	if relErr != nil || rel != key || !store.IsSessionTranscriptName(key) {
		return "", "", fmt.Errorf("session path outside session dir: %s", sessionPath)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("not a regular session file: %s", sessionPath)
	}
	return path, key, nil
}

func validateArchivedSessionPath(sessionDir, archivedPath string) (string, sessionArchiveRecord, error) {
	absDir, err := filepath.Abs(sessionDir)
	if err != nil {
		return "", sessionArchiveRecord{}, err
	}
	absRoot, ok := archiveRelativePath(absDir, sessionArchiveDir)
	if !ok {
		return "", sessionArchiveRecord{}, fmt.Errorf("unsafe session archive directory")
	}
	path, err := filepath.Abs(archivedPath)
	if err != nil {
		return "", sessionArchiveRecord{}, err
	}
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return "", sessionArchiveRecord{}, err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 3 || parts[1] != "files" || parts[2] != parts[0] || !store.IsSessionTranscriptName(parts[2]) {
		return "", sessionArchiveRecord{}, fmt.Errorf("invalid archived session path: %s", archivedPath)
	}
	itemDir := filepath.Join(absRoot, parts[0])
	record, err := readSessionArchiveRecord(itemDir)
	if err != nil || record.Key != parts[0] {
		if err == nil {
			err = fmt.Errorf("archive manifest identity mismatch")
		}
		return "", sessionArchiveRecord{}, err
	}
	return itemDir, record, nil
}

func archiveRelativePath(root, rel string) (string, bool) {
	if !safeArchiveRelative(rel) {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	target := filepath.Join(absRoot, rel)
	current := absRoot
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", false
		}
	}
	return target, true
}

func safeArchiveRelative(rel string) bool {
	rel = filepath.Clean(strings.TrimSpace(rel))
	return rel != "" && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
