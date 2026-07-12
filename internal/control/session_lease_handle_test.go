package control

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reames-agent/internal/agent"
)

func TestSessionLeaseHandleOwnsAndReleasesCanonicalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lease, err := TryAcquireSessionLease(path)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Path() != CanonicalSessionPath(path) {
		t.Fatalf("lease path = %q, want %q", lease.Path(), CanonicalSessionPath(path))
	}
	if _, err := agent.TryAcquireSessionLease(path); !errors.Is(err, agent.ErrSessionLeaseHeld) {
		t.Fatalf("second acquire err = %v", err)
	}
	lease.Release()
	reacquired, err := TryAcquireSessionLease(path)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	reacquired.Release()
}

func TestSessionLeaseReclaimCandidatePreservesOwnerBoundary(t *testing.T) {
	if SessionLeaseReclaimCandidate(agent.ErrSessionLeaseHeld) {
		t.Fatal("bare lease sentinel should not be reclaimable without owner detail")
	}
	if !SessionLeaseReclaimCandidate(&agent.SessionLeaseError{Path: "session.jsonl"}) {
		t.Fatal("missing owner metadata should defer to the OS lock")
	}
	current := &agent.SessionLeaseError{Info: &agent.SessionLeaseInfo{PID: os.Getpid(), WriterID: agent.SessionWriterID()}}
	if !SessionLeaseReclaimCandidate(current) {
		t.Fatal("current-process orphan should be a reclaim candidate")
	}
	foreign := &agent.SessionLeaseError{Info: &agent.SessionLeaseInfo{PID: os.Getpid() + 1, WriterID: "foreign"}}
	if SessionLeaseReclaimCandidate(foreign) {
		t.Fatal("foreign readable owner must not be a reclaim candidate")
	}
}
