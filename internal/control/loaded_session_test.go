package control

import (
	"path/filepath"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

func TestLoadedSessionAdoptsOpaqueHistoryAndRefreshesSystemPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loaded.jsonl")
	source := agent.NewSession("old system")
	source.Add(provider.Message{Role: provider.RoleUser, Content: "persisted task"})
	if err := source.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Empty() {
		t.Fatal("loaded non-empty session reported empty")
	}

	executor := agent.New(nil, nil, agent.NewSession("fresh system"), agent.Options{}, event.Discard)
	controller := New(Options{Executor: executor, SystemPrompt: "fresh system"})
	AdoptLoadedSessionWithCurrentSystemPrompt(controller, loaded, path)

	got := executor.Session().Snapshot()
	if len(got) != 2 || got[0].Content != "fresh system" || got[1].Content != "persisted task" {
		t.Fatalf("adopted loaded session = %+v", got)
	}
}

func TestLoadSessionRejectsCleanupPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.jsonl")
	if err := MarkSessionCleanupPending(path, "delete"); err != nil {
		t.Fatal(err)
	}
	if loaded, err := LoadSession(path); err == nil || loaded != nil {
		t.Fatalf("cleanup-pending load = loaded:%v err:%v", loaded, err)
	}
}
