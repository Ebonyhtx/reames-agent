package control

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/permission"
	"reames-agent/internal/provider"
	"reames-agent/internal/sandbox"
	"reames-agent/internal/tool"
	"reames-agent/internal/tool/builtin"
)

func TestProviderAuthErrorEmitsTurnDoneAndClearsRuntimeStatus(t *testing.T) {
	authErr := &provider.AuthError{Provider: "deepseek", KeyEnv: "DEEPSEEK_API_KEY", Status: 401, HasKey: true}
	prov := &scriptedTurns{turns: [][]provider.Chunk{{
		{Type: provider.ChunkError, Err: authErr},
	}}}
	events := make(chan event.Event, 16)
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {
		events <- e
	}))
	c := New(Options{
		Runner:   ag,
		Executor: ag,
		Sink: event.FuncSink(func(e event.Event) {
			events <- e
		}),
	})

	c.Submit("hello")
	done := waitForTurnDoneEvent(t, events)
	if done.Err == nil {
		t.Fatal("TurnDone.Err is nil, want provider auth error")
	}
	if !strings.Contains(done.Err.Error(), "DEEPSEEK_API_KEY") {
		t.Fatalf("TurnDone.Err = %v, want actionable key env", done.Err)
	}
	status := c.RuntimeStatus()
	if status.Running || status.PendingPrompt || status.CancelRequested || status.Cancellable {
		t.Fatalf("runtime status after auth failure = %+v, want idle", status)
	}
}

func TestProviderAPIErrorEmitsActionableTurnDoneAndClearsRuntimeStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantSubstr []string
	}{
		{
			name:       "rate limit",
			err:        &provider.APIError{Provider: "deepseek", Status: 429, Body: `{"error":{"message":"slow down"}}`},
			wantSubstr: []string{"HTTP 429"},
		},
		{
			name:       "server busy",
			err:        &provider.APIError{Provider: "deepseek", Status: 503, Body: "temporarily unavailable"},
			wantSubstr: []string{"HTTP 503"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov := &scriptedTurns{turns: [][]provider.Chunk{{
				{Type: provider.ChunkError, Err: tt.err},
			}}}
			events := make(chan event.Event, 16)
			ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {
				events <- e
			}))
			c := New(Options{
				Runner:   ag,
				Executor: ag,
				Sink: event.FuncSink(func(e event.Event) {
					events <- e
				}),
			})

			c.Submit("hello")
			done := waitForTurnDoneEvent(t, events)
			if done.Err == nil {
				t.Fatal("TurnDone.Err is nil, want provider API error")
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(done.Err.Error(), want) {
					t.Fatalf("TurnDone.Err = %q, want substring %q", done.Err.Error(), want)
				}
			}
			status := c.RuntimeStatus()
			if status.Running || status.PendingPrompt || status.CancelRequested || status.Cancellable {
				t.Fatalf("runtime status after provider API failure = %+v, want idle", status)
			}
		})
	}
}

func TestProviderStreamInterruptionExhaustionEmitsTurnDoneAndClearsRuntimeStatus(t *testing.T) {
	prov := &scriptedTurns{turns: [][]provider.Chunk{{
		{Type: provider.ChunkText, Text: "partial answer"},
		{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: errors.New("connection reset by peer")}},
	}}}
	events := make(chan event.Event, 32)
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {
		events <- e
	}))
	c := New(Options{
		Runner:   ag,
		Executor: ag,
		Sink: event.FuncSink(func(e event.Event) {
			events <- e
		}),
	})

	c.Submit("hello")
	done := waitForTurnDoneEvent(t, events)
	if done.Err == nil {
		t.Fatal("TurnDone.Err is nil, want exhausted stream interruption")
	}
	errText := done.Err.Error()
	for _, want := range []string{"model stream interrupted", "continue", "connection reset by peer"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("TurnDone.Err = %q, want substring %q", errText, want)
		}
	}
	if prov.call != 4 {
		t.Fatalf("provider stream calls = %d, want initial call plus 3 recovery attempts", prov.call)
	}
	status := c.RuntimeStatus()
	if status.Running || status.PendingPrompt || status.CancelRequested || status.Cancellable {
		t.Fatalf("runtime status after stream interruption exhaustion = %+v, want idle", status)
	}
}

func TestApprovalTimeoutBlocksWriteAndClearsPendingPrompt(t *testing.T) {
	workspace := t.TempDir()
	rel := filepath.Join("notes", "timed-out.txt")
	args, err := json.Marshal(struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{Path: rel, Content: "should not be written\n"})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: workspace}).Tools("write_file") {
		reg.Add(tl)
	}
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		toolCallTurn("w-timeout", "write_file", string(args)),
		textTurn("I could not write the file."),
	}}
	events := make(chan event.Event, 32)
	sink := event.FuncSink(func(e event.Event) { events <- e })
	ag := agent.New(prov, reg, agent.NewSession("sys"), agent.Options{}, sink)
	c := New(Options{
		Runner:          ag,
		Executor:        ag,
		Policy:          permission.New("ask", nil, nil, nil),
		Sink:            sink,
		WorkspaceRoot:   workspace,
		ApprovalTimeout: 30 * time.Millisecond,
	})
	c.EnableInteractiveApproval()

	c.Submit("write the timeout note")

	var result event.Event
	deadline := time.After(5 * time.Second)
	for result.Kind == 0 {
		select {
		case e := <-events:
			if e.Kind == event.ToolResult && e.Tool.ID == "w-timeout" {
				result = e
			}
		case <-deadline:
			t.Fatal("timed out waiting for write_file ToolResult")
		}
	}
	if result.Tool.Err == "" || !strings.Contains(result.Tool.Err, context.DeadlineExceeded.Error()) {
		t.Fatalf("ToolResult.Err = %q, want approval timeout", result.Tool.Err)
	}

	done := waitForTurnDoneEvent(t, events)
	if done.Err != nil {
		t.Fatalf("TurnDone.Err = %v, want nil because the model received the blocked tool result", done.Err)
	}
	if _, err := os.Stat(filepath.Join(workspace, rel)); !os.IsNotExist(err) {
		t.Fatalf("timed-out write stat err = %v, want file not created", err)
	}
	status := c.RuntimeStatus()
	if status.Running || status.PendingPrompt || status.CancelRequested || status.Cancellable {
		t.Fatalf("runtime status after approval timeout = %+v, want idle", status)
	}
}

func TestUserDeniedApprovalBlocksWriteAndClearsPendingPrompt(t *testing.T) {
	workspace := t.TempDir()
	rel := filepath.Join("notes", "denied.txt")
	args, err := json.Marshal(struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{Path: rel, Content: "should not be written\n"})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: workspace}).Tools("write_file") {
		reg.Add(tl)
	}
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		toolCallTurn("w-denied", "write_file", string(args)),
		textTurn("I did not write the file because permission was denied."),
	}}
	events := make(chan event.Event, 32)
	approvalID := make(chan string, 1)
	sink := event.FuncSink(func(e event.Event) {
		events <- e
		if e.Kind == event.ApprovalRequest && e.Approval.Tool == "write_file" {
			approvalID <- e.Approval.ID
		}
	})
	ag := agent.New(prov, reg, agent.NewSession("sys"), agent.Options{}, sink)
	c := New(Options{
		Runner:        ag,
		Executor:      ag,
		Policy:        permission.New("ask", nil, nil, nil),
		Sink:          sink,
		WorkspaceRoot: workspace,
	})
	c.EnableInteractiveApproval()

	go func() { c.Approve(<-approvalID, false, false, false) }()
	c.Submit("write the denied note")

	var result event.Event
	deadline := time.After(5 * time.Second)
	for result.Kind == 0 {
		select {
		case e := <-events:
			if e.Kind == event.ToolResult && e.Tool.ID == "w-denied" {
				result = e
			}
		case <-deadline:
			t.Fatal("timed out waiting for denied write_file ToolResult")
		}
	}
	if result.Tool.Err == "" || !strings.Contains(result.Tool.Err, "permission policy") {
		t.Fatalf("ToolResult.Err = %q, want permission denial", result.Tool.Err)
	}

	done := waitForTurnDoneEvent(t, events)
	if done.Err != nil {
		t.Fatalf("TurnDone.Err = %v, want nil because the model received the denied tool result", done.Err)
	}
	if _, err := os.Stat(filepath.Join(workspace, rel)); !os.IsNotExist(err) {
		t.Fatalf("denied write stat err = %v, want file not created", err)
	}
	status := c.RuntimeStatus()
	if status.Running || status.PendingPrompt || status.CancelRequested || status.Cancellable {
		t.Fatalf("runtime status after approval denial = %+v, want idle", status)
	}
}

func TestToolTimeoutEmitsToolResultAndClearsRuntimeStatus(t *testing.T) {
	workspace := t.TempDir()
	args, err := json.Marshal(struct {
		Command string `json:"command"`
	}{Command: failureLongSleepCommand(sandbox.ResolveShell("", "", nil))})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: workspace, BashTimeout: 150 * time.Millisecond}).Tools("bash") {
		reg.Add(tl)
	}
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		toolCallTurn("bash-timeout", "bash", string(args)),
		textTurn("The shell command timed out; I stopped waiting."),
	}}
	events := make(chan event.Event, 32)
	sink := event.FuncSink(func(e event.Event) { events <- e })
	ag := agent.New(prov, reg, agent.NewSession("sys"), agent.Options{}, sink)
	c := New(Options{
		Runner:        ag,
		Executor:      ag,
		Sink:          sink,
		WorkspaceRoot: workspace,
	})

	c.Submit("run a command that should time out")

	var result event.Event
	deadline := time.After(5 * time.Second)
	for result.Kind == 0 {
		select {
		case e := <-events:
			if e.Kind == event.ToolResult && e.Tool.ID == "bash-timeout" {
				result = e
			}
		case <-deadline:
			t.Fatal("timed out waiting for bash ToolResult")
		}
	}
	if result.Tool.Err == "" || !strings.Contains(result.Tool.Err, "timed out") {
		t.Fatalf("ToolResult.Err = %q, want bash timeout", result.Tool.Err)
	}

	done := waitForTurnDoneEvent(t, events)
	if done.Err != nil {
		t.Fatalf("TurnDone.Err = %v, want nil because the model received the timeout tool result", done.Err)
	}
	status := c.RuntimeStatus()
	if status.Running || status.PendingPrompt || status.CancelRequested || status.Cancellable {
		t.Fatalf("runtime status after tool timeout = %+v, want idle", status)
	}
}

func failureLongSleepCommand(sh sandbox.Shell) string {
	if sh.Kind == sandbox.ShellPowerShell {
		return "Start-Sleep -Seconds 2"
	}
	return "sleep 2"
}

func waitForTurnDoneEvent(t *testing.T, events <-chan event.Event) event.Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-events:
			if e.Kind == event.TurnDone {
				return e
			}
		case <-deadline:
			t.Fatal("timed out waiting for TurnDone")
		}
	}
}
