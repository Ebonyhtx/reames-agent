package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/event"
	"reames-agent/internal/permission"
)

func TestLoadPendingSnapshots_Empty(t *testing.T) {
	os.Remove(pendingSnapshotPath())
	snaps, err := LoadPendingSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 0 {
		t.Fatalf("expected 0 snaps, got %d", len(snaps))
	}
}

func TestLoadPendingSnapshots_Corrupt(t *testing.T) {
	os.WriteFile(pendingSnapshotPath(), []byte("not json"), 0600)
	defer os.Remove(pendingSnapshotPath())
	_, err := LoadPendingSnapshots()
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
}

func TestPendingSnapshotPath(t *testing.T) {
	path := pendingSnapshotPath()
	if path == "" {
		t.Fatal("empty path")
	}
}

func TestPendingSnapshotPersistsApprovalFileDiff(t *testing.T) {
	isolateControlConfigHome(t)
	path := pendingSnapshotPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir pending snapshot dir: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	am := newApprovalManager(permission.New("ask", nil, nil, nil), ToolApprovalAsk, 0)
	am.sessionID = filepath.Join(t.TempDir(), "session.jsonl")
	diff := event.FileDiff{Diff: "--- a/note\n+++ b/note\n+hello\n", Added: 1}
	id, _ := am.register("write_file", "note.md", "", diff)

	snaps, err := LoadPendingSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %+v, want exactly one pending approval", snaps)
	}
	got := snaps[0]
	if got.ID != id || got.Kind != "approval" || got.Tool != "write_file" || got.Subject != "note.md" {
		t.Fatalf("snapshot identity = %+v, want id/tool/subject from pending approval", got)
	}
	if got.FileDiff != diff {
		t.Fatalf("snapshot diff = %+v, want %+v", got.FileDiff, diff)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pending snapshot: %v", err)
	}
	if !strings.Contains(string(raw), `"diff"`) || strings.Contains(string(raw), `"Diff"`) {
		t.Fatalf("pending snapshot JSON should use lower-case file_diff fields, got %s", raw)
	}

	am.resolve(id)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pending snapshot after resolve stat err = %v, want removed", err)
	}
}

func TestPendingSnapshotPersistsStructuredApprovalPlan(t *testing.T) {
	isolateControlConfigHome(t)
	path := pendingSnapshotPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir pending snapshot dir: %v", err)
	}
	t.Cleanup(func() { os.Remove(path) })

	am := newApprovalManager(permission.New("ask", nil, nil, nil), ToolApprovalAsk, 0)
	plan := &event.ApprovalPlan{
		PlanID: "plan-persisted", Operation: "install", Scope: "global",
		Actions: []event.ApprovalAction{{Kind: "plugin", Action: "install_plugin_package", RiskLevel: "high", Target: "plugins/reviewed", Permissions: []string{"hooks:execute"}}},
	}
	id, _ := am.registerStructuredDecision("install_source", "install reviewed", "review exact plan", plan)

	snaps, err := LoadPendingSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || snaps[0].ID != id || snaps[0].Plan == nil {
		t.Fatalf("structured snapshots = %+v", snaps)
	}
	got := snaps[0].Plan
	if got.PlanID != plan.PlanID || len(got.Actions) != 1 || got.Actions[0].RiskLevel != "high" || got.Actions[0].Target != "plugins/reviewed" || len(got.Actions[0].Permissions) != 1 {
		t.Fatalf("persisted plan = %+v, want %+v", got, plan)
	}

	am.resolve(id)
}
