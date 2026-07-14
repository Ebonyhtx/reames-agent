package agent

import (
	"context"
	"strings"
	"testing"

	"reames-agent/internal/agent/testutil"
	"reames-agent/internal/event"
	"reames-agent/internal/memory"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestMemoryRecallIsExplainableDeletableAndKeepsStablePrefix(t *testing.T) {
	store := memory.Store{Dir: t.TempDir()}
	if _, err := store.Save(memory.Memory{
		Name:        "crash-recovery-boundary",
		Title:       "Crash recovery boundary",
		Description: "Subagent recovery must resume from durable boundaries",
		Type:        memory.TypeProject,
		Body:        "Use explicit interrupted continuation and verify workspace state before repeating side effects.",
	}); err != nil {
		t.Fatalf("Save memory: %v", err)
	}

	reg := tool.NewRegistry()
	reg.Add(memory.NewRecallTool(store))
	prov := testutil.NewMock("memory",
		testutil.Turn{ToolCalls: []provider.ToolCall{{
			ID:        "memory-1",
			Name:      "memory",
			Arguments: `{"operation":"search","query":"interrupted durable recovery","limit":5}`,
		}}},
		testutil.Turn{Text: "used the recovered fact"},
	)
	const stableSystem = "cache-stable-system-prefix"
	a := New(prov, reg, NewSession(stableSystem), Options{}, event.Discard)
	if err := a.Run(context.Background(), "recover the task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reqs := prov.Requests()
	if len(reqs) != 2 {
		t.Fatalf("provider requests = %d, want tool round and final round", len(reqs))
	}
	for i, req := range reqs {
		if len(req.Messages) == 0 || req.Messages[0].Role != provider.RoleSystem || req.Messages[0].Content != stableSystem {
			t.Fatalf("request %d system prefix = %+v, want unchanged %q", i, req.Messages, stableSystem)
		}
	}
	var recallResult string
	for _, msg := range reqs[1].Messages {
		if msg.Role == provider.RoleTool && msg.Name == "memory" {
			recallResult = msg.Content
		}
	}
	for _, want := range []string{"score=", "path:", "snippet:", "crash-recovery-boundary"} {
		if !strings.Contains(recallResult, want) {
			t.Fatalf("memory result = %q, want explainability field %q", recallResult, want)
		}
		if strings.Contains(reqs[1].Messages[0].Content, want) {
			t.Fatalf("dynamic recall field %q leaked into system prefix", want)
		}
	}

	if err := store.Delete("crash-recovery-boundary"); err != nil {
		t.Fatalf("Delete memory: %v", err)
	}
	out, err := memory.NewRecallTool(store).Execute(context.Background(), []byte(`{"operation":"search","query":"interrupted durable recovery"}`))
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if strings.Contains(out, "crash-recovery-boundary") || !strings.Contains(out, "No saved memories matched") {
		t.Fatalf("search after delete = %q, want no active hit", out)
	}
	disabled, err := memory.NewRecallTool(memory.Store{}).Execute(context.Background(), []byte(`{"operation":"list"}`))
	if err != nil {
		t.Fatalf("disabled memory: %v", err)
	}
	if disabled != "Memory store is unavailable." {
		t.Fatalf("disabled memory output = %q", disabled)
	}
}
