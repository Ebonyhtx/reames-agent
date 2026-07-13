package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/checkpoint"
	"reames-agent/internal/diff"
	"reames-agent/internal/event"
	"reames-agent/internal/evidence"
	"reames-agent/internal/instruction"
	"reames-agent/internal/jobs"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestWritableSubagentEffectsMergeIntoParentEvidenceAndCheckpoint(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "child.txt")
	args := mustEffectWriterArgs(t, path, "child output\n")

	childProvider := &scriptedProvider{name: "child", turns: [][]provider.Chunk{
		{toolCallChunk("child-write", "write_file", args), {Type: provider.ChunkDone}},
		{toolCallChunk("child-test", "bash", `{"command":"go test ./..."}`), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "implemented and verified"}, {Type: provider.ChunkDone}},
	}}
	root, store := newEffectsTestAgent(t, workspace, childProvider,
		&scriptedProvider{name: "root", turns: [][]provider.Chunk{
			{toolCallChunk("task-root", "task", `{"prompt":"implement child change"}`), {Type: provider.ChunkDone}},
			{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
		}}, &effectsWriter{}, fakeTool{name: "bash", readOnly: false})
	root.projectChecks = []instruction.VerifyCheck{{Command: "go test ./...", SourcePath: "AGENTS.md"}}

	if err := root.Run(WithParentSession(context.Background(), "parent-session"), "delegate implementation"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "child output\n" {
		t.Fatalf("child write = %q, %v", got, err)
	}

	receipts := root.evidence.Receipts(20)
	writeIndex, write := findEffectReceipt(receipts, "write_file")
	if write == nil || !write.Success || !write.MutationAttempt || write.Source != "subagent" || write.ParentToolCallID != "task-root" || write.SubagentDepth != 1 {
		t.Fatalf("merged write receipt = %+v", write)
	}
	_, command := findEffectReceipt(receipts, "bash")
	if command == nil || !command.Success || command.Command != "go test ./..." || command.Source != "subagent" {
		t.Fatalf("merged command receipt = %+v", command)
	}
	if !root.evidence.HasSuccessfulCommandAfter("go test ./...", writeIndex) {
		t.Fatal("parent evidence did not preserve child write -> verification ordering")
	}
	if touched := root.EvidenceSnapshot().Touched; len(touched) != 1 || !strings.EqualFold(filepath.Base(touched[0]), filepath.Base(path)) {
		t.Fatalf("parent touched paths = %v, want child path", touched)
	}

	metas := store.List()
	if len(metas) != 1 || len(metas[0].Paths) != 1 || metas[0].Paths[0] != path {
		t.Fatalf("checkpoint metadata = %+v, want child write", metas)
	}
	if _, deleted, err := store.RestoreCode(0); err != nil {
		t.Fatalf("RestoreCode: %v", err)
	} else if len(deleted) != 1 || deleted[0] != path {
		t.Fatalf("deleted = %v, want child file", deleted)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("child file still exists after parent checkpoint rewind: %v", err)
	}
}

func TestWritableSubagentPartialFailureKeepsSnapshotAndUncertainReceipt(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "partial.txt")
	args := mustEffectWriterArgs(t, path, "partial bytes\n")
	childProvider := &scriptedProvider{name: "child", turns: [][]provider.Chunk{
		{toolCallChunk("partial-write", "write_file", args), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "writer failed after touching disk"}, {Type: provider.ChunkDone}},
	}}
	root, store := newEffectsTestAgent(t, workspace, childProvider,
		&scriptedProvider{name: "root", turns: [][]provider.Chunk{
			{toolCallChunk("task-partial", "task", `{"prompt":"try child write"}`), {Type: provider.ChunkDone}},
			{{Type: provider.ChunkText, Text: "reported partial failure"}, {Type: provider.ChunkDone}},
		}}, &effectsWriter{failAfterWrite: true})

	if err := root.Run(WithParentSession(context.Background(), "parent-session"), "delegate risky write"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "partial bytes\n" {
		t.Fatalf("partial write = %q, %v", got, err)
	}
	_, receipt := findEffectReceipt(root.evidence.Receipts(20), "write_file")
	if receipt == nil || receipt.Success || !receipt.MutationAttempt || receipt.Source != "subagent" {
		t.Fatalf("partial receipt = %+v, want failed mutation attempt", receipt)
	}
	if _, ok := root.evidence.LatestSuccessfulWriterIndex(); ok {
		t.Fatal("failed child writer was accepted as successful evidence")
	}
	if _, ok := root.evidence.LatestWriterBoundaryIndex(); !ok {
		t.Fatal("failed child writer did not create a verification boundary")
	}
	if _, deleted, err := store.RestoreCode(0); err != nil || len(deleted) != 1 {
		t.Fatalf("RestoreCode partial = deleted %v, err %v", deleted, err)
	}
}

func TestWritableSubagentCancellationKeepsSnapshotAndFailedReceipt(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "cancelled.txt")
	args := mustEffectWriterArgs(t, path, "written before cancel\n")
	started := make(chan struct{})
	writer := &effectsWriter{waitForCancel: true, started: started}
	childProvider := &scriptedProvider{name: "child", turns: [][]provider.Chunk{
		{toolCallChunk("cancel-write", "write_file", args), {Type: provider.ChunkDone}},
	}}
	root, store := newEffectsTestAgent(t, workspace, childProvider,
		&scriptedProvider{name: "root", turns: [][]provider.Chunk{
			{toolCallChunk("task-cancel", "task", `{"prompt":"cancel child write"}`), {Type: provider.ChunkDone}},
		}}, writer)

	ctx, cancel := context.WithCancel(WithParentSession(context.Background(), "parent-session"))
	done := make(chan error, 1)
	go func() { done <- root.Run(ctx, "delegate cancellable write") }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("child writer did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled child did not unwind")
	}

	_, receipt := findEffectReceipt(root.evidence.Receipts(20), "write_file")
	if receipt == nil || receipt.Success || !receipt.MutationAttempt || receipt.ParentToolCallID != "task-cancel" {
		t.Fatalf("cancel receipt = %+v", receipt)
	}
	if _, deleted, err := store.RestoreCode(0); err != nil || len(deleted) != 1 {
		t.Fatalf("RestoreCode cancel = deleted %v, err %v", deleted, err)
	}
}

func TestBackgroundWritableSubagentCarriesEffectsIntoJobContext(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "background.txt")
	args := mustEffectWriterArgs(t, path, "background effect\n")
	childProvider := &scriptedProvider{name: "child", turns: [][]provider.Chunk{
		{toolCallChunk("background-write", "write_file", args), {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "background done"}, {Type: provider.ChunkDone}},
	}}
	registry := tool.NewRegistry()
	registry.Add(&effectsWriter{})
	task := NewTaskTool(childProvider, nil, registry, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(NewSubagentStore(t.TempDir()), workspace, "child-model", "")

	parentLedger := evidence.NewLedger()
	var changes []diff.Change
	effects := &SubagentEffects{
		ledgers: []subagentEffectLedger{{ledger: parentLedger, generation: parentLedger.Generation()}},
		preEditHooks: []PreEditHook{func(change diff.Change) error {
			changes = append(changes, change)
			return nil
		}},
		parentCallID: "task-background",
	}
	manager := jobs.NewManager(event.Discard)
	defer manager.Close()
	ctx := WithSubagentEffects(testTaskContext(), effects)
	ctx = jobs.WithSession(ctx, "parent-session")
	ctx = jobs.WithManager(ctx, manager)
	out, err := task.Execute(ctx, []byte(`{"prompt":"write in background","run_in_background":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	jobID := extractJobID(out)
	results := manager.WaitForSession(context.Background(), "parent-session", []string{jobID}, 5)
	if len(results) != 1 || results[0].Status != jobs.Done {
		t.Fatalf("background result = %+v", results)
	}
	_, receipt := findEffectReceipt(parentLedger.Receipts(10), "write_file")
	if receipt == nil || !receipt.Success || receipt.ParentToolCallID != "task-background" || receipt.SubagentDepth != 1 {
		t.Fatalf("background merged receipt = %+v", receipt)
	}
	if len(changes) != 1 || changes[0].Path != path {
		t.Fatalf("background pre-edit snapshots = %+v", changes)
	}
}

func TestSubagentEffectsRejectReceiptsFromAnOlderParentTurn(t *testing.T) {
	parentLedger := evidence.NewLedger()
	effects := &SubagentEffects{
		ledgers:      []subagentEffectLedger{{ledger: parentLedger, generation: parentLedger.Generation()}},
		parentCallID: "task-old-turn",
	}
	parentLedger.Reset()

	effects.record(evidence.Receipt{ToolName: "write_file", Success: true, Write: true, Paths: []string{"late.txt"}}, 1)
	if parentLedger.Len() != 0 {
		t.Fatalf("old-turn child receipt contaminated current turn: %+v", parentLedger.Receipts(10))
	}
}

func TestPreEditPersistenceFailureBlocksWriter(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "blocked.txt")
	reg := tool.NewRegistry()
	writer := &effectsWriter{}
	reg.Add(writer)
	a := New(nil, reg, NewSession("sys"), Options{}, event.Discard)
	a.SetPreEditHook(func(diff.Change) error {
		return errors.New("injected durable snapshot failure")
	})

	outcome := a.executeOne(context.Background(), provider.ToolCall{
		ID: "write-blocked", Name: "write_file", Arguments: mustEffectWriterArgs(t, path, "should not land"),
	})
	if !outcome.blocked || !strings.Contains(outcome.output, "injected durable snapshot failure") {
		t.Fatalf("writer outcome = %+v, want persistence refusal", outcome)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("blocked writer touched disk: %v", err)
	}
}

func TestAncestorPreEditPersistenceFailureBlocksChildWriter(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "child-blocked.txt")
	reg := tool.NewRegistry()
	reg.Add(&effectsWriter{})
	effects := &SubagentEffects{preEditHooks: []PreEditHook{func(diff.Change) error {
		return errors.New("injected ancestor checkpoint failure")
	}}}
	a := New(nil, reg, NewSession("sys"), Options{SubagentEffects: effects}, event.Discard)

	outcome := a.executeOne(context.Background(), provider.ToolCall{
		ID: "child-write-blocked", Name: "write_file", Arguments: mustEffectWriterArgs(t, path, "should not land"),
	})
	if !outcome.blocked || !strings.Contains(outcome.output, "injected ancestor checkpoint failure") {
		t.Fatalf("child writer outcome = %+v, want ancestor persistence refusal", outcome)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("blocked child writer touched disk: %v", err)
	}
}

func newEffectsTestAgent(t *testing.T, workspace string, childProvider, rootProvider provider.Provider, tools ...tool.Tool) (*Agent, *checkpoint.Store) {
	t.Helper()
	registry := tool.NewRegistry()
	for _, childTool := range tools {
		registry.Add(childTool)
	}
	task := NewTaskTool(childProvider, nil, registry, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(NewSubagentStore(t.TempDir()), workspace, "child-model", "")
	registry.Add(task)
	root := New(rootProvider, registry, NewSession("root sys"), Options{}, event.Discard)
	store := checkpoint.New("", workspace)
	store.Begin(0, "parent turn", 0)
	root.SetPreEditHook(store.Snapshot)
	return root, store
}

func mustEffectWriterArgs(t *testing.T, path, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"path": path, "content": content})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func findEffectReceipt(receipts []evidence.Receipt, toolName string) (int, *evidence.Receipt) {
	for i := range receipts {
		receipt := &receipts[i]
		if receipt.ToolName != toolName {
			continue
		}
		return i, receipt
	}
	return -1, nil
}

type effectsWriter struct {
	failAfterWrite bool
	waitForCancel  bool
	started        chan struct{}
}

func (*effectsWriter) Name() string            { return "write_file" }
func (*effectsWriter) Description() string     { return "test writer" }
func (*effectsWriter) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*effectsWriter) ReadOnly() bool          { return false }
func (*effectsWriter) PlanModeSafe() bool      { return false }
func (w *effectsWriter) Preview(args json.RawMessage) (diff.Change, error) {
	params, err := decodeEffectsWriterArgs(args)
	if err != nil {
		return diff.Change{}, err
	}
	old, err := os.ReadFile(params.Path)
	kind := diff.Modify
	if os.IsNotExist(err) {
		old = nil
		kind = diff.Create
	} else if err != nil {
		return diff.Change{}, err
	}
	return diff.Build(params.Path, string(old), params.Content, kind), nil
}

func (w *effectsWriter) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	params, err := decodeEffectsWriterArgs(args)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(params.Path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return "", err
	}
	if w.started != nil {
		close(w.started)
	}
	if w.waitForCancel {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if w.failAfterWrite {
		return "", errors.New("injected failure after write")
	}
	return "written", nil
}

type effectsWriterArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func decodeEffectsWriterArgs(args json.RawMessage) (effectsWriterArgs, error) {
	var params effectsWriterArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return params, err
	}
	if strings.TrimSpace(params.Path) == "" {
		return params, errors.New("path is required")
	}
	return params, nil
}
