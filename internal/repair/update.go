package repair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

const updateTransactionVersion = 1

var repairExecutable = os.Executable

type UpdateTransaction struct {
	SchemaVersion int                     `json:"schemaVersion"`
	FromVersion   string                  `json:"fromVersion,omitempty"`
	ToVersion     string                  `json:"toVersion"`
	Platform      string                  `json:"platform"`
	TargetKind    string                  `json:"targetKind"`
	TargetPath    string                  `json:"targetPath"`
	BackupPath    string                  `json:"backupPath"`
	BackupSHA256  string                  `json:"backupSha256,omitempty"`
	Files         []UpdateTransactionFile `json:"files,omitempty"`
	CreatedAt     string                  `json:"createdAt"`
}

type UpdateTransactionFile struct {
	TargetPath    string `json:"targetPath"`
	BackupPath    string `json:"backupPath,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	MissingBefore bool   `json:"missingBefore,omitempty"`
}

type UpdateRollbackResult struct {
	RolledBack   bool   `json:"rolledBack"`
	FromVersion  string `json:"fromVersion,omitempty"`
	ToVersion    string `json:"toVersion,omitempty"`
	TargetPath   string `json:"targetPath,omitempty"`
	MixedInstall bool   `json:"mixedInstall,omitempty"`
}

func PendingUpdatePath() string {
	if root := config.MemoryUserDir(); root != "" {
		return filepath.Join(root, "repair", "pending-update.json")
	}
	return ""
}

func lockPendingUpdate() (func(), error) {
	path := PendingUpdatePath()
	if path == "" {
		return nil, fmt.Errorf("pending update: state directory is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return lockRepairStateFile(path)
}

// PrepareFileUpdate snapshots the complete installed release unit before the
// updater may replace any entry. Missing siblings are recorded so rollback can
// remove files introduced by the new release.
func PrepareFileUpdate(fromVersion, toVersion, targetPath string, siblingPaths ...string) (*UpdateTransaction, error) {
	targetPath = canonicalUpdatePath(targetPath)
	if targetPath == "" || targetPath == "." {
		return nil, fmt.Errorf("prepare update: empty target path")
	}
	root := config.MemoryUserDir()
	if root == "" {
		return nil, fmt.Errorf("prepare update: state directory is unavailable")
	}
	unlock, err := lockPendingUpdate()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := refusePendingUpdateOverwrite(); err != nil {
		return nil, err
	}
	backupDir := filepath.Join(root, "repair", "updates")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return nil, err
	}
	tx := &UpdateTransaction{
		SchemaVersion: updateTransactionVersion,
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		TargetKind:    "file",
		TargetPath:    targetPath,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	seen := map[string]bool{}
	paths := append([]string{targetPath}, siblingPaths...)
	for i, path := range paths {
		path = canonicalUpdatePath(path)
		if path == "" || path == "." || seen[path] {
			continue
		}
		seen[path] = true
		if i > 0 {
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					tx.Files = append(tx.Files, UpdateTransactionFile{TargetPath: path, MissingBefore: true})
					continue
				}
				return nil, fmt.Errorf("prepare update backup: %w", err)
			}
		}
		backupPath := filepath.Join(backupDir, filepath.Base(path)+".previous")
		hash, err := copyFileWithHash(path, backupPath, 0o700)
		if err != nil {
			return nil, fmt.Errorf("prepare update backup: %w", err)
		}
		file := UpdateTransactionFile{TargetPath: path, BackupPath: backupPath, SHA256: hash}
		tx.Files = append(tx.Files, file)
		if i == 0 {
			tx.BackupPath = backupPath
			tx.BackupSHA256 = hash
		}
	}
	if err := writePendingUpdate(tx); err != nil {
		return nil, err
	}
	return tx, nil
}

func PrepareAppBundleUpdate(fromVersion, toVersion, appPath, backupPath string) (*UpdateTransaction, error) {
	tx := &UpdateTransaction{
		SchemaVersion: updateTransactionVersion,
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		TargetKind:    "app-bundle",
		TargetPath:    filepath.Clean(strings.TrimSpace(appPath)),
		BackupPath:    filepath.Clean(strings.TrimSpace(backupPath)),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if !strings.HasSuffix(strings.ToLower(tx.TargetPath), ".app") || tx.BackupPath != tx.TargetPath+".reames-update-backup" {
		return nil, fmt.Errorf("prepare update: invalid macOS bundle paths")
	}
	unlock, err := lockPendingUpdate()
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := refusePendingUpdateOverwrite(); err != nil {
		return nil, err
	}
	if err := writePendingUpdate(tx); err != nil {
		return nil, err
	}
	return tx, nil
}

func refusePendingUpdateOverwrite() error {
	path := PendingUpdatePath()
	if path == "" {
		return fmt.Errorf("prepare update: state directory is unavailable")
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("prepare update: an earlier pending update must be committed, cancelled, or rolled back first")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("prepare update: inspect pending transaction: %w", err)
	}
	return nil
}

func ReadPendingUpdate() (*UpdateTransaction, error) {
	path := PendingUpdatePath()
	if path == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tx UpdateTransaction
	if err := json.Unmarshal(b, &tx); err != nil {
		return nil, err
	}
	if err := validateUpdateTransaction(&tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

func HasPendingUpdate() bool {
	_, err := ReadPendingUpdate()
	return err == nil
}

func MarkUpdateHealthy(runningVersion string) error {
	unlock, err := lockPendingUpdate()
	if err != nil {
		if PendingUpdatePath() == "" {
			return nil
		}
		return err
	}
	defer unlock()
	tx, err := ReadPendingUpdate()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(runningVersion) != strings.TrimSpace(tx.ToVersion) {
		return nil
	}
	running, executableErr := repairExecutable()
	if executableErr != nil {
		return executableErr
	}
	running = canonicalUpdatePath(running)
	if tx.TargetKind == "file" && running != tx.TargetPath {
		return nil
	}
	if tx.TargetKind == "app-bundle" && !strings.HasPrefix(running, tx.TargetPath+string(filepath.Separator)) {
		return nil
	}
	if err := os.Remove(PendingUpdatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	removeUpdateBackups(tx)
	return nil
}

func CancelPendingUpdate(toVersion string) error {
	unlock, err := lockPendingUpdate()
	if err != nil {
		return err
	}
	defer unlock()
	tx, err := ReadPendingUpdate()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(toVersion) != strings.TrimSpace(tx.ToVersion) {
		return nil
	}
	if err := os.Remove(PendingUpdatePath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	removeUpdateBackups(tx)
	return nil
}

func RollbackPendingUpdate() (UpdateRollbackResult, error) {
	return rollbackPendingUpdate("", "")
}

// RollbackPendingUpdateIfCurrent rolls back only the exact transaction the
// caller already attributed to a failed startup. The identity is rechecked
// while the pending-update lock is held; a changed transaction returns a
// zero, non-error result so the caller can fail closed without mutating it.
func RollbackPendingUpdateIfCurrent(toVersion, createdAt string) (UpdateRollbackResult, error) {
	if strings.TrimSpace(toVersion) == "" || strings.TrimSpace(createdAt) == "" {
		return UpdateRollbackResult{}, fmt.Errorf("rollback update: expected transaction identity is incomplete")
	}
	return rollbackPendingUpdate(toVersion, createdAt)
}

func rollbackPendingUpdate(expectedVersion, expectedCreatedAt string) (UpdateRollbackResult, error) {
	unlock, err := lockPendingUpdate()
	if err != nil {
		return UpdateRollbackResult{}, err
	}
	defer unlock()
	tx, err := ReadPendingUpdate()
	if err != nil {
		if os.IsNotExist(err) {
			return UpdateRollbackResult{}, nil
		}
		return UpdateRollbackResult{}, err
	}
	if expectedVersion != "" && strings.TrimSpace(tx.ToVersion) != strings.TrimSpace(expectedVersion) {
		return UpdateRollbackResult{}, nil
	}
	if expectedCreatedAt != "" && strings.TrimSpace(tx.CreatedAt) != strings.TrimSpace(expectedCreatedAt) {
		return UpdateRollbackResult{}, nil
	}
	result := UpdateRollbackResult{FromVersion: tx.ToVersion, ToVersion: tx.FromVersion, TargetPath: tx.TargetPath}
	switch tx.TargetKind {
	case "file":
		files := tx.Files
		if len(files) == 0 {
			files = []UpdateTransactionFile{{TargetPath: tx.TargetPath, BackupPath: tx.BackupPath, SHA256: tx.BackupSHA256}}
		}
		for _, file := range files {
			if file.MissingBefore {
				continue
			}
			got, hashErr := hashFile(file.BackupPath)
			if hashErr != nil || file.SHA256 == "" || !strings.EqualFold(got, file.SHA256) {
				return result, fmt.Errorf("rollback update: backup hash mismatch for %s", filepath.Base(file.TargetPath))
			}
		}
		mixed, restoreErr := restoreReleaseUnit(files)
		if restoreErr != nil {
			result.MixedInstall = mixed
			return result, fmt.Errorf("rollback update: %w", restoreErr)
		}
	case "app-bundle":
		if _, err := os.Stat(tx.BackupPath); err != nil {
			return result, fmt.Errorf("rollback update: backup bundle: %w", err)
		}
		failed := tx.TargetPath + ".reames-failed-" + time.Now().UTC().Format("20060102T150405Z")
		if err := os.Rename(tx.TargetPath, failed); err != nil {
			return result, fmt.Errorf("rollback update: move failed bundle: %w", err)
		}
		if err := os.Rename(tx.BackupPath, tx.TargetPath); err != nil {
			_ = os.Rename(failed, tx.TargetPath)
			return result, fmt.Errorf("rollback update: restore bundle: %w", err)
		}
	default:
		return result, fmt.Errorf("rollback update: unsupported target kind %q", tx.TargetKind)
	}
	result.RolledBack = true
	_ = os.Remove(PendingUpdatePath())
	return result, nil
}

var (
	rollbackStageCopy  = copyFileWithHash
	rollbackSwapRename = os.Rename
)

func restoreReleaseUnit(files []UpdateTransactionFile) (mixed bool, err error) {
	stages := make([]string, len(files))
	defer func() {
		for _, stage := range stages {
			if stage != "" {
				_ = os.Remove(stage)
			}
		}
	}()
	for i, file := range files {
		if file.MissingBefore {
			continue
		}
		mode := os.FileMode(0o700)
		if st, statErr := os.Stat(file.TargetPath); statErr == nil {
			mode = st.Mode().Perm()
		}
		stage := file.TargetPath + ".reames-rollback-stage"
		if _, copyErr := rollbackStageCopy(file.BackupPath, stage, mode); copyErr != nil {
			return false, fmt.Errorf("stage %s: %w", filepath.Base(file.TargetPath), copyErr)
		}
		stages[i] = stage
	}
	asides := make([]string, len(files))
	processed := make([]bool, len(files))
	restoreAttempted := make([]bool, len(files))
	failedIndex := -1
	var swapErr error
	for i, file := range files {
		aside := file.TargetPath + ".reames-rollback-aside"
		if renameErr := rollbackSwapRename(file.TargetPath, aside); renameErr != nil {
			if os.IsNotExist(renameErr) {
				if file.MissingBefore {
					aside = ""
				} else if _, statErr := os.Lstat(aside); statErr != nil {
					aside = ""
				}
			} else {
				failedIndex = i
				swapErr = fmt.Errorf("retain %s: %w", filepath.Base(file.TargetPath), renameErr)
				break
			}
		}
		asides[i] = aside
		if file.MissingBefore {
			processed[i] = true
			continue
		}
		restoreAttempted[i] = true
		if renameErr := rollbackSwapRename(stages[i], file.TargetPath); renameErr != nil {
			failedIndex = i
			swapErr = fmt.Errorf("restore %s: %w", filepath.Base(file.TargetPath), renameErr)
			break
		}
		stages[i] = ""
		processed[i] = true
	}
	if swapErr == nil {
		for _, file := range files {
			_ = os.Remove(file.TargetPath + ".reames-rollback-aside")
		}
		return false, nil
	}
	for i, file := range files {
		if !processed[i] && i != failedIndex {
			continue
		}
		if asides[i] != "" {
			if rollbackSwapRename(asides[i], file.TargetPath) != nil {
				mixed = true
			}
			continue
		}
		if !file.MissingBefore && restoreAttempted[i] {
			mixed = true
		}
	}
	return mixed, swapErr
}

func allowedUpdateTargetBase(base string, primary bool) bool {
	switch strings.ToLower(base) {
	case "reames-agent-desktop", "reames-agent-desktop.exe", "reames-agent", "reames-agent.exe":
		return primary
	case "reames agent.exe", "reames-agent-guard", "reames-agent-guard.exe", "reames-agent-launcher.exe", "reames-agent-update-helper.exe":
		return !primary
	default:
		return false
	}
}

func validateUpdateTransaction(tx *UpdateTransaction) error {
	if tx == nil || tx.SchemaVersion != updateTransactionVersion || strings.TrimSpace(tx.ToVersion) == "" {
		return fmt.Errorf("pending update metadata is incomplete")
	}
	tx.TargetPath = filepath.Clean(tx.TargetPath)
	tx.BackupPath = filepath.Clean(tx.BackupPath)
	launcher, err := repairExecutable()
	if err != nil {
		return fmt.Errorf("pending update launcher path is unavailable")
	}
	if resolved, resolveErr := filepath.EvalSymlinks(launcher); resolveErr == nil {
		launcher = resolved
	}
	launcher = filepath.Clean(launcher)
	switch tx.TargetKind {
	case "file":
		if !allowedUpdateTargetBase(filepath.Base(tx.TargetPath), true) {
			return fmt.Errorf("pending update target is not a Reames Agent executable")
		}
		if filepath.Dir(launcher) != filepath.Dir(tx.TargetPath) {
			return fmt.Errorf("pending update target is outside the current Guard installation")
		}
		repairRoot := filepath.Clean(filepath.Join(config.MemoryUserDir(), "repair"))
		insideRepair := func(path string) bool {
			rel, err := filepath.Rel(repairRoot, filepath.Clean(path))
			return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		}
		if tx.BackupSHA256 == "" || !insideRepair(tx.BackupPath) {
			return fmt.Errorf("pending update primary backup metadata is invalid")
		}
		primaryListed := len(tx.Files) == 0
		for i := range tx.Files {
			file := &tx.Files[i]
			file.TargetPath = filepath.Clean(file.TargetPath)
			primary := file.TargetPath == tx.TargetPath
			primaryListed = primaryListed || primary
			if !allowedUpdateTargetBase(filepath.Base(file.TargetPath), primary) || filepath.Dir(file.TargetPath) != filepath.Dir(tx.TargetPath) {
				return fmt.Errorf("pending update release file is invalid")
			}
			if file.MissingBefore {
				if primary || file.BackupPath != "" || file.SHA256 != "" {
					return fmt.Errorf("pending update missing release file metadata is invalid")
				}
				continue
			}
			file.BackupPath = filepath.Clean(file.BackupPath)
			if file.SHA256 == "" || !insideRepair(file.BackupPath) {
				return fmt.Errorf("pending update release backup metadata is invalid")
			}
		}
		if !primaryListed {
			return fmt.Errorf("pending update release unit omits the primary executable")
		}
	case "app-bundle":
		if !strings.HasSuffix(strings.ToLower(tx.TargetPath), ".app") || tx.BackupPath != tx.TargetPath+".reames-update-backup" {
			return fmt.Errorf("pending update bundle paths are invalid")
		}
		if !strings.HasPrefix(launcher, tx.TargetPath+string(filepath.Separator)) {
			return fmt.Errorf("pending update bundle is not the current Guard installation")
		}
	default:
		return fmt.Errorf("pending update target kind is invalid")
	}
	return nil
}

func writePendingUpdate(tx *UpdateTransaction) error {
	path := PendingUpdatePath()
	if path == "" {
		return fmt.Errorf("pending update: state directory is unavailable")
	}
	b, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(b, '\n'), 0o600)
}

func removeUpdateBackups(tx *UpdateTransaction) {
	if tx == nil {
		return
	}
	if tx.TargetKind == "app-bundle" {
		if tx.BackupPath != "" {
			_ = os.RemoveAll(tx.BackupPath)
		}
		return
	}
	for _, file := range tx.Files {
		if file.BackupPath != "" {
			_ = os.Remove(file.BackupPath)
		}
	}
	if len(tx.Files) == 0 && tx.BackupPath != "" {
		_ = os.Remove(tx.BackupPath)
	}
}

func copyFileWithHash(src, dst string, mode os.FileMode) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".repair-copy-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), in); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := fileutil.ReplaceFile(tmpPath, dst); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func canonicalUpdatePath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return path
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}
