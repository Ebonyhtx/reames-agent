package repair

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

type UpdateApplyFailure struct {
	SchemaVersion int    `json:"schemaVersion"`
	ToVersion     string `json:"toVersion,omitempty"`
	Reason        string `json:"reason,omitempty"`
	RecordedAt    string `json:"recordedAt"`
}

func MarkUpdateApplyFailed(toVersion, reason string) error {
	path := updateApplyFailurePath()
	if path == "" {
		return fmt.Errorf("update apply failure: state directory is unavailable")
	}
	failure := UpdateApplyFailure{SchemaVersion: 1, ToVersion: toVersion, Reason: reason, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	b, err := json.MarshalIndent(failure, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(b, '\n'), 0o600)
}

func ReadUpdateApplyFailure() (*UpdateApplyFailure, bool) {
	b, err := os.ReadFile(updateApplyFailurePath())
	if err != nil {
		return nil, false
	}
	var failure UpdateApplyFailure
	if json.Unmarshal(b, &failure) != nil || failure.SchemaVersion != 1 {
		return nil, false
	}
	return &failure, true
}

func ClearUpdateApplyFailure() error {
	path := updateApplyFailurePath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func RecoverFailedInstall() (UpdateRollbackResult, *UpdateApplyFailure, error) {
	failure, ok := ReadUpdateApplyFailure()
	if !ok {
		return UpdateRollbackResult{}, nil, nil
	}
	tx, err := ReadPendingUpdate()
	if err != nil {
		if os.IsNotExist(err) {
			return UpdateRollbackResult{}, failure, ClearUpdateApplyFailure()
		}
		return UpdateRollbackResult{}, failure, err
	}
	if !applyFailureMatchesUpdate(failure, tx) {
		return UpdateRollbackResult{}, failure, ClearUpdateApplyFailure()
	}
	result, err := rollbackPendingUpdate(tx.ToVersion, tx.CreatedAt)
	if err != nil {
		return result, failure, err
	}
	if err := ClearUpdateApplyFailure(); err != nil {
		return result, failure, err
	}
	return result, failure, nil
}

func applyFailureMatchesUpdate(failure *UpdateApplyFailure, tx *UpdateTransaction) bool {
	if failure == nil || tx == nil {
		return false
	}
	if version := strings.TrimSpace(failure.ToVersion); version != "" && version != strings.TrimSpace(tx.ToVersion) {
		return false
	}
	failureAt, failureErr := time.Parse(time.RFC3339Nano, failure.RecordedAt)
	txAt, txErr := time.Parse(time.RFC3339Nano, tx.CreatedAt)
	return failureErr == nil && txErr == nil && !failureAt.Before(txAt)
}

func updateApplyFailurePath() string {
	if root := config.MemoryUserDir(); root != "" {
		return filepath.Join(root, "repair", "update-apply-failed.json")
	}
	return ""
}
