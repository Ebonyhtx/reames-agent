package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"reames-agent/internal/agent/testutil"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

type steerThenCancelTool struct {
	agent     *Agent
	cancel    context.CancelFunc
	steerText string
	accepted  bool
}

func (*steerThenCancelTool) Name() string        { return "steer_then_cancel" }
func (*steerThenCancelTool) Description() string { return "queues a steer and cancels the turn" }
func (*steerThenCancelTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (*steerThenCancelTool) ReadOnly() bool { return true }
func (t *steerThenCancelTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.accepted = t.agent.Steer(t.steerText)
	t.cancel()
	return "ok", nil
}

func TestRunFlushesUnconsumedSteersOnCancel(t *testing.T) {
	mp := testutil.NewMock("m",
		testutil.Turn{ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "steer_then_cancel", Arguments: `{}`}}},
		testutil.Turn{Text: "never reached"},
	)
	hijack := &steerThenCancelTool{steerText: "use plan B"}
	reg := tool.NewRegistry()
	reg.Add(hijack)
	var steerEvents []string
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.Steer {
			steerEvents = append(steerEvents, e.Text)
		}
	})
	a := New(mp, reg, NewSession(""), Options{}, sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hijack.agent = a
	hijack.cancel = cancel

	err := a.Run(ctx, "go")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run should exit on cancellation, got %v", err)
	}
	if !hijack.accepted {
		t.Fatal("Steer during an active turn should be accepted")
	}

	var persisted []string
	for _, message := range a.Session().Messages {
		if message.Role != provider.RoleUser {
			continue
		}
		if text, ok := SteerText(message.Content); ok {
			persisted = append(persisted, text)
		}
	}
	if len(persisted) != 1 || persisted[0] != "use plan B" {
		t.Fatalf("persisted steers = %v, want [use plan B]", persisted)
	}
	if len(steerEvents) != 1 || steerEvents[0] != "use plan B" {
		t.Fatalf("steer events = %v, want [use plan B]", steerEvents)
	}
	if n := a.steerQueueLen(); n != 0 {
		t.Fatalf("steer queue length = %d, want 0", n)
	}
	if a.Steer("after the turn") {
		t.Fatal("Steer after the turn must be rejected")
	}
}

func TestSteerTextSurvivesTurnPreferenceWrapping(t *testing.T) {
	plain := New(nil, nil, NewSession(""), Options{}, event.Discard)
	explicit := New(nil, nil, NewSession(""), Options{}, event.Discard)
	explicit.SetReasoningLanguage("zh")
	explicit.SetResponseLanguage("zh")

	cases := []struct {
		name  string
		agent *Agent
		text  string
	}{
		{name: "english auto", agent: plain, text: "use plan B"},
		{name: "chinese auto", agent: plain, text: "请改用方案B"},
		{name: "explicit zh", agent: explicit, text: "switch to plan B"},
		{name: "exact whitespace", agent: plain, text: "  spaced\ttext  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			persisted := tc.agent.withTurnPreferences(midTurnSteerMessage(tc.text))
			got, ok := SteerText(persisted)
			if !ok || got != tc.text {
				t.Fatalf("SteerText(%q) = (%q, %v), want (%q, true)", persisted, got, ok, tc.text)
			}
		})
	}
	if _, ok := SteerText(plain.withTurnPreferences("请总结一下这个文件")); ok {
		t.Fatal("ordinary wrapped user message must not be detected as a steer")
	}
}

func TestSteerRejectedWithoutActiveTurn(t *testing.T) {
	a := New(testutil.NewMock("m", testutil.Turn{Text: "done"}), tool.NewRegistry(), NewSession(""), Options{}, event.Discard)
	if a.Steer("early") {
		t.Fatal("Steer with no active turn must be rejected")
	}
	if err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if a.Steer("between turns") {
		t.Fatal("Steer between turns must be rejected")
	}
}
