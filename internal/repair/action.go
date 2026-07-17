package repair

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reames-agent/internal/config"
)

const (
	ActionRepairConfig   = "repair-config"
	ActionRollbackUpdate = "rollback-update"
	ActionRestoreConfig  = "restore-config"
	ActionUndoRepair     = "undo-repair"
	ActionRebuildState   = "rebuild-state"
	ActionDisablePlugins = "disable-plugins"
)

// ActionRequest is the transport-independent recovery mutation contract shared
// by Desktop and control.Controller. Identity-bearing operations must include
// the exact object observed in the preceding Report so stale UI state cannot
// mutate a newer transaction.
type ActionRequest struct {
	Action                  string `json:"action"`
	Target                  string `json:"target,omitempty"`
	SnapshotID              string `json:"snapshotId,omitempty"`
	ExpectedRepairID        string `json:"expectedRepairId,omitempty"`
	ExpectedUpdateVersion   string `json:"expectedUpdateVersion,omitempty"`
	ExpectedUpdateCreatedAt string `json:"expectedUpdateCreatedAt,omitempty"`
}

type ActionResult struct {
	Action      string   `json:"action"`
	Changed     bool     `json:"changed"`
	Affected    []string `json:"affected"`
	Transaction string   `json:"transaction,omitempty"`
	Report      Report   `json:"report"`
}

type ActionOptions struct {
	Root           string
	ExecutablePath string
}

// ExecuteAction runs one bounded recovery operation and immediately returns a
// fresh Report. It never loads config, Provider, MCP, plugin runtimes, Hooks, or
// an Agent. A process-wide recovery lock prevents competing Desktop actions;
// individual stores retain their own cross-process locks and identity checks.
func ExecuteAction(req ActionRequest, opts ActionOptions) (ActionResult, error) {
	action := strings.TrimSpace(req.Action)
	result := ActionResult{Action: action, Affected: []string{}}
	unlock, err := lockRecoveryAction()
	if err != nil {
		return result, err
	}
	defer unlock()

	switch action {
	case ActionRepairConfig:
		scope := strings.ToLower(strings.TrimSpace(req.Target))
		if scope != "global" && scope != "project" {
			return result, fmt.Errorf("repair config: target must be global or project")
		}
		if scope == "project" && strings.TrimSpace(opts.Root) == "" {
			return result, fmt.Errorf("repair config: no project workspace is selected")
		}
		report, repairErr := InspectAndRepairConfig(ConfigOptions{
			Root:           opts.Root,
			Apply:          true,
			IncludeProject: scope == "project",
			OnlyScope:      scope,
		})
		if repairErr != nil {
			return result, repairErr
		}
		result.Affected = append(result.Affected, report.Applied...)
		result.Changed = len(report.Applied) > 0
		if report.Transaction != nil {
			result.Transaction = report.Transaction.ID
		}

	case ActionRollbackUpdate:
		version := strings.TrimSpace(req.ExpectedUpdateVersion)
		createdAt := strings.TrimSpace(req.ExpectedUpdateCreatedAt)
		if version == "" || createdAt == "" {
			return result, fmt.Errorf("rollback update: expected transaction identity is incomplete")
		}
		rollback, rollbackErr := RollbackPendingUpdateIfCurrent(version, createdAt)
		if rollbackErr != nil {
			return result, rollbackErr
		}
		if !rollback.RolledBack {
			return result, fmt.Errorf("rollback update: pending transaction changed; refresh recovery status")
		}
		result.Changed = true
		result.Affected = append(result.Affected, rollback.TargetPath)

	case ActionRestoreConfig:
		id := strings.TrimSpace(req.SnapshotID)
		if id == "" {
			return result, fmt.Errorf("restore config: snapshot id is required")
		}
		tx, restoreErr := RestoreConfigSnapshot(id)
		if restoreErr != nil {
			return result, restoreErr
		}
		result.Changed = true
		result.Transaction = tx.ID
		for _, change := range tx.Changes {
			result.Affected = append(result.Affected, change.TargetPath)
		}

	case ActionUndoRepair:
		expected := strings.TrimSpace(req.ExpectedRepairID)
		if expected == "" {
			return result, fmt.Errorf("undo repair: expected transaction identity is required")
		}
		current, readErr := ReadLastRepair()
		if readErr != nil {
			return result, readErr
		}
		if current.ID != expected {
			return result, fmt.Errorf("undo repair: recovery transaction changed; refresh recovery status")
		}
		tx, undoErr := UndoLastRepair()
		if undoErr != nil {
			return result, undoErr
		}
		result.Changed = true
		result.Transaction = tx.ID
		for _, change := range tx.Changes {
			result.Affected = append(result.Affected, change.TargetPath)
		}

	case ActionRebuildState:
		paths, rebuildErr := RebuildDerivedState(req.Target)
		if rebuildErr != nil {
			return result, rebuildErr
		}
		result.Affected = append(result.Affected, paths...)
		result.Changed = len(paths) > 0

	case ActionDisablePlugins:
		disabled, disableErr := DisableAllPlugins()
		if disableErr != nil {
			return result, disableErr
		}
		result.Affected = append(result.Affected, disabled...)
		result.Changed = len(disabled) > 0

	default:
		return result, fmt.Errorf("unknown recovery action %q", action)
	}

	result.Report, err = Inspect(InspectOptions{Root: opts.Root, ExecutablePath: opts.ExecutablePath})
	if err != nil {
		return result, err
	}
	return result, nil
}

func lockRecoveryAction() (func(), error) {
	root := configStateRoot()
	if root == "" {
		return nil, fmt.Errorf("recovery action: state directory is unavailable")
	}
	path := filepath.Join(root, "repair", "recovery-action.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return lockRepairStateFile(path)
}

func configStateRoot() string {
	return strings.TrimSpace(config.MemoryUserDir())
}
