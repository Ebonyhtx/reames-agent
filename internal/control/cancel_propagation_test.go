package control

import (
	"context"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestCancelPropagationStopsProviderStream(t *testing.T) {
	c, prov, events := newCancelPropagationController()
	c.Submit("slow request")
	waitCancelSignal(t, prov.started)
	c.Cancel()
	waitCancelSignal(t, prov.cancelled)
	waitForTurnDoneEvent(t, events)

	if status := c.RuntimeStatus(); status.Running || status.CancelRequested || status.Cancellable {
		t.Fatalf("runtime status after cancel = %+v, want idle", status)
	}
}

func TestCancelPropagationIsIdempotent(t *testing.T) {
	c, prov, events := newCancelPropagationController()
	c.Submit("request")
	waitCancelSignal(t, prov.started)
	c.Cancel()
	c.Cancel()
	c.Cancel()
	waitCancelSignal(t, prov.cancelled)
	waitForTurnDoneEvent(t, events)

	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running after repeated cancellation")
	}
}

func TestCancelRequestedClearsAfterDone(t *testing.T) {
	release := make(chan struct{})
	c, prov, events := newCancelPropagationController()
	prov.release = release
	c.Submit("request")
	waitCancelSignal(t, prov.started)
	c.Cancel()
	waitCancelSignal(t, prov.cancelled)
	if !c.CancelRequested() {
		t.Fatal("CancelRequested should remain true while the provider is unwinding")
	}
	close(release)
	waitForTurnDoneEvent(t, events)

	if status := c.RuntimeStatus(); status.Running || status.CancelRequested {
		t.Fatalf("runtime status after TurnDone = %+v, want cancellation cleared", status)
	}
}

func TestCancelPropagationAllowsNewTurn(t *testing.T) {
	c, prov, events := newCancelPropagationController()

	c.Submit("first")
	waitCancelSignal(t, prov.started)
	c.Cancel()
	waitCancelSignal(t, prov.cancelled)
	waitForTurnDoneEvent(t, events)

	c.Submit("second")
	waitCancelSignal(t, prov.started)
	c.Cancel()
	waitCancelSignal(t, prov.cancelled)
	waitForTurnDoneEvent(t, events)

	if c.RuntimeStatus().Running {
		t.Fatal("runtime should not be running after second cancellation")
	}
}

func newCancelPropagationController() (*Controller, *blockingProvider, <-chan event.Event) {
	prov := &blockingProvider{
		started:   make(chan struct{}, 2),
		cancelled: make(chan struct{}, 2),
	}
	events := make(chan event.Event, 32)
	sink := event.FuncSink(func(e event.Event) { events <- e })
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, sink)
	c := New(Options{Runner: ag, Executor: ag, Sink: sink})
	return c, prov, events
}

func waitCancelSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider synchronization signal")
	}
}

type blockingProvider struct {
	started   chan struct{}
	cancelled chan struct{}
	release   <-chan struct{}
}

func (p *blockingProvider) Name() string { return "blocking" }

func (p *blockingProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.started <- struct{}{}
	ch := make(chan provider.Chunk, 1)
	go func() {
		defer close(ch)
		<-ctx.Done()
		p.cancelled <- struct{}{}
		if p.release != nil {
			<-p.release
		}
		ch <- provider.Chunk{Type: provider.ChunkError, Err: ctx.Err()}
	}()
	return ch, nil
}
