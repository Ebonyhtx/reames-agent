package repair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

const configSnapshotRetention = 5

type ConfigCheck struct {
	Scope        string `json:"scope"`
	Path         string `json:"path"`
	Exists       bool   `json:"exists"`
	Valid        bool   `json:"valid"`
	Error        string `json:"error,omitempty"`
	SnapshotPath string `json:"snapshotPath,omitempty"`
}

type ConfigReport struct {
	Checks      []ConfigCheck      `json:"checks"`
	Applied     []string           `json:"applied"`
	Transaction *RepairTransaction `json:"transaction,omitempty"`
}

type ConfigOptions struct {
	Root           string
	Apply          bool
	IncludeProject bool
	OnlyScope      string
	Now            func() time.Time
}

type RepairChange struct {
	Scope         string `json:"scope"`
	TargetPath    string `json:"targetPath"`
	PreviousPath  string `json:"previousPath,omitempty"`
	MissingBefore bool   `json:"missingBefore,omitempty"`
	Undone        bool   `json:"undone,omitempty"`
}

type RepairTransaction struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	CreatedAt     string         `json:"createdAt"`
	Changes       []RepairChange `json:"changes"`
	Undone        bool           `json:"undone,omitempty"`
	UndoneAt      string         `json:"undoneAt,omitempty"`
}

type ConfigSnapshot struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	SourcePath    string `json:"sourcePath"`
	RecordedAt    string `json:"recordedAt"`
	Version       string `json:"version,omitempty"`
}

func InspectAndRepairConfig(opts ConfigOptions) (ConfigReport, error) {
	if opts.OnlyScope != "" && opts.OnlyScope != "global" && opts.OnlyScope != "project" {
		return ConfigReport{}, fmt.Errorf("unknown config repair scope %q", opts.OnlyScope)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	root := strings.TrimSpace(opts.Root)
	items := []struct{ scope, path string }{{scope: "global", path: config.UserConfigPath()}}
	if root != "" {
		project := "reames-agent.toml"
		if root != "." {
			project = filepath.Join(filepath.Clean(root), project)
		}
		items = append(items, struct{ scope, path string }{scope: "project", path: project})
	}
	report := ConfigReport{Checks: make([]ConfigCheck, 0, len(items)), Applied: []string{}}
	tx := newRepairTransaction(opts.Now())
	for _, item := range items {
		check := inspectConfig(item.scope, item.path)
		if item.scope == "global" {
			check.SnapshotPath = latestSnapshotPath()
		}
		report.Checks = append(report.Checks, check)
		if !opts.Apply || !check.Exists || check.Valid || (opts.OnlyScope != "" && opts.OnlyScope != item.scope) || (item.scope == "project" && !opts.IncludeProject) {
			continue
		}
		quarantine := item.path + ".reames-quarantine-" + opts.Now().UTC().Format("20060102T150405.000000000Z")
		if err := os.Rename(item.path, quarantine); err != nil {
			return report, fmt.Errorf("quarantine %s config: %w", item.scope, err)
		}
		change := RepairChange{Scope: item.scope, TargetPath: item.path, PreviousPath: quarantine}
		tx.Changes = append(tx.Changes, change)
		if err := persistRepairTransaction(tx); err != nil {
			_ = os.Rename(quarantine, item.path)
			return report, err
		}
		report.Applied = append(report.Applied, "quarantined "+item.scope+" config at "+quarantine)
		if item.scope == "global" {
			if snap, ok := latestValidConfigSnapshot(); ok {
				if err := restoreSnapshotBytes(snap, item.path); err != nil {
					return report, err
				}
				report.Applied = append(report.Applied, "restored global config from snapshot "+snap.ID)
			}
		}
		report.Checks[len(report.Checks)-1] = inspectConfig(item.scope, item.path)
	}
	if len(tx.Changes) > 0 {
		report.Transaction = tx
		appendRepairLogBestEffort(tx)
	}
	return report, nil
}

func inspectConfig(scope, path string) ConfigCheck {
	check := ConfigCheck{Scope: scope, Path: path, Valid: true}
	if strings.TrimSpace(path) == "" {
		return check
	}
	if _, err := os.Lstat(path); err != nil {
		if !os.IsNotExist(err) {
			check.Valid = false
			check.Error = err.Error()
		}
		return check
	}
	check.Exists = true
	if err := config.ValidateFile(path); err != nil {
		check.Valid = false
		check.Error = err.Error()
	}
	return check
}

func RecordHealthyConfig(version string) error {
	path := config.UserConfigPath()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := config.ValidateFile(path); err != nil {
		return err
	}
	dir := snapshotDir()
	if dir == "" {
		return nil
	}
	sum := sha256.Sum256(b)
	digest := hex.EncodeToString(sum[:])
	existing, _ := ListConfigSnapshots()
	if len(existing) > 0 && strings.EqualFold(existing[0].SHA256, digest) {
		return nil
	}
	now := time.Now().UTC()
	id := now.Format("20060102T150405.000000000Z") + "-" + digest[:12]
	snapshotPath := filepath.Join(dir, id+".toml")
	if err := fileutil.AtomicWriteFile(snapshotPath, b, 0o600); err != nil {
		return err
	}
	meta := ConfigSnapshot{
		SchemaVersion: 1,
		ID:            id,
		Path:          snapshotPath,
		SHA256:        digest,
		SourcePath:    path,
		RecordedAt:    now.Format(time.RFC3339Nano),
		Version:       version,
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := fileutil.AtomicWriteFile(snapshotPath+".json", append(encoded, '\n'), 0o600); err != nil {
		_ = os.Remove(snapshotPath)
		return err
	}
	return pruneConfigSnapshots(configSnapshotRetention)
}

func ListConfigSnapshots() ([]ConfigSnapshot, error) {
	dir := snapshotDir()
	if dir == "" {
		return []ConfigSnapshot{}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ConfigSnapshot{}, nil
		}
		return nil, err
	}
	var out []ConfigSnapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml.json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var snap ConfigSnapshot
		if json.Unmarshal(b, &snap) != nil || validateSnapshotMetadata(dir, &snap) != nil {
			continue
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt > out[j].RecordedAt })
	return out, nil
}

func RestoreConfigSnapshot(id string) (*RepairTransaction, error) {
	snapshots, err := ListConfigSnapshots()
	if err != nil {
		return nil, err
	}
	var selected *ConfigSnapshot
	for i := range snapshots {
		if snapshots[i].ID == strings.TrimSpace(id) {
			selected = &snapshots[i]
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("config snapshot %q not found", id)
	}
	if err := verifyConfigSnapshot(*selected); err != nil {
		return nil, err
	}
	dest := config.UserConfigPath()
	if dest == "" {
		return nil, errors.New("global config path is unavailable")
	}
	tx := newRepairTransaction(time.Now())
	quarantine := dest + ".reames-restore-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, quarantine); err != nil {
			return nil, err
		}
		tx.Changes = append(tx.Changes, RepairChange{Scope: "global", TargetPath: dest, PreviousPath: quarantine})
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		tx.Changes = append(tx.Changes, RepairChange{Scope: "global", TargetPath: dest, MissingBefore: true})
	}
	if err := restoreSnapshotBytes(*selected, dest); err != nil {
		if !tx.Changes[0].MissingBefore {
			_ = os.Rename(quarantine, dest)
		}
		return nil, err
	}
	if err := persistRepairTransaction(tx); err != nil {
		_ = os.Remove(dest)
		if !tx.Changes[0].MissingBefore {
			_ = os.Rename(quarantine, dest)
		}
		return nil, err
	}
	appendRepairLogBestEffort(tx)
	return tx, nil
}

func UndoLastRepair() (*RepairTransaction, error) {
	tx, err := ReadLastRepair()
	if err != nil {
		return nil, err
	}
	if tx.Undone {
		return nil, fmt.Errorf("repair %s was already undone", tx.ID)
	}
	for i := len(tx.Changes) - 1; i >= 0; i-- {
		change := &tx.Changes[i]
		if change.Undone {
			continue
		}
		if err := validateRepairChange(*change); err != nil {
			return nil, err
		}
		if change.MissingBefore {
			redo := ""
			if _, err := os.Lstat(change.TargetPath); err == nil {
				redo = change.TargetPath + ".reames-redo-" + time.Now().UTC().Format("20060102T150405.000000000Z")
				if err := os.Rename(change.TargetPath, redo); err != nil {
					return nil, err
				}
			} else if !os.IsNotExist(err) {
				return nil, err
			}
			change.Undone = true
			if err := persistRepairTransaction(tx); err != nil {
				change.Undone = false
				if redo != "" {
					_ = os.Rename(redo, change.TargetPath)
				}
				return nil, err
			}
			continue
		}
		if _, err := os.Lstat(change.PreviousPath); err != nil {
			return nil, fmt.Errorf("undo repair: previous file: %w", err)
		}
		redo := ""
		if _, err := os.Lstat(change.TargetPath); err == nil {
			redo = change.TargetPath + ".reames-redo-" + time.Now().UTC().Format("20060102T150405.000000000Z")
			if err := os.Rename(change.TargetPath, redo); err != nil {
				return nil, err
			}
		}
		if err := os.Rename(change.PreviousPath, change.TargetPath); err != nil {
			if redo != "" {
				_ = os.Rename(redo, change.TargetPath)
			}
			return nil, err
		}
		change.Undone = true
		if err := persistRepairTransaction(tx); err != nil {
			change.Undone = false
			_ = os.Rename(change.TargetPath, change.PreviousPath)
			if redo != "" {
				_ = os.Rename(redo, change.TargetPath)
			}
			return nil, err
		}
	}
	tx.Undone = true
	tx.UndoneAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := persistRepairTransaction(tx); err != nil {
		return nil, err
	}
	appendRepairLogBestEffort(tx)
	return tx, nil
}

func ReadLastRepair() (*RepairTransaction, error) {
	path := repairTransactionPath()
	if path == "" {
		return nil, os.ErrNotExist
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tx RepairTransaction
	if err := json.Unmarshal(b, &tx); err != nil {
		return nil, err
	}
	if tx.SchemaVersion != 1 || tx.ID == "" || len(tx.Changes) == 0 {
		return nil, errors.New("last repair transaction is incomplete")
	}
	return &tx, nil
}

func newRepairTransaction(now time.Time) *RepairTransaction {
	now = now.UTC()
	return &RepairTransaction{SchemaVersion: 1, ID: fmt.Sprintf("repair-%d", now.UnixNano()), CreatedAt: now.Format(time.RFC3339Nano), Changes: []RepairChange{}}
}

func validateRepairChange(change RepairChange) error {
	target := filepath.Clean(change.TargetPath)
	switch change.Scope {
	case "global":
		if target != filepath.Clean(config.UserConfigPath()) {
			return errors.New("repair global target is invalid")
		}
	case "project":
		if filepath.Base(target) != "reames-agent.toml" {
			return errors.New("repair project target is invalid")
		}
	default:
		return errors.New("repair scope is invalid")
	}
	if change.MissingBefore {
		if strings.TrimSpace(change.PreviousPath) != "" {
			return errors.New("repair missing target has unexpected previous path")
		}
		return nil
	}
	previous := filepath.Clean(change.PreviousPath)
	if filepath.Dir(previous) != filepath.Dir(target) || !strings.HasPrefix(filepath.Base(previous), filepath.Base(target)+".reames-") {
		return errors.New("repair previous path is invalid")
	}
	return nil
}

func persistRepairTransaction(tx *RepairTransaction) error {
	path := repairTransactionPath()
	if path == "" || tx == nil || len(tx.Changes) == 0 {
		return nil
	}
	b, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(b, '\n'), 0o600)
}

func appendRepairLogBestEffort(tx *RepairTransaction) {
	path := repairLogPath()
	if path == "" || tx == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	b, err := json.Marshal(tx)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func restoreSnapshotBytes(snap ConfigSnapshot, dest string) error {
	if err := verifyConfigSnapshot(snap); err != nil {
		return err
	}
	b, err := os.ReadFile(snap.Path)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(dest, b, 0o600)
}

func verifyConfigSnapshot(snap ConfigSnapshot) error {
	if err := validateSnapshotMetadata(snapshotDir(), &snap); err != nil {
		return err
	}
	b, err := os.ReadFile(snap.Path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), snap.SHA256) {
		return errors.New("config snapshot hash mismatch")
	}
	if err := config.ValidateFile(snap.Path); err != nil {
		return fmt.Errorf("config snapshot TOML: %w", err)
	}
	return nil
}

func validateSnapshotMetadata(dir string, snap *ConfigSnapshot) error {
	if snap == nil || snap.SchemaVersion != 1 || snap.ID == "" || snap.SHA256 == "" || snap.SourcePath == "" {
		return errors.New("config snapshot metadata is incomplete")
	}
	if filepath.Clean(snap.SourcePath) != filepath.Clean(config.UserConfigPath()) {
		return errors.New("config snapshot source is not the global config")
	}
	want := filepath.Join(filepath.Clean(dir), snap.ID+".toml")
	if filepath.Clean(snap.Path) != want {
		return errors.New("config snapshot path is outside snapshot directory")
	}
	if len(snap.SHA256) != 64 {
		return errors.New("config snapshot hash is invalid")
	}
	return nil
}

func latestValidConfigSnapshot() (ConfigSnapshot, bool) {
	snapshots, _ := ListConfigSnapshots()
	for _, snap := range snapshots {
		if verifyConfigSnapshot(snap) == nil {
			return snap, true
		}
	}
	return ConfigSnapshot{}, false
}

func latestSnapshotPath() string {
	if snap, ok := latestValidConfigSnapshot(); ok {
		return snap.Path
	}
	return ""
}

func pruneConfigSnapshots(keep int) error {
	snapshots, err := ListConfigSnapshots()
	if err != nil {
		return err
	}
	for _, snap := range snapshots[minimum(keep, len(snapshots)):] {
		_ = os.Remove(snap.Path)
		_ = os.Remove(snap.Path + ".json")
	}
	return nil
}

func minimum(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func snapshotDir() string {
	if root := config.MemoryUserDir(); root != "" {
		return filepath.Join(root, "repair", "snapshots")
	}
	return ""
}

func repairTransactionPath() string {
	if root := config.MemoryUserDir(); root != "" {
		return filepath.Join(root, "repair", "last-repair.json")
	}
	return ""
}

func repairLogPath() string {
	if root := config.MemoryUserDir(); root != "" {
		return filepath.Join(root, "repair", "repair-log.jsonl")
	}
	return ""
}
