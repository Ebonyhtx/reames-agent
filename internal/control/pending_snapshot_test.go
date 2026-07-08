package control

import (
	"os"
	"testing"
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
