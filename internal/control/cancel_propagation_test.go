package control

import (
	"context"
	"sync"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

// TestCancelPropagation_StopsProviderStream verifies that Cancel()
// propagates context cancellation to the provider, causing the stream
// to terminate and the runtime status to clear.
func TestCancelPropagation_StopsProviderStream(t *testing.T) {
	// Use a slow provider that blocks until context is cancelled.
	prov := &slowProvider{delay: 10 * time.Second}
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {}))
	c := New(Options{Runner: ag, Executor: ag, Sink: event.FuncSink(func(e event.Event) {})})

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) { events <- e })
	c.mu.Unlock()

	// Start a turn.
	c.Submit("slow request")

	// Give it a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	c.Cancel()

	// Wait for the turn to finish.
	select {
	case e := <-events:
		if e.Kind != event.TurnDone {
			t.Fatalf("expected TurnDone after cancel, got %v", e.Kind)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for TurnDone after cancel")
	}

	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running after cancel")
	}
}

// TestCancelPropagation_Idempotent verifies that calling Cancel()
// multiple times does not panic or corrupt state.
func TestCancelPropagation_Idempotent(t *testing.T) {
	prov := &slowProvider{delay: 5 * time.Second}
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {}))
	c := New(Options{Runner: ag, Executor: ag, Sink: event.FuncSink(func(e event.Event) {})})

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) { events <- e })
	c.mu.Unlock()

	c.Submit("request")
	time.Sleep(20 * time.Millisecond)

	// Cancel multiple times — should not panic.
	c.Cancel()
	c.Cancel()
	c.Cancel()

	// Wait for TurnDone.
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out after cancel")
	}

	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running after cancel")
	}
}

// TestCancelPropagation_ClearsCancelFlagAfterDone verifies that
// CancelRequested resets to false after the turn completes.
func TestCancelPropagation_ClearsCancelFlagAfterDone(t *testing.T) {
	prov := &slowProvider{delay: 3 * time.Second}
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {}))
	c := New(Options{Runner: ag, Executor: ag, Sink: event.FuncSink(func(e event.Event) {})})

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) { events <- e })
	c.mu.Unlock()

	c.Submit("request")
	time.Sleep(20 * time.Millisecond)
	c.Cancel()

	if !c.CancelRequested() {
		t.Fatal("CancelRequested should be true after Cancel()")
	}

	// Wait for turn to finish.
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	if c.CancelRequested() {
		t.Fatal("CancelRequested should be false after TurnDone")
	}
	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running")
	}
}

// TestCancelPropagation_AllowsNewTurnAfterCancel verifies that
// a new turn can be submitted after cancellation completes.
func TestCancelPropagation_AllowsNewTurnAfterCancel(t *testing.T) {
	prov := &slowProvider{delay: 3 * time.Second}
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.FuncSink(func(e event.Event) {}))
	c := New(Options{Runner: ag, Executor: ag, Sink: event.FuncSink(func(e event.Event) {})})

	events := make(chan event.Event, 32)
	c.mu.Lock()
	c.sink = event.FuncSink(func(e event.Event) { events <- e })
	c.mu.Unlock()

	// First turn — cancel it.
	c.Submit("first")
	time.Sleep(20 * time.Millisecond)
	c.Cancel()
	<-events // drain TurnDone

	time.Sleep(20 * time.Millisecond)

	// Second turn — should work.
	c.Submit("second")
	c.Cancel()
	<-events

	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running after second cancel")
	}
}

// --- slow provider that respects context cancellation ---

type slowProvider struct {
	mu    sync.Mutex
	delay time.Duration
	calls int
}

func (p *slowProvider) Name() string { return "slow" }

func (p *slowProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	ch := make(chan provider.Chunk, 1)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			ch <- provider.Chunk{Type: provider.ChunkError, Err: ctx.Err()}
		case <-time.After(p.delay):
			ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
			ch <- provider.Chunk{Type: provider.ChunkDone}
		}
	}()
	return ch, nil
}
