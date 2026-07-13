package control

import (
	"path/filepath"
	"testing"

	"reames-agent/internal/diff"
)

func TestCheckpointManagerScopedSnapshotRejectsLaterTurn(t *testing.T) {
	root := t.TempDir()
	var manager checkpointManager
	manager.rebind("", root)
	manager.begin("first", 0, "digest-0", nil)
	lateSnapshot := manager.scopedSnapshot()
	if lateSnapshot == nil {
		t.Fatal("scoped snapshot was not captured")
	}
	manager.begin("second", 2, "digest-1", nil)

	lateSnapshot(diff.Change{Path: filepath.Join(root, "late.txt"), Kind: diff.Create})
	manager.snapshot(diff.Change{Path: filepath.Join(root, "current.txt"), Kind: diff.Create})
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
