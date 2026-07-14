package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"reames-agent/internal/agent/testutil"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestSessionSyncFailureStopsBeforeProvider(t *testing.T) {
	prov := testutil.NewMock("sync", testutil.Turn{Text: "must not run"})
	want := errors.New("disk unavailable")
	a := New(prov, tool.NewRegistry(), NewSession("system"), Options{
		SessionSync: func(*Session) error { return want },
	}, event.Discard)

	err := a.Run(context.Background(), "persist me")
	if !errors.Is(err, want) {
		t.Fatalf("Run error = %v, want wrapped sync error", err)
	}
	if got := prov.CallCount(); got != 0 {
		t.Fatalf("provider calls = %d, want 0 before durable user boundary", got)
	}
	msgs := a.Session().Snapshot()
	if got := msgs[len(msgs)-1]; got.Role != provider.RoleUser || got.Content != "persist me" {
		t.Fatalf("last message = %+v, want in-memory user boundary", got)
	}
}

type sessionSyncOrderTool struct {
	executed bool
}

func (t *sessionSyncOrderTool) Name() string            { return "sync_order" }
func (t *sessionSyncOrderTool) Description() string     { return "test sync ordering" }
func (t *sessionSyncOrderTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *sessionSyncOrderTool) ReadOnly() bool          { return true }
func (t *sessionSyncOrderTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.executed = true
	return "executed", nil
}

func TestSessionSyncPersistsToolEnvelopeBeforeExecution(t *testing.T) {
	called := &sessionSyncOrderTool{}
	reg := tool.NewRegistry()
	reg.Add(called)
	prov := testutil.NewMock("sync",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: called.Name(), Arguments: `{}`}}},
		testutil.Turn{Text: "done"},
	)
	var sawEnvelopeBeforeExecution bool
	var sawResultAfterExecution bool
	a := New(prov, reg, NewSession("system"), Options{
		SessionSync: func(sess *Session) error {
			msgs := sess.Snapshot()
			if len(msgs) == 0 {
				return nil
			}
			last := msgs[len(msgs)-1]
			if last.Role == provider.RoleAssistant && len(last.ToolCalls) == 1 {
				if called.executed {
					t.Fatal("tool executed before assistant tool-call envelope was persisted")
				}
				sawEnvelopeBeforeExecution = true
			}
			if last.Role == provider.RoleTool {
				if !called.executed {
					t.Fatal("tool result persisted before tool execution")
				}
				sawResultAfterExecution = true
			}
			return nil
		},
	}, event.Discard)

	if err := a.Run(context.Background(), "run the tool"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sawEnvelopeBeforeExecution || !sawResultAfterExecution {
		t.Fatalf("sync boundaries envelope/result = %v/%v, want true/true", sawEnvelopeBeforeExecution, sawResultAfterExecution)
	}
}

func TestCompactNowSyncsRewrittenTranscript(t *testing.T) {
	sess := NewSession("system")
	for i := 0; i < 8; i++ {
		sess.Add(provider.Message{Role: provider.RoleUser, Content: strings.Repeat("user detail ", 80)})
		sess.Add(provider.Message{Role: provider.RoleAssistant, Content: strings.Repeat("assistant work ", 80)})
	}
	prov := &fakeProvider{reply: "durable compacted summary"}
	var persisted []provider.Message
	a := New(prov, tool.NewRegistry(), sess, Options{
		RecentKeep: 2,
		SessionSync: func(current *Session) error {
			persisted = current.Snapshot()
			return nil
		},
	}, event.Discard)

	if err := a.CompactNow(context.Background(), "retain recovery evidence"); err != nil {
		t.Fatalf("CompactNow: %v", err)
	}
	if len(persisted) == 0 {
		t.Fatal("compaction did not publish a durable recovery point")
	}
	found := false
	for _, msg := range persisted {
		if strings.Contains(msg.Content, "durable compacted summary") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("persisted transcript does not contain compacted summary: %+v", persisted)
	}
	if len(persisted) >= 17 {
		t.Fatalf("persisted message count = %d, want rewritten transcript smaller than original", len(persisted))
	}
}
