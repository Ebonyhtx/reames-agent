package control

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

func promptTestController(system string) (*Controller, *agent.Agent) {
	executor := agent.New(nil, nil, agent.NewSession(system), agent.Options{}, event.Discard)
	return New(Options{Runner: executor, Executor: executor, SystemPrompt: system, Sink: event.Discard}), executor
}

func TestMessagesWithSystemPromptReplacesOnlySystemSemantics(t *testing.T) {
	original := []provider.Message{{
		Role: provider.RoleSystem, Content: "old", ReasoningContent: "drop", ReasoningSignature: "drop",
		ToolCalls: []provider.ToolCall{{ID: "call"}}, ToolCallID: "call", Name: "tool",
	}, {Role: provider.RoleUser, Content: "task"}}
	got := messagesWithSystemPrompt(original, "new")
	if !reflect.DeepEqual(got[0], provider.Message{Role: provider.RoleSystem, Content: "new"}) {
		t.Fatalf("fresh system message = %+v", got[0])
	}
	if original[0].Content != "old" || original[0].ReasoningContent != "drop" {
		t.Fatal("system prompt conversion mutated carried history")
	}
}

func TestAdoptLoadedHistoryPreservesLegacySystemlessTranscript(t *testing.T) {
	controller, executor := promptTestController("fresh system")
	controller.AdoptLoadedHistoryWithCurrentSystemPrompt([]provider.Message{{Role: provider.RoleUser, Content: "legacy task"}}, "")
	got := executor.Session().Snapshot()
	if len(got) != 1 || got[0].Role != provider.RoleUser || got[0].Content != "legacy task" {
		t.Fatalf("legacy loaded history = %+v", got)
	}
}

func TestAdoptHistoryWithCurrentSystemPromptPreservesRewriteBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	saved := agent.NewSession("old sys")
	saved.Add(provider.Message{Role: provider.RoleUser, Content: "first"})
	saved.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "tool-1", Name: "read_file", Arguments: "{}"}}})
	saved.Add(provider.Message{Role: provider.RoleTool, ToolCallID: "tool-1", Name: "read_file", Content: strings.Repeat("detail ", 100)})
	saved.Add(provider.Message{Role: provider.RoleAssistant, Content: "done"})
	if err := saved.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	controller, executor := promptTestController("new sys")
	controller.AdoptHistoryWithCurrentSystemPrompt(loaded.Snapshot(), path)
	resumed := executor.Session()
	msgs := resumed.Snapshot()
	msgs[3].Content = "[elided tool result]"
	resumed.Replace(msgs)
	if err := resumed.SaveRewrite(path); err != nil {
		t.Fatalf("SaveRewrite fresh-system resume: %v", err)
	}
	reloaded, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Messages[0].Content != "new sys" || reloaded.Messages[3].Content != "[elided tool result]" {
		t.Fatalf("rewritten messages = %+v", reloaded.Messages)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*-recovery-*.jsonl")); err != nil || len(matches) != 0 {
		t.Fatalf("recovery branches = %v err=%v", matches, err)
	}
}

func TestAdoptHistoryWithCurrentSystemPromptRejectsStaleBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	current := agent.NewSession("old sys")
	current.Add(provider.Message{Role: provider.RoleUser, Content: "first"})
	current.Add(provider.Message{Role: provider.RoleAssistant, Content: "one"})
	current.Add(provider.Message{Role: provider.RoleUser, Content: "disk second"})
	current.Add(provider.Message{Role: provider.RoleAssistant, Content: "disk two"})
	if err := current.Save(path); err != nil {
		t.Fatal(err)
	}
	stale := []provider.Message{
		{Role: provider.RoleSystem, Content: "old sys"},
		{Role: provider.RoleUser, Content: "first"},
		{Role: provider.RoleAssistant, Content: "one"},
	}
	controller, executor := promptTestController("new sys")
	controller.AdoptHistoryWithCurrentSystemPrompt(stale, path)
	if err := executor.Session().SaveRewrite(path); !errors.Is(err, agent.ErrSessionSnapshotConflict) {
		t.Fatalf("SaveRewrite stale history err = %v", err)
	}
	reloaded, err := agent.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Messages[len(reloaded.Messages)-1].Content; got != "disk two" {
		t.Fatalf("disk tail = %q", got)
	}
}

func TestSessionHistorySnapshotIsOpaqueAndStableAcrossRebuild(t *testing.T) {
	source, sourceExecutor := promptTestController("system")
	sourceExecutor.Session().Add(provider.Message{Role: provider.RoleUser, Content: "captured"})
	history := CaptureSessionHistory(source)
	sourceExecutor.Session().Add(provider.Message{Role: provider.RoleAssistant, Content: "late mutation"})

	target, targetExecutor := promptTestController("system")
	target.AdoptSessionHistoryWithCurrentSystemPrompt(history, "")
	got := targetExecutor.Session().Snapshot()
	if len(got) != 2 || got[1].Content != "captured" {
		t.Fatalf("adopted opaque history = %+v", got)
	}
}
