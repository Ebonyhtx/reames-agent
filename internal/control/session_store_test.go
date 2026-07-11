package control

import (
	"path/filepath"
	"testing"
)

func TestSessionLeaseProbeAndRemovalGuardPreserveOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned.jsonl")
	keeper := NewSessionLeaseKeeper()
	if err := keeper.Rebind(path); err != nil {
		t.Fatal(err)
	}
	if !SessionLeaseAvailableForRebuild(path) {
		t.Fatal("same-process lease should allow the real rebuild bind to decide")
	}
	if guard, err := TryAcquireSessionRemovalGuard(path); guard != nil || !IsSessionLeaseHeld(err) {
		t.Fatalf("removal guard under live lease = guard %v err %v", guard, err)
	}
	keeper.Release()
	guard, err := TryAcquireSessionRemovalGuard(path)
	if err != nil {
		t.Fatal(err)
	}
	guard.Release()
}

func TestSessionNameIsTransportStable(t *testing.T) {
	if got := SessionName(filepath.Join("sessions", "turn-123.jsonl")); got != "turn-123" {
		t.Fatalf("SessionName = %q", got)
	}
}
