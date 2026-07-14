package control

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/permission"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
	"reames-agent/internal/tool/builtin"
)

type recordingWriter struct {
	mu    sync.Mutex
	paths []string
}

func (w *recordingWriter) Name() string        { return "write_file" }
func (w *recordingWriter) Description() string { return "write a file" }
func (w *recordingWriter) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
}
func (w *recordingWriter) ReadOnly() bool { return false }
func (w *recordingWriter) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &a)
	w.mu.Lock()
	w.paths = append(w.paths, a.Path)
	w.mu.Unlock()
	return "ok", nil
}

func toolCallTurn(id, name, args string) []provider.Chunk {
	return []provider.Chunk{
		{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: id, Name: name, Arguments: args}},
		{Type: provider.ChunkDone},
	}
}

// TestApprovalToolWideEndToEnd drives a full agent turn through the real gate:
// the model writes two different files, the user answers "allow for this session"
// on the first, and the second must run without a second prompt. Regression for
// #3498 / #3520 (a session/persist grant used to pin the exact subject, so every
// new file/command re-prompted).
func TestApprovalToolWideEndToEnd(t *testing.T) {
	writer := &recordingWriter{}
	reg := tool.NewRegistry()
	reg.Add(writer)

	prov := &scriptedTurns{turns: [][]provider.Chunk{
		toolCallTurn("c1", "write_file", `{"path":"a.txt"}`),
		toolCallTurn("c2", "write_file", `{"path":"b.txt"}`),
		textTurn("Done."),
	}}
	ag := agent.New(prov, reg, agent.NewSession(""), agent.Options{}, event.Discard)

	approvalID := make(chan string, 4)
	prompts := 0
	c := New(Options{
		Runner:   ag,
		Executor: ag,
		Policy:   permission.New("ask", nil, nil, nil), // writers ask by default
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.ApprovalRequest {
				prompts++
				approvalID <- e.Approval.ID
			}
		}),
	})
	c.EnableInteractiveApproval()

	// Answer the first prompt with "allow for this session" (allow, session, !persist).
	go func() { c.Approve(<-approvalID, true, true, false) }()

	if err := c.runTurnWithRaw(context.Background(), "edit the files", "edit the files"); err != nil {
		t.Fatalf("runTurnWithRaw: %v", err)
	}

	if prompts != 1 {
		t.Errorf("approval prompts = %d, want 1 (the session grant must cover the second file too)", prompts)
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.paths) != 2 || writer.paths[0] != "a.txt" || writer.paths[1] != "b.txt" {
		t.Errorf("executed writes = %v, want both a.txt and b.txt", writer.paths)
	}
}

// TestApprovalRealWriteFilePreviewDiskAndRewindEndToEnd locks the M1 file-write
// contract to a real builtin writer: a model-requested write_file call must
// surface an approval, carry the same patch preview on ToolDispatch and
// ApprovalRequest, write to disk only after approval, and be removable by code
// rewind through the checkpoint pre-edit hook.
func TestApprovalRealWriteFilePreviewDiskAndRewindEndToEnd(t *testing.T) {
	workspace := t.TempDir()
	sessionDir := t.TempDir()
	rel := filepath.Join("notes", "hello.txt")
	content := "hello\nfrom reames\n"
	args, err := json.Marshal(struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{Path: rel, Content: content})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: workspace}).Tools("write_file") {
		reg.Add(tl)
	}
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		toolCallTurn("w1", "write_file", string(args)),
		textTurn("Done."),
	}}

	var events []event.Event
	approvalID := make(chan string, 1)
	sink := event.FuncSink(func(e event.Event) {
		events = append(events, e)
		if e.Kind == event.ApprovalRequest {
			approvalID <- e.Approval.ID
		}
	})
	ag := agent.New(prov, reg, agent.NewSession("sys"), agent.Options{}, sink)
	c := New(Options{
		Runner:        ag,
		Executor:      ag,
		Policy:        permission.New("ask", nil, nil, nil),
		SessionDir:    sessionDir,
		WorkspaceRoot: workspace,
		Sink:          sink,
	})
	c.SetSessionPath(agent.NewSessionPath(sessionDir, "m1-write"))
	c.EnableInteractiveApproval()

	go func() { c.Approve(<-approvalID, true, false, false) }()
	if err := c.runTurnWithRaw(context.Background(), "write the note", "write the note"); err != nil {
		t.Fatalf("runTurnWithRaw: %v", err)
	}

	var dispatchDiff, approvalDiff event.FileDiff
	for _, e := range events {
		switch e.Kind {
		case event.ToolDispatch:
			if e.Tool.ID == "w1" {
				dispatchDiff = e.Tool.FileDiff
			}
		case event.ApprovalRequest:
			if e.Approval.Tool == "write_file" {
				approvalDiff = e.Approval.FileDiff
			}
		}
	}
	if dispatchDiff.Diff == "" || dispatchDiff.Added == 0 {
		t.Fatalf("ToolDispatch FileDiff = %+v, want non-empty patch preview", dispatchDiff)
	}
	if approvalDiff.Diff == "" || approvalDiff.Added == 0 {
		t.Fatalf("ApprovalRequest FileDiff = %+v, want non-empty patch preview", approvalDiff)
	}
	if dispatchDiff.Diff != approvalDiff.Diff || dispatchDiff.Added != approvalDiff.Added || dispatchDiff.Removed != approvalDiff.Removed {
		t.Fatalf("approval diff = %+v, want same preview as dispatch %+v", approvalDiff, dispatchDiff)
	}
	if !strings.Contains(approvalDiff.Diff, "+hello") || !strings.Contains(approvalDiff.Diff, "+from reames") {
		t.Fatalf("approval diff %q does not show added content", approvalDiff.Diff)
	}

	target := filepath.Join(workspace, rel)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("written content = %q, want %q", got, content)
	}
	cps := c.Checkpoints()
	if len(cps) != 1 || len(cps[0].Paths) != 1 {
		t.Fatalf("checkpoints = %+v, want one checkpoint with one file snapshot", cps)
	}
	if err := c.Rewind(0, RewindCode); err != nil {
		t.Fatalf("Rewind code: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("written file after rewind stat err = %v, want not exist", err)
	}
}

func TestPreviewableWriterFailsClosedOnRecoveryPersistenceFailures(t *testing.T) {
	tests := []struct {
		name        string
		inject      func(*Controller, string) error
		wantTurnErr string
	}{
		{
			name: "checkpoint",
			inject: func(_ *Controller, sessionPath string) error {
				return os.WriteFile(ckptDir(sessionPath), []byte("not a directory"), 0o644)
			},
			wantTurnErr: "begin turn recovery checkpoint",
		},
		{
			name: "runtime sidecar",
			inject: func(c *Controller, _ string) error {
				c.goals.stateWrite = func(string, []byte, os.FileMode) error {
					return errors.New("injected runtime persistence failure")
				}
				return nil
			},
			wantTurnErr: "runtime",
		},
		{
			name: "in-flight marker",
			inject: func(c *Controller, sessionPath string) error {
				// A stale marker must not satisfy the current turn's writer gate
				// when persisting the replacement marker fails.
				if err := agent.MarkSessionInFlightTurn(sessionPath, 999, false); err != nil {
					return err
				}
				c.inFlightMark = func(string, int, bool) error {
					return errors.New("injected in-flight marker failure")
				}
				return nil
			},
			wantTurnErr: "mark in-flight turn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			sessionDir := t.TempDir()
			target := filepath.Join(workspace, "blocked.txt")
			args, err := json.Marshal(map[string]string{"path": "blocked.txt", "content": "must not land"})
			if err != nil {
				t.Fatal(err)
			}
			reg := tool.NewRegistry()
			for _, tl := range (builtin.Workspace{Dir: workspace}).Tools("write_file") {
				reg.Add(tl)
			}
			prov := &scriptedTurns{turns: [][]provider.Chunk{
				toolCallTurn("w-blocked", "write_file", string(args)),
				textTurn("The write was blocked."),
			}}
			ag := agent.New(prov, reg, agent.NewSession("sys"), agent.Options{}, event.Discard)
			c := New(Options{Runner: ag, Executor: ag, SessionDir: sessionDir, WorkspaceRoot: workspace})
			sessionPath := agent.NewSessionPath(sessionDir, "persistence-gate")
			c.SetSessionPath(sessionPath)
			if err := tt.inject(c, sessionPath); err != nil {
				t.Fatal(err)
			}

			turnErr := c.runTurnWithRaw(context.Background(), "write the file", "write the file")
			if turnErr == nil || !strings.Contains(turnErr.Error(), tt.wantTurnErr) {
				t.Fatalf("runTurnWithRaw error = %v, want %q", turnErr, tt.wantTurnErr)
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatalf("writer touched disk despite %s failure: %v", tt.name, err)
			}
		})
	}
}

// TestApprovalTimeoutDeniesWhenUnanswered verifies a positive ApprovalTimeout
// turns an unanswered prompt into a denial (error) instead of blocking forever
// (#4626, #4402). Ask shares the same wait context as tool-approval prompts.
func TestApprovalTimeoutDeniesWhenUnanswered(t *testing.T) {
	c := New(Options{
		Policy:          permission.New("ask", nil, nil, nil),
		Sink:            event.Discard,
		ApprovalTimeout: 40 * time.Millisecond,
	})
	c.EnableInteractiveApproval()

	start := time.Now()
	_, err := c.Ask(context.Background(), []event.AskQuestion{{ID: "q1", Prompt: "pick one"}})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Ask should error when the approval timeout elapses unanswered")
	}
	// Must return near the timeout, not hang. Allow generous slack for CI scheduling.
	if elapsed > 2*time.Second {
		t.Fatalf("Ask blocked for %v; timeout should have fired near 40ms", elapsed)
	}
}

// TestApprovalTimeoutZeroWaitsIndefinitely confirms the default (zero) keeps the
// interactive behavior: an unanswered Ask blocks rather than timing out, so a
// human at a terminal is never cut off.
func TestApprovalTimeoutZeroWaitsIndefinitely(t *testing.T) {
	c := New(Options{
		Policy: permission.New("ask", nil, nil, nil),
		Sink:   event.Discard,
		// ApprovalTimeout intentionally zero (default).
	})
	c.EnableInteractiveApproval()

	done := make(chan error, 1)
	go func() {
		_, err := c.Ask(context.Background(), []event.AskQuestion{{ID: "q1", Prompt: "pick one"}})
		done <- err
	}()

	select {
	case <-done:
		t.Fatal("Ask with zero timeout must block until answered, not return on its own")
	case <-time.After(120 * time.Millisecond):
		// Good: still blocked, as expected for interactive use.
	}

	// Clean up so the goroutine doesn't linger: answer the prompt.
	c.approval.mu.Lock()
	var ids []string
	for id := range c.approval.asks {
		ids = append(ids, id)
	}
	c.approval.mu.Unlock()

	for _, id := range ids {
		c.AnswerQuestion(id, []event.AskAnswer{{QuestionID: "q1", Selected: []string{"x"}}})
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Ask did not unblock after answering")
	}
}
