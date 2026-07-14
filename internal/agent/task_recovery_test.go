package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent/testutil"
	"reames-agent/internal/event"
	"reames-agent/internal/jobs"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

type durableBoundaryTool struct {
	started chan struct{}
	release chan struct{}
}

func (t *durableBoundaryTool) Name() string            { return "durable_boundary" }
func (t *durableBoundaryTool) Description() string     { return "wait at a durable boundary" }
func (t *durableBoundaryTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *durableBoundaryTool) ReadOnly() bool          { return false }
func (t *durableBoundaryTool) Execute(context.Context, json.RawMessage) (string, error) {
	close(t.started)
	<-t.release
	return "tool completed", nil
}

func TestBackgroundTaskPersistsToolEnvelopeBeforeToolExecution(t *testing.T) {
	blocking := &durableBoundaryTool{started: make(chan struct{}), release: make(chan struct{})}
	reg := tool.NewRegistry()
	reg.Add(blocking)
	prov := testutil.NewMock("subagent",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "write-1", Name: blocking.Name(), Arguments: `{}`}}},
		testutil.Turn{Text: "background complete"},
	)
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	task := NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, t.TempDir(), "base-model", "base-effort")

	jm := jobs.NewManager(event.Discard)
	defer jm.Close()
	jm.SetActiveSessionPath("parent-session", filepath.Join(t.TempDir(), "parent-session.jsonl"))
	ctx := jobs.WithManager(jobs.WithSession(testTaskContext(), "parent-session"), jm)
	startOut, err := task.Execute(ctx, []byte(`{"prompt":"perform durable work","run_in_background":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	ref := subagentRefFromOutput(t, startOut)
	jobID := extractJobID(startOut)
	<-blocking.started

	loaded, err := LoadSession(store.sessionPath(ref))
	if err != nil {
		t.Fatalf("LoadSession while tool is executing: %v", err)
	}
	foundEnvelope := false
	for _, msg := range loaded.Snapshot() {
		if msg.Role == provider.RoleAssistant && len(msg.ToolCalls) == 1 && msg.ToolCalls[0].ID == "write-1" {
			foundEnvelope = true
			break
		}
	}
	if !foundEnvelope {
		t.Fatalf("durable transcript did not contain tool-call envelope before execution: %+v", loaded.Snapshot())
	}
	meta, err := store.LoadMeta(ref)
	if err != nil {
		t.Fatalf("LoadMeta while running: %v", err)
	}
	if meta.Status != SubagentRunning {
		t.Fatalf("running status = %q, want %q", meta.Status, SubagentRunning)
	}

	close(blocking.release)
	res := jm.WaitForSession(context.Background(), "parent-session", []string{jobID}, 5)
	if len(res) != 1 || res[0].Status != jobs.Done || !strings.Contains(res[0].Output, "background complete") {
		t.Fatalf("background result = %+v, want completed durable task", res)
	}
	meta, err = store.LoadMeta(ref)
	if err != nil {
		t.Fatalf("LoadMeta completed: %v", err)
	}
	if meta.Status != SubagentCompleted {
		t.Fatalf("completed status = %q, want %q", meta.Status, SubagentCompleted)
	}
}

func TestInterruptedTaskContinueInjectsRecoveryContext(t *testing.T) {
	reg := tool.NewRegistry()
	prov := &mockProvider{name: "subagent", chunks: []provider.Chunk{
		{Type: provider.ChunkText, Text: "recovered safely"},
		{Type: provider.ChunkDone},
	}}
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	task := NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, t.TempDir(), "base-model", "base-effort")

	subReg := task.buildSubReg(nil, 1)
	run, err := task.prepareTranscriptRun(subReg, "", "", "parent-session", "root-call", "", "")
	if err != nil {
		t.Fatalf("prepareTranscriptRun: %v", err)
	}
	run.Session.Add(provider.Message{Role: provider.RoleUser, Content: "work before crash"})
	if err := store.MarkRunning(run); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	ref := run.Ref
	run.Release()
	if cleaned, err := store.CleanupStaleRunning(); err != nil || cleaned != 1 {
		t.Fatalf("CleanupStaleRunning = %d, %v; want 1, nil", cleaned, err)
	}

	out, err := task.Execute(testTaskContext(), []byte(`{"prompt":"resume carefully","continue_from":"`+ref+`"}`))
	if err != nil {
		t.Fatalf("Execute continuation: %v", err)
	}
	if !strings.Contains(out, "recovered safely") {
		t.Fatalf("continuation output = %q, want recovered answer", out)
	}
	user := lastUser(prov.lastReq)
	for _, want := range []string{
		`<recovery-context event="InterruptedSubagentResume">`,
		"may have completed, partially completed, or not started",
		"Do not replay side-effecting work",
		"resume carefully",
	} {
		if !strings.Contains(user, want) {
			t.Fatalf("recovery user message = %q, want %q", user, want)
		}
	}
	meta, err := store.LoadMeta(ref)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Status != SubagentCompleted {
		t.Fatalf("continued status = %q, want completed", meta.Status)
	}
}

func TestCompactedSubagentTranscriptSurvivesInterruptedContinuation(t *testing.T) {
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	spec := testSubagentSpec(t, "review")
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	for i := 0; i < 8; i++ {
		run.Session.Add(provider.Message{Role: provider.RoleUser, Content: strings.Repeat("durable user fact ", 80)})
		run.Session.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("completed analysis ", 80)})
	}
	if err := store.MarkRunning(run); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	prov := &fakeProvider{reply: "compacted recovery digest"}
	a := New(prov, tool.NewRegistry(), run.Session, Options{
		RecentKeep: 2,
		SessionSync: func(current *Session) error {
			if current != run.Session {
				t.Fatal("compaction sync received a different session")
			}
			return store.SaveRunning(run)
		},
	}, event.Discard)
	if err := a.CompactNow(context.Background(), "retain durable recovery state"); err != nil {
		t.Fatalf("CompactNow: %v", err)
	}
	ref := run.Ref
	run.Release()
	if cleaned, err := store.CleanupStaleRunning(); err != nil || cleaned != 1 {
		t.Fatalf("CleanupStaleRunning = %d, %v; want 1, nil", cleaned, err)
	}

	continued, err := store.PrepareContinue(ref, spec)
	if err != nil {
		t.Fatalf("PrepareContinue: %v", err)
	}
	defer continued.Release()
	if !continued.ResumedFromInterrupted {
		t.Fatal("compacted continuation lost interrupted identity")
	}
	found := false
	for _, msg := range continued.Session.Snapshot() {
		if strings.Contains(msg.Content, "compacted recovery digest") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("continued transcript lost compacted digest: %+v", continued.Session.Snapshot())
	}
}
