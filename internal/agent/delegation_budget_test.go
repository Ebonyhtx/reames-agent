package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestDelegationLedgerCapsActiveProviderRounds(t *testing.T) {
	ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxConcurrent: 2, MaxSteps: 10})
	defer ledger.Close()

	acquired := make(chan struct{}, 4)
	release := make(chan struct{}, 4)
	errCh := make(chan error, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done, err := ledger.AcquireRound(ledger.Context())
			if err != nil {
				errCh <- err
				return
			}
			acquired <- struct{}{}
			<-release
			done()
		}()
	}

	for range 2 {
		select {
		case <-acquired:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for initial provider slots")
		}
	}
	select {
	case <-acquired:
		t.Fatal("more provider rounds acquired than max concurrency")
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	release <- struct{}{}
	for range 2 {
		select {
		case <-acquired:
		case <-time.After(time.Second):
			t.Fatal("queued provider round did not acquire a released slot")
		}
	}
	release <- struct{}{}
	release <- struct{}{}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("AcquireRound: %v", err)
	}

	snapshot := ledger.Snapshot()
	if snapshot.PeakRounds != 2 || snapshot.ActiveRounds != 0 || snapshot.Steps != 4 {
		t.Fatalf("snapshot = %+v, want peak=2 active=0 steps=4", snapshot)
	}
}

func TestDelegationLedgerWaitingCancellationDoesNotSpendStep(t *testing.T) {
	ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxConcurrent: 1, MaxSteps: 2})
	defer ledger.Close()

	first, err := ledger.AcquireRound(ledger.Context())
	if err != nil {
		t.Fatal(err)
	}
	child, cancelChild := context.WithCancel(ledger.Context())
	cancelChild()
	if _, err := ledger.AcquireRound(child); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error = %v, want context cancellation", err)
	}
	if got := ledger.Snapshot().Steps; got != 1 {
		t.Fatalf("cancelled waiter spent a step: got %d, want 1", got)
	}
	first()
	second, err := ledger.AcquireRound(ledger.Context())
	if err != nil {
		t.Fatalf("second real round: %v", err)
	}
	second()
	if _, err := ledger.AcquireRound(ledger.Context()); !errors.Is(err, ErrDelegationStepBudget) {
		t.Fatalf("third round error = %v, want step budget", err)
	}
	if !errors.Is(ledger.Cause(), ErrDelegationStepBudget) {
		t.Fatalf("ledger cause = %v, want step budget", ledger.Cause())
	}
}

func TestDelegationLedgerTokenCrossingCancelsTree(t *testing.T) {
	ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxTokens: 10})
	defer ledger.Close()

	if err := ledger.RecordUsage(&provider.Usage{TotalTokens: 6}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordUsage(&provider.Usage{TotalTokens: 5}); !errors.Is(err, ErrDelegationTokenBudget) {
		t.Fatalf("RecordUsage error = %v, want token budget", err)
	}
	select {
	case <-ledger.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("token crossing did not cancel the delegation tree")
	}
	snapshot := ledger.Snapshot()
	if snapshot.Tokens != 11 || !errors.Is(ledger.Cause(), ErrDelegationTokenBudget) {
		t.Fatalf("snapshot/cause = %+v / %v", snapshot, ledger.Cause())
	}
}

func TestDelegationLedgerDeadlineAndParentCancellation(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxDuration: 30 * time.Millisecond})
		defer ledger.Close()
		select {
		case <-ledger.Context().Done():
		case <-time.After(time.Second):
			t.Fatal("delegation deadline did not cancel the tree")
		}
		if !errors.Is(ledger.Cause(), ErrDelegationTimeBudget) {
			t.Fatalf("cause = %v, want time budget", ledger.Cause())
		}
	})

	t.Run("parent", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		ledger := NewDelegationLedger(parent, DelegationLimits{})
		defer ledger.Close()
		cancel()
		select {
		case <-ledger.Context().Done():
		case <-time.After(time.Second):
			t.Fatal("parent cancellation did not cancel the tree")
		}
		if !errors.Is(ledger.Cause(), context.Canceled) {
			t.Fatalf("cause = %v, want parent cancellation", ledger.Cause())
		}
	})
}

func TestDelegationLedgerLocalChildCancellationLeavesSiblingsRunning(t *testing.T) {
	ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxConcurrent: 1})
	defer ledger.Close()
	child, cancel := context.WithCancel(ledger.Context())
	cancel()
	if _, err := ledger.AcquireRound(child); !errors.Is(err, context.Canceled) {
		t.Fatalf("child error = %v, want local cancellation", err)
	}
	if cause := ledger.Cause(); cause != nil {
		t.Fatalf("local child cancellation poisoned root ledger: %v", cause)
	}
	release, err := ledger.AcquireRound(ledger.Context())
	if err != nil {
		t.Fatalf("sibling could not acquire after local cancellation: %v", err)
	}
	release()
}

func TestAgentUsesSharedStepAndTokenLedger(t *testing.T) {
	t.Run("steps", func(t *testing.T) {
		prov := &delegationScriptedProvider{turns: [][]provider.Chunk{
			{toolCallChunk("read-1", "read_file", `{}`), {Type: provider.ChunkDone}},
			{{Type: provider.ChunkText, Text: "done"}, {Type: provider.ChunkDone}},
		}}
		reg := tool.NewRegistry()
		reg.Add(fakeTool{name: "read_file", readOnly: true})
		ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxSteps: 1})
		defer ledger.Close()
		agent := New(prov, reg, NewSession("sys"), Options{DelegationLedger: ledger}, event.Discard)
		if err := agent.Run(WithDelegationLedger(ledger.Context(), ledger), "inspect"); !errors.Is(err, ErrDelegationStepBudget) {
			t.Fatalf("Run error = %v, want aggregate step budget", err)
		}
		if got := prov.Calls(); got != 1 {
			t.Fatalf("provider calls = %d, want 1", got)
		}
	})

	t.Run("tokens", func(t *testing.T) {
		prov := &delegationScriptedProvider{turns: [][]provider.Chunk{{
			{Type: provider.ChunkText, Text: "answer"},
			{Type: provider.ChunkUsage, Usage: &provider.Usage{TotalTokens: 11}},
			{Type: provider.ChunkDone},
		}}}
		ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxTokens: 10})
		defer ledger.Close()
		agent := New(prov, tool.NewRegistry(), NewSession("sys"), Options{DelegationLedger: ledger}, event.Discard)
		if err := agent.Run(WithDelegationLedger(ledger.Context(), ledger), "answer"); !errors.Is(err, ErrDelegationTokenBudget) {
			t.Fatalf("Run error = %v, want aggregate token budget", err)
		}
		messages := agent.Session().Snapshot()
		if got := messages[len(messages)-1]; got.Role != provider.RoleAssistant || got.Content != "answer" {
			t.Fatalf("terminal assistant receipt not preserved after token crossing: %+v", got)
		}
	})
}

func TestDelegationLedgerRecordsUsageBeforeStreamFailure(t *testing.T) {
	t.Run("interrupted recovery", func(t *testing.T) {
		prov := &delegationScriptedProvider{turns: [][]provider.Chunk{
			{
				{Type: provider.ChunkText, Text: "partial"},
				{Type: provider.ChunkUsage, Usage: &provider.Usage{TotalTokens: 6}},
				{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: errors.New("connection cut")}},
			},
			{
				{Type: provider.ChunkText, Text: "recovered"},
				{Type: provider.ChunkUsage, Usage: &provider.Usage{TotalTokens: 5}},
				{Type: provider.ChunkDone},
			},
		}}
		ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxTokens: 100})
		defer ledger.Close()
		agent := New(prov, tool.NewRegistry(), NewSession("sys"), Options{DelegationLedger: ledger}, event.Discard)
		if err := agent.Run(ledger.Context(), "answer"); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := ledger.Snapshot().Tokens; got != 11 {
			t.Fatalf("tree tokens = %d, want interrupted+recovered usage 11", got)
		}
	})

	t.Run("interrupted crossing", func(t *testing.T) {
		prov := &delegationScriptedProvider{turns: [][]provider.Chunk{{
			{Type: provider.ChunkText, Text: "partial"},
			{Type: provider.ChunkUsage, Usage: &provider.Usage{TotalTokens: 11}},
			{Type: provider.ChunkError, Err: &provider.StreamInterruptedError{Err: errors.New("connection cut")}},
		}}}
		ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxTokens: 10})
		defer ledger.Close()
		agent := New(prov, tool.NewRegistry(), NewSession("sys"), Options{DelegationLedger: ledger}, event.Discard)
		if err := agent.Run(ledger.Context(), "answer"); !errors.Is(err, ErrDelegationTokenBudget) {
			t.Fatalf("Run error = %v, want token budget", err)
		}
		if got := prov.Calls(); got != 1 {
			t.Fatalf("provider calls = %d, want no recovery after budget crossing", got)
		}
	})

	t.Run("compaction error", func(t *testing.T) {
		prov := &delegationScriptedProvider{turns: [][]provider.Chunk{{
			{Type: provider.ChunkUsage, Usage: &provider.Usage{TotalTokens: 11}},
			{Type: provider.ChunkError, Err: errors.New("summary failed")},
		}}}
		ledger := NewDelegationLedger(context.Background(), DelegationLimits{MaxTokens: 10})
		defer ledger.Close()
		agent := New(prov, tool.NewRegistry(), NewSession("sys"), Options{DelegationLedger: ledger}, event.Discard)
		_, err := agent.summarize(ledger.Context(), []provider.Message{{Role: provider.RoleUser, Content: "old work"}}, "")
		if !errors.Is(err, ErrDelegationTokenBudget) {
			t.Fatalf("summarize error = %v, want token budget", err)
		}
		if got := ledger.Snapshot().Tokens; got != 11 {
			t.Fatalf("compaction tokens = %d, want 11", got)
		}
	})
}

func TestNestedTaskSharesLedgerWithoutHoldingProviderSlot(t *testing.T) {
	prov := &nestedDelegationProvider{}
	reg := tool.NewRegistry()
	task := newTestTaskTool(t, prov, reg, "sys", "", "", nil).
		WithMaxSubagentDepth(2).
		WithDelegationLimits(DelegationLimits{MaxConcurrent: 1, MaxSteps: 2})
	reg.Add(task)

	done := make(chan error, 1)
	go func() {
		_, err := task.Execute(testTaskContext(), []byte(`{"prompt":"outer child"}`))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrDelegationStepBudget) {
			t.Fatalf("Execute error = %v, want shared aggregate step budget", err)
		}
	// Full-suite Windows runs can spend several seconds scheduling this
	// goroutine while many package test binaries build/run in parallel. Keep a
	// bounded deadlock watchdog without turning host load into a false failure.
	case <-time.After(10 * time.Second):
		t.Fatal("nested task deadlocked while max concurrency was one")
	}
	if got := prov.calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want outer+nested rounds only", got)
	}
	if got := prov.peak.Load(); got != 1 {
		t.Fatalf("provider peak = %d, want one active round", got)
	}
}

type delegationScriptedProvider struct {
	mu    sync.Mutex
	turns [][]provider.Chunk
	calls int
}

func (p *delegationScriptedProvider) Name() string { return "delegation-scripted" }

func (p *delegationScriptedProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	i := p.calls
	p.calls++
	if i >= len(p.turns) {
		i = len(p.turns) - 1
	}
	chunks := append([]provider.Chunk(nil), p.turns[i]...)
	p.mu.Unlock()
	ch := make(chan provider.Chunk, len(chunks))
	for _, chunk := range chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func (p *delegationScriptedProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type nestedDelegationProvider struct {
	calls  atomic.Int32
	active atomic.Int32
	peak   atomic.Int32
}

func (p *nestedDelegationProvider) Name() string { return "nested-delegation" }

func (p *nestedDelegationProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.calls.Add(1)
	active := p.active.Add(1)
	for {
		peak := p.peak.Load()
		if active <= peak || p.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	defer p.active.Add(-1)

	ch := make(chan provider.Chunk, 2)
	if last := lastUser(req); containsNestedPrompt(last) {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "nested done"}
		ch <- provider.Chunk{Type: provider.ChunkDone}
	} else {
		ch <- toolCallChunk("nested-task", "task", `{"prompt":"nested child"}`)
		ch <- provider.Chunk{Type: provider.ChunkDone}
	}
	close(ch)
	return ch, nil
}

func containsNestedPrompt(input string) bool {
	return strings.Contains(input, "nested child")
}
