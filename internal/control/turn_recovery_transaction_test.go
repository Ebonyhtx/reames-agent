package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

type interruptedTranscriptRunner struct {
	session *agent.Session
}

func (r interruptedTranscriptRunner) Run(_ context.Context, input string) error {
	r.session.Add(provider.Message{Role: provider.RoleUser, Content: input})
	r.session.Add(provider.Message{Role: provider.RoleAssistant, Content: "durable partial response"})
	return &provider.StreamInterruptedError{Err: errors.New("connection reset by peer")}
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

func TestGracefulInterruptedTurnKeepsCompletePairsAndLocalizesUnsafeTail(t *testing.T) {
	sess := agent.NewSession("system")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "previous"})
	start := sess.Len()
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "update config", CreatedAt: time.Now().UnixMilli()})
	sess.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		ID: "write-1", Name: "write_file", Arguments: `{"path":"config.json","content":"{}"}`, Added: 1,
	}}})
	sess.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "write-1", Name: "write_file", Content: "wrote config.json"})
	sess.Add(provider.Message{
		Role: provider.RoleAssistant, Content: "unsafe partial answer", ReasoningContent: "unsafe partial reasoning",
		ReasoningSignature: "must-not-replay", ReasoningBlocks: []provider.ReasoningBlock{{Type: "redacted_thinking", Data: "opaque"}},
		ToolCalls: []provider.ToolCall{{ID: "partial-1", Name: "bash", Arguments: `{"command":"danger"}`}},
	})
	exec := agent.New(nil, nil, sess, agent.Options{}, event.Discard)
	c := New(Options{Executor: exec})
	if err := c.preserveInterruptedVisibleTurnMessagesAfter(start, provider.Message{}); err != nil {
		t.Fatalf("preserve interrupted turn: %v", err)
	}

	msgs := sess.Snapshot()
	if len(msgs) != start+4 || msgs[start+1].Role != provider.RoleAssistant || msgs[start+2].Role != provider.RoleTool {
		t.Fatalf("complete canonical tool pair was not retained: %+v", msgs)
	}
	local := msgs[len(msgs)-1]
	if !local.LocalOnly || local.InterruptedTurn == nil || !local.InterruptedTurn.Pending || local.ReasoningSignature != "" || len(local.ReasoningBlocks) != 0 {
		t.Fatalf("unsafe tail was not converted to a safe local recovery record: %+v", local)
	}
	if len(local.ToolCalls) != 1 || local.ToolCalls[0].Name != "bash" || local.ToolCalls[0].Arguments != "" {
		t.Fatalf("partial tool arguments leaked into local display contract: %+v", local.ToolCalls)
	}
	recovery := local.InterruptedTurn
	if len(recovery.CompletedTools) != 1 || recovery.CompletedTools[0].Name != "write_file" || len(recovery.CompletedTools[0].Files) != 1 || recovery.CompletedTools[0].Files[0] != "config.json" || len(recovery.InterruptedTools) != 1 || recovery.InterruptedTools[0] != "bash" {
		t.Fatalf("recovery facts = %+v", recovery)
	}
	for _, m := range provider.MessagesForRequest(msgs) {
		if strings.Contains(m.Content, "unsafe partial") || strings.Contains(m.ReasoningContent, "unsafe partial") {
			t.Fatalf("unsafe local output leaked to provider messages: %+v", provider.MessagesForRequest(msgs))
		}
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

func TestStreamInterruptedTurnCommitsPartialTranscriptAndRuntime(t *testing.T) {
	workspace := t.TempDir()
	sessionDir := t.TempDir()
	path := filepath.Join(sessionDir, "stream-interrupted.jsonl")
	sess := agent.NewSession("system")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "previous"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "done"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}
	startMessages := sess.Len()
	exec := agent.New(nil, nil, sess, agent.Options{}, event.Discard)
	c := New(Options{
		Runner: interruptedTranscriptRunner{session: sess}, Executor: exec,
		SessionDir: sessionDir, SessionPath: path, WorkspaceRoot: workspace,
	})

	err := c.runTurnWithRaw(context.Background(), "continue", "continue")
	if !provider.IsStreamInterrupted(err) {
		t.Fatalf("turn error = %v, want StreamInterruptedError", err)
	}
	loaded, loadErr := agent.LoadSession(path)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	msgs := loaded.Messages
	if len(msgs) != startMessages+2 || !msgs[len(msgs)-1].LocalOnly || msgs[len(msgs)-1].Content != "durable partial response" || msgs[len(msgs)-1].InterruptedTurn == nil || !msgs[len(msgs)-1].InterruptedTurn.Pending {
		t.Fatalf("persisted interrupted transcript = %+v, want provider-excluded partial display tail", msgs)
	}
	wire := provider.MessagesForRequest(msgs)
	for _, message := range wire {
		if strings.Contains(message.Content, "durable partial response") {
			t.Fatalf("partial interrupted output leaked into provider history: %+v", wire)
		}
	}
	state, ok := c.goals.readSessionState(path)
	if !ok {
		t.Fatal("interrupted stream runtime sidecar missing")
	}
	equal, _ := loaded.CompareTranscriptAnchor(state.MessageCount, state.TranscriptDigest)
	if !equal {
		t.Fatalf("runtime anchor count=%d digest=%q does not match partial transcript", state.MessageCount, state.TranscriptDigest)
	}
	meta, ok, metaErr := agent.LoadBranchMeta(path)
	if metaErr != nil || !ok || meta.InFlightTurn != nil {
		t.Fatalf("stream interruption marker after commit ok=%v err=%v marker=%+v", ok, metaErr, meta.InFlightTurn)
	}
}

func TestStreamInterruptedTurnCommitFailureRollsBackPartialTranscript(t *testing.T) {
	workspace := t.TempDir()
	sessionDir := t.TempDir()
	path := filepath.Join(sessionDir, "stream-interrupted-commit-failure.jsonl")
	sess := agent.NewSession("system")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "previous"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "done"})
	if err := sess.Save(path); err != nil {
		t.Fatal(err)
	}
	startMessages := sess.Len()
	exec := agent.New(nil, nil, sess, agent.Options{}, event.Discard)
	c := New(Options{
		Runner: interruptedTranscriptRunner{session: sess}, Executor: exec,
		SessionDir: sessionDir, SessionPath: path, WorkspaceRoot: workspace,
	})
	c.inFlightCommit = func(string, agent.InFlightTurnMeta, int, string) error {
		return errors.New("injected interrupted stream commit failure")
	}

	err := c.runTurnWithRaw(context.Background(), "continue", "continue")
	if !provider.IsStreamInterrupted(err) || !strings.Contains(err.Error(), "commit interrupted turn") {
		t.Fatalf("turn error = %v, want interrupted stream commit failure", err)
	}
	loaded, loadErr := agent.LoadSession(path)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	msgs := loaded.Messages
	if len(msgs) != startMessages+1 || msgs[len(msgs)-1].Role != provider.RoleUser || msgs[len(msgs)-1].Content != "continue" {
		t.Fatalf("rolled-back interrupted transcript = %+v, want only preserved visible user prompt", msgs)
	}
	state, ok := c.goals.readSessionState(path)
	if !ok {
		t.Fatal("rolled-back interrupted stream runtime sidecar missing")
	}
	equal, _ := loaded.CompareTranscriptAnchor(state.MessageCount, state.TranscriptDigest)
	if !equal {
		t.Fatalf("runtime anchor count=%d digest=%q does not match rolled-back transcript", state.MessageCount, state.TranscriptDigest)
	}
	meta, ok, metaErr := agent.LoadBranchMeta(path)
	if metaErr != nil || !ok || meta.InFlightTurn != nil {
		t.Fatalf("stream interruption marker after rollback ok=%v err=%v marker=%+v", ok, metaErr, meta.InFlightTurn)
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
