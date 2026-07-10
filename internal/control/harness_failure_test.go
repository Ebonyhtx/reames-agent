package control

import (
	"strings"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/provider/harness"
	"reames-agent/internal/provider/openai"
	"reames-agent/internal/tool"
)

// newHarnessController creates a Controller backed by a localhost harness
// server speaking the OpenAI protocol via the real openai provider.
func newHarnessController(t *testing.T, script harness.Script) (*Controller, *harness.Server) {
	t.Helper()
	srv := harness.MustNew(script)

	prov, err := openai.New(provider.Config{
		Name:    "harness",
		BaseURL: srv.URL(),
		APIKey:  "sk-" + "test-harness",
		Model:   "harness-model",
	})
	if err != nil {
		t.Fatalf("openai.New: %v", err)
	}

	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("system"), agent.Options{}, event.FuncSink(func(e event.Event) {}))
	c := New(Options{
		Runner:   ag,
		Executor: ag,
		Sink:     event.FuncSink(func(e event.Event) {}),
	})

	return c, srv
}

func TestProviderAuthErrorViaHarness(t *testing.T) {
	c, srv := newHarnessController(t, harness.Script{
		harness.AuthError401("DEEPSEEK_API_KEY"),
	})
	defer srv.Close()

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) {
		events <- e
	})
	c.mu.Unlock()

	c.Submit("hello")
	done := waitForTurnDoneEvent(t, events)
	if done.Err == nil {
		t.Fatal("TurnDone.Err is nil, want auth error")
	}
	if !strings.Contains(done.Err.Error(), "auth") && !strings.Contains(done.Err.Error(), "401") {
		t.Fatalf("TurnDone.Err = %v, want auth-related error", done.Err)
	}

	status := c.RuntimeStatus()
	if status.Running || status.CancelRequested {
		t.Fatalf("runtime status after auth error = %+v, want idle", status)
	}
}

func TestRecoveryAfterProviderFailureViaHarness(t *testing.T) {
	c, srv := newHarnessController(t, harness.Script{
		harness.AuthError401("KEY"),
		harness.TextChunk("recovered successfully"),
	})
	defer srv.Close()

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) {
		events <- e
	})
	c.mu.Unlock()

	c.Submit("fail")
	done1 := waitForTurnDoneEvent(t, events)
	if done1.Err == nil {
		t.Fatal("first turn should have failed with auth error")
	}
	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running after failed turn")
	}

	time.Sleep(20 * time.Millisecond)

	c.Submit("recover")
	done2 := waitForTurnDoneEvent(t, events)
	if done2.Err != nil {
		t.Fatalf("second turn should succeed, got error: %v", done2.Err)
	}

	status := c.RuntimeStatus()
	if status.Running {
		t.Fatal("runtime should not be running after recovery turn")
	}
}

func TestCancelDuringHarnessDelay(t *testing.T) {
	c, srv := newHarnessController(t, harness.Script{
		{Status: 200, Chunks: []harness.Chunk{{Text: "slow-response"}}, DelayBefore: 3 * time.Second},
	})
	defer srv.Close()

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) {
		events <- e
	})
	c.mu.Unlock()

	c.Submit("slow")

	time.Sleep(100 * time.Millisecond)
	c.Cancel()

	_ = waitForTurnDoneEvent(t, events)

	status := c.RuntimeStatus()
	if status.Running {
		t.Fatal("runtime should not be running after cancel during delayed response")
	}
}
