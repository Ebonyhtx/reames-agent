package control

import (
	"os"
	"path/filepath"
	"testing"

	"reames-agent/internal/diff"
)

func TestCheckpointManagerScopedSnapshotRejectsLaterTurn(t *testing.T) {
	root := t.TempDir()
	var manager checkpointManager
	manager.rebind("", root)
	if err := manager.begin("first", 0, "digest-0", nil); err != nil {
		t.Fatal(err)
	}
	lateSnapshot := manager.scopedSnapshot()
	if lateSnapshot == nil {
		t.Fatal("scoped snapshot was not captured")
	}
	if err := manager.begin("second", 2, "digest-1", nil); err != nil {
		t.Fatal(err)
	}

	if err := lateSnapshot(diff.Change{Path: filepath.Join(root, "late.txt"), Kind: diff.Create}); err == nil {
		t.Fatal("late checkpoint hook accepted a retired turn")
	}
	if err := manager.snapshot(diff.Change{Path: filepath.Join(root, "current.txt"), Kind: diff.Create}); err != nil {
		t.Fatal(err)
	}
	metas := manager.list()
	if len(metas) != 2 {
		t.Fatalf("checkpoint metadata = %+v, want two turns", metas)
	}
	if len(metas[0].Paths) != 0 {
		t.Fatalf("late child effect changed origin checkpoint: %+v", metas[0])
	}
	if len(metas[1].Paths) != 1 || filepath.Base(metas[1].Paths[0]) != "current.txt" {
		t.Fatalf("current checkpoint paths = %+v", metas[1].Paths)
	}
}

func TestCheckpointManagerSyntheticTurnIsHiddenAndRestoresOnlyItsEffects(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	if err := os.WriteFile(path, []byte("v0"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkpointDir := filepath.Join(t.TempDir(), "session.ckpt")
	var manager checkpointManager
	manager.rebind(checkpointDir, root)
	if err := manager.begin("visible", 0, "digest-0", nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "v0"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manager.beginSynthetic("continuation", 2, "digest-1", nil); err != nil {
		t.Fatal(err)
	}
	syntheticTurn, ok := manager.currentTurn()
	if !ok || syntheticTurn != 1 {
		t.Fatalf("synthetic current turn = %d, %v, want 1, true", syntheticTurn, ok)
	}
	if err := manager.snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	var resumed checkpointManager
	resumed.rebind(checkpointDir, root)
	if metas := resumed.list(); len(metas) != 1 || metas[0].Prompt != "visible" {
		t.Fatalf("user-facing checkpoint metadata = %+v, want only visible turn", metas)
	}
	if _, ok := resumed.boundary(syntheticTurn); ok {
		t.Fatalf("synthetic turn %d unexpectedly exposed a conversation boundary", syntheticTurn)
	}
	if _, _, err := resumed.restoreCode(syntheticTurn); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1" {
		t.Fatalf("synthetic rollback restored %q, want v1", got)
	}
}
