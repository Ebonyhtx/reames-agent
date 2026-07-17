package repair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/config"
)

func TestExecuteActionRepairsAndIdentityBindsUndo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	path := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	broken := []byte("[broken\n")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ExecuteAction(ActionRequest{Action: ActionRepairConfig, Target: "global"}, ActionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Transaction == "" || result.Report.LastRepair == nil {
		t.Fatalf("repair result = %+v", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("broken config still present: %v", err)
	}

	if _, err := ExecuteAction(ActionRequest{Action: ActionUndoRepair, ExpectedRepairID: "stale"}, ActionOptions{}); err == nil || !strings.Contains(err.Error(), "transaction changed") {
		t.Fatalf("stale undo error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale undo mutated config: %v", err)
	}

	undo, err := ExecuteAction(ActionRequest{Action: ActionUndoRepair, ExpectedRepairID: result.Transaction}, ActionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !undo.Changed || undo.Report.LastRepair == nil || !undo.Report.LastRepair.Undone {
		t.Fatalf("undo result = %+v", undo)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(broken) {
		t.Fatalf("restored config = %q", got)
	}
}

func TestExecuteActionRejectsUnknownAndIncompleteIdentity(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	if _, err := ExecuteAction(ActionRequest{Action: "delete-everything"}, ActionOptions{}); err == nil {
		t.Fatal("unknown recovery action succeeded")
	}
	if _, err := ExecuteAction(ActionRequest{Action: ActionRollbackUpdate}, ActionOptions{}); err == nil || !strings.Contains(err.Error(), "identity is incomplete") {
		t.Fatalf("incomplete rollback identity error = %v", err)
	}
}
