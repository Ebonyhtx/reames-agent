package agent

import (
	"path/filepath"
	"testing"
	"time"

	"reames-agent/internal/provider"
)

func TestBranchMetaRoundTripAndList(t *testing.T) {
	dir := t.TempDir()
	rootPath := filepath.Join(dir, "root.jsonl")
	childPath := filepath.Join(dir, "child.jsonl")

	root := NewSession("sys")
	root.Add(provider.Message{Role: provider.RoleUser, Content: "root prompt"})
	if err := root.Save(rootPath); err != nil {
		t.Fatal(err)
	}
	if err := TouchBranchMeta(rootPath); err != nil {
		t.Fatal(err)
	}

	child := NewSession("sys")
	child.Add(provider.Message{Role: provider.RoleUser, Content: "child prompt"})
	if err := child.Save(childPath); err != nil {
		t.Fatal(err)
	}
	if err := SaveBranchMeta(childPath, BranchMeta{Name: "experiment", ParentID: BranchID(rootPath), ForkTurn: 2}); err != nil {
		t.Fatal(err)
	}

	branches, err := ListBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 {
		t.Fatalf("branches = %d, want 2", len(branches))
	}
	var rootFound, childFound bool
	for _, b := range branches {
		if b.ID == "root" {
			rootFound = true
		}
		if b.ParentID == "root" && b.Name == "experiment" {
			childFound = true
		}
	}
	if !rootFound {
		t.Fatal("root branch not found")
	}
	if !childFound {
		t.Fatalf("child with parent root and name experiment not found among %+v", branches)
	}
}

func TestListBranchesSkipsCleanupPending(t *testing.T) {
	dir := t.TempDir()
	visiblePath := filepath.Join(dir, "visible.jsonl")
	pendingPath := filepath.Join(dir, "pending.jsonl")

	visible := NewSession("sys")
	visible.Add(provider.Message{Role: provider.RoleUser, Content: "visible prompt"})
	if err := visible.Save(visiblePath); err != nil {
		t.Fatal(err)
	}
	if err := TouchBranchMeta(visiblePath); err != nil {
		t.Fatal(err)
	}

	pending := NewSession("sys")
	pending.Add(provider.Message{Role: provider.RoleUser, Content: "pending prompt"})
	if err := pending.Save(pendingPath); err != nil {
		t.Fatal(err)
	}
	if err := SaveBranchMeta(pendingPath, BranchMeta{Name: "pending experiment"}); err != nil {
		t.Fatal(err)
	}
	if err := MarkCleanupPending(pendingPath, "delete"); err != nil {
		t.Fatal(err)
	}

	branches, err := ListBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 1 {
		t.Fatalf("branches = %d, want 1: %+v", len(branches), branches)
	}
	if branches[0].Path != visiblePath {
		t.Fatalf("listed branch path = %q, want %q", branches[0].Path, visiblePath)
	}
}

func TestSessionInFlightTurnMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in-flight.jsonl")
	sess := NewSession("sys")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "work"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := TouchBranchMeta(path); err != nil {
		t.Fatal(err)
	}
	before, ok, err := LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta ok=%v err=%v", ok, err)
	}
	updatedAt := before.UpdatedAt

	checkpointTurn := 0
	if err := MarkSessionInFlightTurnAtCheckpoint(path, 1, true, &checkpointTurn); err != nil {
		t.Fatal(err)
	}
	marked, ok, err := LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta marked ok=%v err=%v", ok, err)
	}
	if marked.InFlightTurn == nil {
		t.Fatal("in-flight turn marker missing")
	}
	if marked.InFlightTurn.StartMessageIndex != 1 || !marked.InFlightTurn.PreserveUser {
		t.Fatalf("in-flight marker = %+v, want index=1 preserveUser=true", marked.InFlightTurn)
	}
	if marked.InFlightTurn.CheckpointTurn == nil || *marked.InFlightTurn.CheckpointTurn != checkpointTurn {
		t.Fatalf("in-flight checkpoint = %+v, want %d", marked.InFlightTurn.CheckpointTurn, checkpointTurn)
	}
	if marked.InFlightTurn.StartedAt.IsZero() || time.Since(marked.InFlightTurn.StartedAt) > time.Minute {
		t.Fatalf("unexpected marker timestamp: %v", marked.InFlightTurn.StartedAt)
	}
	if !marked.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("MarkSessionInFlightTurn updated activity time: got %v want %v", marked.UpdatedAt, updatedAt)
	}

	if err := UpdateSessionMeta(path, "model-a", "preview", 1, true); err != nil {
		t.Fatal(err)
	}
	refreshed, ok, err := LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta refreshed ok=%v err=%v", ok, err)
	}
	if refreshed.InFlightTurn == nil {
		t.Fatal("UpdateSessionMeta dropped in-flight marker")
	}
	if refreshed.InFlightTurn.StartMessageIndex != 1 || !refreshed.InFlightTurn.PreserveUser {
		t.Fatalf("refreshed in-flight marker = %+v, want index=1 preserveUser=true", refreshed.InFlightTurn)
	}
	if refreshed.InFlightTurn.CheckpointTurn == nil || *refreshed.InFlightTurn.CheckpointTurn != checkpointTurn {
		t.Fatalf("refreshed checkpoint = %+v, want %d", refreshed.InFlightTurn.CheckpointTurn, checkpointTurn)
	}
	expected := *refreshed.InFlightTurn
	if err := CommitSessionInFlightTurn(path, expected, 2, "digest-final"); err != nil {
		t.Fatal(err)
	}
	committed, ok, err := LoadBranchMeta(path)
	if err != nil || !ok || committed.InFlightTurn == nil {
		t.Fatalf("LoadBranchMeta committed ok=%v err=%v meta=%+v", ok, err, committed.InFlightTurn)
	}
	if committed.InFlightTurn.CommitMessageCount != 2 || committed.InFlightTurn.CommitTranscriptDigest != "digest-final" {
		t.Fatalf("commit anchor = %+v, want count=2 digest-final", committed.InFlightTurn)
	}
	updatedAt = refreshed.UpdatedAt

	if err := ClearSessionInFlightTurn(path); err != nil {
		t.Fatal(err)
	}
	cleared, ok, err := LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta cleared ok=%v err=%v", ok, err)
	}
	if cleared.InFlightTurn != nil {
		t.Fatalf("in-flight marker survived clear: %+v", cleared.InFlightTurn)
	}
	if !cleared.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("ClearSessionInFlightTurn updated activity time: got %v want %v", cleared.UpdatedAt, updatedAt)
	}
}

func TestSessionRewindTransactionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rewind.jsonl")
	sess := NewSession("sys")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "work"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := TouchBranchMeta(path); err != nil {
		t.Fatal(err)
	}
	before, _, err := LoadBranchMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	transaction := RewindTransactionMeta{
		Turn: 3, Boundary: 1, TranscriptDigest: "target-digest",
		Runtime: []byte(`{"version":2}`), IncludeCode: true,
		Phase: RewindTransactionPrepared, StartedAt: time.Now().UTC(),
	}
	if err := MarkSessionRewindTransaction(path, transaction); err != nil {
		t.Fatal(err)
	}
	marked, ok, err := LoadBranchMeta(path)
	if err != nil || !ok || marked.Rewind == nil {
		t.Fatalf("LoadBranchMeta marked ok=%v err=%v rewind=%+v", ok, err, marked.Rewind)
	}
	if marked.Rewind.Phase != RewindTransactionPrepared || marked.Rewind.Turn != 3 || !marked.Rewind.IncludeCode {
		t.Fatalf("prepared rewind = %+v", marked.Rewind)
	}
	if marked.Rewind.StartedAt.IsZero() || !marked.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("rewind timestamp/activity = started:%v before:%v after:%v", marked.Rewind.StartedAt, before.UpdatedAt, marked.UpdatedAt)
	}
	if err := UpdateSessionMeta(path, "model-a", "preview", 1, false); err != nil {
		t.Fatal(err)
	}
	refreshed, _, err := LoadBranchMeta(path)
	if err != nil || refreshed.Rewind == nil {
		t.Fatalf("listing refresh dropped rewind marker: err=%v meta=%+v", err, refreshed)
	}
	expected := *refreshed.Rewind
	if err := AdvanceSessionRewindTransaction(path, expected); err != nil {
		t.Fatal(err)
	}
	committed, _, err := LoadBranchMeta(path)
	if err != nil || committed.Rewind == nil || committed.Rewind.Phase != RewindTransactionResourcesApplied {
		t.Fatalf("committed rewind = %+v err=%v", committed.Rewind, err)
	}
	if err := ClearSessionRewindTransaction(path, *committed.Rewind); err != nil {
		t.Fatal(err)
	}
	cleared, _, err := LoadBranchMeta(path)
	if err != nil || cleared.Rewind != nil {
		t.Fatalf("cleared rewind = %+v err=%v", cleared.Rewind, err)
	}
}

func TestSessionModelRoundTripPreservesActivity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	session := NewSession("sys")
	session.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	if err := session.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadSessionModel(path); ok {
		t.Fatal("fresh session should not have a stored model")
	}
	meta, err := EnsureBranchMeta(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := SetBranchModelPreserveUpdated(path, "openrouter/anthropic/claude-sonnet"); err != nil {
		t.Fatal(err)
	}
	model, ok := LoadSessionModel(path)
	if !ok || model != "openrouter/anthropic/claude-sonnet" {
		t.Fatalf("LoadSessionModel = %q, %v", model, ok)
	}
	updated, ok, err := LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("LoadBranchMeta ok=%v err=%v", ok, err)
	}
	if !updated.UpdatedAt.Equal(meta.UpdatedAt) {
		t.Fatalf("model write refreshed activity: before=%s after=%s", meta.UpdatedAt, updated.UpdatedAt)
	}
}
