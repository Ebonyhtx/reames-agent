package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/diff"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

type recoveryTransactionFixture struct {
	controller    *Controller
	session       *agent.Session
	sessionPath   string
	workspaceRoot string
	targetPath    string
	startMessages int
}

func newRecoveryTransactionFixture(t *testing.T) recoveryTransactionFixture {
	t.Helper()
	workspace := t.TempDir()
	target := filepath.Join(workspace, "state.txt")
	if err := os.WriteFile(target, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessionDir := t.TempDir()
	path := filepath.Join(sessionDir, "transaction.jsonl")
	sess := agent.NewSession("system")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "previous"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "done"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}
	exec := agent.New(nil, nil, sess, agent.Options{}, event.Discard)
	c := New(Options{Executor: exec, SessionDir: sessionDir, SessionPath: path, WorkspaceRoot: workspace})
	start := sess.Len()
	if err := c.beginCheckpoint("edit state", false); err != nil {
		t.Fatal(err)
	}
	if err := c.markInFlightTurn(start, true); err != nil {
		t.Fatal(err)
	}
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "edit state"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		ID: "write-1", Name: "write_file", Arguments: `{"path":"state.txt","content":"after"}`,
	}}})
	if err := c.persistWriterRecoveryState(diff.Change{Path: target, Kind: diff.Modify, OldText: "before", NewText: "after"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	return recoveryTransactionFixture{
		controller: c, session: sess, sessionPath: path,
		workspaceRoot: workspace, targetPath: target, startMessages: start,
	}
}

func resumeRecoveryTransaction(t *testing.T, f recoveryTransactionFixture) *Controller {
	t.Helper()
	loaded, err := agent.LoadSession(f.sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	exec := agent.New(nil, nil, agent.NewSession("system"), agent.Options{}, event.Discard)
	c := New(Options{
		Executor: exec, SessionDir: filepath.Dir(f.sessionPath), SessionPath: f.sessionPath,
		WorkspaceRoot: f.workspaceRoot,
	})
	c.Resume(loaded, f.sessionPath)
	return c
}

func TestInterruptedWriterTransactionRollsBackWorkspaceTranscriptAndRuntime(t *testing.T) {
	f := newRecoveryTransactionFixture(t)
	f.session.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "write-1", Name: "write_file", Content: "partial result"})
	if err := f.controller.SnapshotActivity(); err != nil {
		t.Fatal(err)
	}

	recovered := resumeRecoveryTransaction(t, f)
	data, err := os.ReadFile(f.targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("workspace after interrupted recovery = %q, want before", data)
	}
	msgs := recovered.History()
	if len(msgs) != f.startMessages+1 {
		t.Fatalf("recovered transcript length = %d, want %d: %+v", len(msgs), f.startMessages+1, msgs)
	}
	last := msgs[len(msgs)-1]
	if last.Role != provider.RoleUser || last.Content != "edit state" {
		t.Fatalf("recovered last message = %+v, want preserved visible user prompt", last)
	}
	state, ok := recovered.goals.readSessionState(f.sessionPath)
	if !ok {
		t.Fatal("recovered runtime sidecar missing")
	}
	equal, _ := recovered.executor.Session().CompareTranscriptAnchor(state.MessageCount, state.TranscriptDigest)
	if !equal {
		t.Fatalf("runtime anchor count=%d digest=%q does not match cleaned transcript", state.MessageCount, state.TranscriptDigest)
	}
	meta, ok, err := agent.LoadBranchMeta(f.sessionPath)
	if err != nil || !ok || meta.InFlightTurn != nil {
		t.Fatalf("recovery marker after rollback ok=%v err=%v marker=%+v", ok, err, meta.InFlightTurn)
	}
}

func TestCommittedWriterTransactionSurvivesCrashBeforeMarkerClear(t *testing.T) {
	f := newRecoveryTransactionFixture(t)
	f.session.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "write-1", Name: "write_file", Content: "wrote state.txt"})
	f.session.Add(provider.Message{Role: provider.RoleAssistant, Content: "finished"})
	f.controller.inFlightClear = func(string) error { return errors.New("injected crash after commit") }
	if err := f.controller.commitInFlightTurn(f.startMessages); err == nil {
		t.Fatal("commit should surface the injected marker-clear failure")
	}
	meta, ok, err := agent.LoadBranchMeta(f.sessionPath)
	if err != nil || !ok || meta.InFlightTurn == nil || meta.InFlightTurn.CommitTranscriptDigest == "" {
		t.Fatalf("committed crash marker ok=%v err=%v marker=%+v", ok, err, meta.InFlightTurn)
	}

	recovered := resumeRecoveryTransaction(t, f)
	data, err := os.ReadFile(f.targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after" {
		t.Fatalf("committed workspace after recovery = %q, want after", data)
	}
	msgs := recovered.History()
	if len(msgs) != f.startMessages+4 || msgs[len(msgs)-1].Content != "finished" {
		t.Fatalf("committed transcript changed during recovery: %+v", msgs)
	}
	meta, ok, err = agent.LoadBranchMeta(f.sessionPath)
	if err != nil || !ok || meta.InFlightTurn != nil {
		t.Fatalf("committed marker after recovery ok=%v err=%v marker=%+v", ok, err, meta.InFlightTurn)
	}
}

func TestPendingTurnRecoveryFailureBlocksNextTurn(t *testing.T) {
	f := newRecoveryTransactionFixture(t)
	f.controller.goals.stateWrite = func(string, []byte, os.FileMode) error {
		return errors.New("injected recovered runtime failure")
	}
	err := f.controller.runTurnWithRaw(context.Background(), "must not start", "must not start")
	if err == nil || !strings.Contains(err.Error(), "recover session before new turn: interrupted turn") {
		t.Fatalf("next turn error = %v, want pending recovery failure", err)
	}
	meta, ok, loadErr := agent.LoadBranchMeta(f.sessionPath)
	if loadErr != nil || !ok || meta.InFlightTurn == nil {
		t.Fatalf("pending marker after blocked next turn ok=%v err=%v marker=%+v", ok, loadErr, meta.InFlightTurn)
	}
}

func newRecoveryGateController(t *testing.T) (*Controller, *agent.Session) {
	t.Helper()
	workspace := t.TempDir()
	sessionDir := t.TempDir()
	path := filepath.Join(sessionDir, "gate.jsonl")
	sess := agent.NewSession("system")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "previous"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}
	exec := agent.New(nil, nil, sess, agent.Options{}, event.Discard)
	c := New(Options{
		Runner: appendingRunner{session: sess}, Executor: exec,
		SessionDir: sessionDir, SessionPath: path, WorkspaceRoot: workspace,
	})
	return c, sess
}

func TestCheckpointPersistenceFailureBlocksTurnBeforeRunner(t *testing.T) {
	c, sess := newRecoveryGateController(t)
	start := sess.Len()
	c.checkpoints.mu.Lock()
	c.checkpoints.turn = -1
	c.checkpoints.mu.Unlock()

	err := c.runTurnWithRaw(context.Background(), "must not run", "must not run")
	if err == nil || !strings.Contains(err.Error(), "begin turn recovery checkpoint") {
		t.Fatalf("turn error = %v, want checkpoint persistence failure", err)
	}
	if sess.Len() != start {
		t.Fatalf("runner changed transcript length from %d to %d", start, sess.Len())
	}
	if checkpoints := c.Checkpoints(); len(checkpoints) != 0 {
		t.Fatalf("failed checkpoint became user-visible: %+v", checkpoints)
	}
}

func TestInFlightMarkerFailureBlocksTurnAndRetiresCheckpoint(t *testing.T) {
	c, sess := newRecoveryGateController(t)
	start := sess.Len()
	c.inFlightMark = func(string, int, bool) error {
		return errors.New("injected marker persistence failure")
	}

	err := c.runTurnWithRaw(context.Background(), "must not run", "must not run")
	if err == nil || !strings.Contains(err.Error(), "mark in-flight turn") {
		t.Fatalf("turn error = %v, want marker persistence failure", err)
	}
	if sess.Len() != start {
		t.Fatalf("runner changed transcript length from %d to %d", start, sess.Len())
	}
	if checkpoints := c.Checkpoints(); len(checkpoints) != 0 {
		t.Fatalf("unarmed checkpoint remained user-visible: %+v", checkpoints)
	}
	if _, ok := c.checkpoints.currentTurn(); ok {
		t.Fatal("unarmed checkpoint remained active")
	}
}
