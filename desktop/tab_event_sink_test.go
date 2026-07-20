package main

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reames-agent/internal/event"
)

type closeTrackingSink struct {
	closed atomic.Bool
}

func (s *closeTrackingSink) Emit(event.Event) {}

func (s *closeTrackingSink) Close() {
	s.closed.Store(true)
}

func TestTabEventSinkSetBotSinkClosesPreviousSink(t *testing.T) {
	sink := &tabEventSink{}
	first := &closeTrackingSink{}
	second := &closeTrackingSink{}

	sink.SetBotSink(first)
	if first.closed.Load() {
		t.Fatal("newly attached sink was closed")
	}

	sink.SetBotSink(second)
	if !first.closed.Load() {
		t.Fatal("previous sink was not closed when replaced")
	}
	if second.closed.Load() {
		t.Fatal("replacement sink was closed too early")
	}

	sink.SetBotSink(nil)
	if !second.closed.Load() {
		t.Fatal("second sink was not closed when cleared")
	}
}

func TestTabEventSinkDoesNotBlockOnRuntimeEventsEmit(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan string, 2)
	var calls atomic.Int32

	sink := &tabEventSink{tabID: "tab", ctx: context.Background()}
	sink.runtimeEvents.emit = func(_ context.Context, name string, payload ...interface{}) {
		if name != eventChannel {
			t.Errorf("event name = %q, want %q", name, eventChannel)
		}
		if len(payload) != 1 {
			t.Errorf("payload count = %d, want 1", len(payload))
			return
		}
		wire, ok := payload[0].(wireEventTab)
		if !ok {
			t.Errorf("payload type = %T, want wireEventTab", payload[0])
			return
		}
		delivered <- wire.Text
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	wrapped := event.Sync(sink)
	wrapped.Emit(event.Event{Kind: event.Text, Text: "one"})

	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first runtime emit did not start")
	}

	done := make(chan struct{})
	go func() {
		wrapped.Emit(event.Event{Kind: event.Text, Text: "two"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second event blocked behind runtime EventsEmit")
	}

	close(release)
	if got := <-delivered; got != "one" {
		t.Fatalf("first delivered event = %q, want one", got)
	}
	select {
	case got := <-delivered:
		if got != "two" {
			t.Fatalf("second delivered event = %q, want two", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second queued event was not delivered")
	}
}

func TestEmitProjectTreeChangedDoesNotBlockOnRuntimeEventsEmit(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	app := &App{ctx: context.Background()}
	app.runtimeEvents.emit = func(_ context.Context, name string, payload ...interface{}) {
		if name != "project-tree:changed" {
			t.Errorf("event name = %q, want project-tree:changed", name)
		}
		if len(payload) != 0 {
			t.Errorf("payload count = %d, want 0", len(payload))
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	app.emitProjectTreeChanged()
	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first project tree runtime emit did not start")
	}

	done := make(chan struct{})
	go func() {
		app.emitProjectTreeChanged()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("project tree event blocked behind runtime EventsEmit")
	}

	close(release)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if calls.Load() >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("runtime emit calls = %d, want at least 2", calls.Load())
}

func TestAsyncRuntimeEmitterDrainsBacklogInOrder(t *testing.T) {
	const backlog = 256

	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan string, backlog)
	var calls atomic.Int32

	emitter := &asyncRuntimeEmitter{}
	emitter.emit = func(_ context.Context, _ string, payload ...interface{}) {
		if len(payload) != 1 {
			t.Errorf("payload count = %d, want 1", len(payload))
			return
		}
		value, ok := payload[0].(string)
		if !ok {
			t.Errorf("payload type = %T, want string", payload[0])
			return
		}
		delivered <- value
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	ctx := context.Background()
	for i := 0; i < backlog; i++ {
		emitter.Emit(ctx, "agent:event", strconv.Itoa(i))
	}

	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first runtime emit did not start")
	}
	close(release)

	for i := 0; i < backlog; i++ {
		select {
		case got := <-delivered:
			if want := strconv.Itoa(i); got != want {
				t.Fatalf("delivered[%d] = %q, want %q", i, got, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for delivered event %d", i)
		}
	}
}

func TestAsyncRuntimeEmitterCoalescesStreamingDeltas(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan interface{}, 4)
	var calls atomic.Int32
	emitter := &asyncRuntimeEmitter{limit: 4}
	emitter.emit = func(_ context.Context, _ string, payload ...interface{}) {
		if len(payload) == 1 {
			delivered <- payload[0]
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	emitter.Emit(context.Background(), "hold", "first")
	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocking runtime emit did not start")
	}
	for i := 0; i < 100; i++ {
		emitter.Emit(context.Background(), eventChannel, toWireTab(event.Event{Kind: event.Text, Text: "x"}, "tab-a"))
	}
	if queued := emitter.queueStats(); queued != 1 {
		t.Fatalf("queue stats = %d, want one merged delta", queued)
	}
	close(release)
	if got := <-delivered; got != "first" {
		t.Fatalf("first delivered payload = %#v", got)
	}
	select {
	case raw := <-delivered:
		wire, ok := raw.(wireEventTab)
		if !ok || wire.Kind != "text" || wire.TabID != "tab-a" || wire.Text != strings.Repeat("x", 100) {
			t.Fatalf("merged wire event = %#v", raw)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("merged runtime delta was not delivered")
	}
}

func TestAsyncRuntimeEmitterBoundsBacklogAndBackpressuresCompletion(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan interface{}, 16)
	var calls atomic.Int32
	emitter := &asyncRuntimeEmitter{limit: 4}
	emitter.emit = func(_ context.Context, _ string, payload ...interface{}) {
		if len(payload) == 1 {
			delivered <- payload[0]
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	emitter.Emit(context.Background(), "hold", "first")
	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocking runtime emit did not start")
	}
	for i := 0; i < 4; i++ {
		kind := event.Text
		if i%2 == 1 {
			kind = event.Reasoning
		}
		emitter.Emit(context.Background(), eventChannel, toWireTab(event.Event{Kind: kind, Text: strconv.Itoa(i)}, "tab-"+strconv.Itoa(i)))
	}
	if queued := emitter.queueStats(); queued != 4 {
		t.Fatalf("full queue = %d, want 4", queued)
	}
	producerDone := make(chan struct{})
	go func() {
		emitter.Emit(context.Background(), eventChannel, toWireTab(event.Event{Kind: event.TurnDone}, "tab-final"))
		close(producerDone)
	}()
	select {
	case <-producerDone:
		t.Fatal("completion bypassed the full bounded queue")
	case <-time.After(30 * time.Millisecond):
	}
	if queued := emitter.queueStats(); queued != 4 {
		t.Fatalf("queue grew beyond limit: %d", queued)
	}
	close(release)
	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("completion producer did not resume after queue space opened")
	}

	foundDone := false
	deadline := time.After(time.Second)
	for !foundDone {
		select {
		case raw := <-delivered:
			if wire, ok := raw.(wireEventTab); ok && wire.Kind == "turn_done" && wire.TabID == "tab-final" {
				foundDone = true
			}
		case <-deadline:
			t.Fatal("authoritative turn_done was lost behind transient overflow")
		}
	}
}

func TestAsyncRuntimeEmitterCriticalBackpressureHonorsCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	emitter := &asyncRuntimeEmitter{limit: 1}
	emitter.emit = func(context.Context, string, ...interface{}) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	emitter.Emit(context.Background(), "hold", "first")
	select {
	case <-entered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocking runtime emit did not start")
	}
	emitter.Emit(context.Background(), eventChannel, toWireTab(event.Event{Kind: event.Notice, Text: "critical"}, "tab"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		emitter.Emit(ctx, eventChannel, toWireTab(event.Event{Kind: event.TurnDone}, "tab"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled critical producer remained blocked")
	}
	if queued := emitter.queueStats(); queued != 1 {
		t.Fatalf("critical queue size = %d", queued)
	}
	close(release)
}

func TestAsyncRuntimeEmitterClearRejectsBlockedOldGeneration(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	delivered := make(chan string, 4)
	var calls atomic.Int32
	emitter := &asyncRuntimeEmitter{limit: 1}
	emitter.emit = func(_ context.Context, _ string, payload ...interface{}) {
		if len(payload) == 1 {
			if value, ok := payload[0].(string); ok {
				delivered <- value
			}
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}

	emitter.Emit(context.Background(), "hold", "in-flight")
	<-entered
	emitter.Emit(context.Background(), "old", "queued-old")
	blockedDone := make(chan struct{})
	go func() {
		emitter.Emit(context.Background(), "old", "blocked-old")
		close(blockedDone)
	}()
	select {
	case <-blockedDone:
		t.Fatal("old producer did not block behind the full queue")
	case <-time.After(30 * time.Millisecond):
	}

	emitter.Clear()
	select {
	case <-blockedDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("clear did not release the old producer")
	}
	if queued := emitter.queueStats(); queued != 0 {
		t.Fatalf("queue after clear = %d, want 0", queued)
	}
	emitter.Emit(context.Background(), "new", "new-generation")
	close(release)

	if got := <-delivered; got != "in-flight" {
		t.Fatalf("first delivered = %q", got)
	}
	select {
	case got := <-delivered:
		if got != "new-generation" {
			t.Fatalf("post-clear delivered = %q, want new-generation", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("new generation event was not delivered")
	}
	select {
	case got := <-delivered:
		t.Fatalf("cleared old generation event leaked: %q", got)
	case <-time.After(30 * time.Millisecond):
	}
}

func BenchmarkAsyncRuntimeEmitterCoalescedBacklog(b *testing.B) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	emitter := &asyncRuntimeEmitter{limit: 64}
	emitter.emit = func(context.Context, string, ...interface{}) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	emitter.Emit(context.Background(), "hold", "first")
	<-entered
	wire := toWireTab(event.Event{Kind: event.Text, Text: "x"}, "tab")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		emitter.Emit(context.Background(), eventChannel, wire)
	}
	b.StopTimer()
	close(release)
}
