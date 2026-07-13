package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"reames-agent/internal/provider"
)

var (
	ErrDelegationStepBudget  = errors.New("delegation step budget exhausted")
	ErrDelegationTokenBudget = errors.New("delegation token budget exhausted")
	ErrDelegationTimeBudget  = errors.New("delegation time budget exhausted")
)

// DelegationLimits bounds one complete subagent tree. MaxConcurrent limits
// active provider rounds, not child lifetimes, so a parent can wait for a nested
// child without occupying a slot. Non-positive limits are unbounded.
type DelegationLimits struct {
	MaxConcurrent int
	MaxSteps      int
	MaxTokens     int64
	MaxDuration   time.Duration
}

// DelegationSnapshot is a point-in-time, prompt-invisible resource projection.
type DelegationSnapshot struct {
	MaxConcurrent int
	ActiveRounds  int
	PeakRounds    int
	MaxSteps      int
	Steps         int
	MaxTokens     int64
	Tokens        int64
	Deadline      time.Time
	Cause         string
}

type delegationLedgerContextKey struct{}

// DelegationLedger atomically accounts provider concurrency, rounds, tokens,
// deadline and root cancellation across every descendant in a delegation tree.
type DelegationLedger struct {
	limits DelegationLimits
	ctx    context.Context
	cancel context.CancelCauseFunc
	stop   context.CancelFunc
	sem    chan struct{}

	mu     sync.Mutex
	steps  int
	tokens int64
	active int
	peak   int
	cause  error

	closeOnce sync.Once
	deadline  time.Time
}

// NewDelegationLedger creates a tree root derived from parent. Parent
// cancellation stops the whole tree; cancelling an independently-derived child
// context does not cancel its siblings.
func NewDelegationLedger(parent context.Context, limits DelegationLimits) *DelegationLedger {
	if parent == nil {
		parent = context.Background()
	}
	if limits.MaxConcurrent < 0 {
		limits.MaxConcurrent = 0
	}
	if limits.MaxSteps < 0 {
		limits.MaxSteps = 0
	}
	if limits.MaxTokens < 0 {
		limits.MaxTokens = 0
	}
	if limits.MaxDuration < 0 {
		limits.MaxDuration = 0
	}

	base := parent
	stop := func() {}
	var deadline time.Time
	if limits.MaxDuration > 0 {
		deadline = time.Now().Add(limits.MaxDuration)
		base, stop = context.WithDeadlineCause(parent, deadline, ErrDelegationTimeBudget)
	}
	ctx, cancel := context.WithCancelCause(base)
	l := &DelegationLedger{
		limits:   limits,
		ctx:      ctx,
		cancel:   cancel,
		stop:     stop,
		deadline: deadline,
	}
	if limits.MaxConcurrent > 0 {
		l.sem = make(chan struct{}, limits.MaxConcurrent)
	}
	return l
}

// EnsureDelegationLedger reuses an ancestor ledger or creates a new tree root.
// The cleanup function owns only a newly-created root and is otherwise a no-op.
func EnsureDelegationLedger(ctx context.Context, limits DelegationLimits) (context.Context, *DelegationLedger, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ledger, ok := DelegationLedgerFromContext(ctx); ok {
		return ctx, ledger, func() {}
	}
	ledger := NewDelegationLedger(ctx, limits)
	root := context.WithValue(ledger.Context(), delegationLedgerContextKey{}, ledger)
	return root, ledger, ledger.Close
}

// WithDelegationLedger propagates a ledger through an Agent's tool-call context.
func WithDelegationLedger(ctx context.Context, ledger *DelegationLedger) context.Context {
	if ledger == nil {
		return ctx
	}
	return context.WithValue(ctx, delegationLedgerContextKey{}, ledger)
}

// DelegationLedgerFromContext returns the tree ledger carried by ctx.
func DelegationLedgerFromContext(ctx context.Context) (*DelegationLedger, bool) {
	if ctx == nil {
		return nil, false
	}
	ledger, ok := ctx.Value(delegationLedgerContextKey{}).(*DelegationLedger)
	return ledger, ok && ledger != nil
}

func (l *DelegationLedger) Context() context.Context {
	if l == nil || l.ctx == nil {
		return context.Background()
	}
	return l.ctx
}

// Close releases deadline resources and cancels a completed tree. It is safe to
// call more than once.
func (l *DelegationLedger) Close() {
	if l == nil {
		return
	}
	l.closeOnce.Do(func() {
		l.cancel(nil)
		l.stop()
	})
}

// AcquireRound reserves one aggregate step and one provider concurrency slot.
// The returned release function must be called when the provider stream closes.
func (l *DelegationLedger) AcquireRound(ctx context.Context) (func(), error) {
	if l == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = l.ctx
	}
	if err := l.contextError(ctx); err != nil {
		return nil, err
	}
	if l.sem != nil {
		select {
		case l.sem <- struct{}{}:
		case <-ctx.Done():
			return nil, l.contextError(ctx)
		case <-l.ctx.Done():
			return nil, l.Cause()
		}
	}
	releaseSlot := func() {
		if l.sem != nil {
			<-l.sem
		}
	}

	l.mu.Lock()
	if err := context.Cause(l.ctx); err != nil {
		l.mu.Unlock()
		releaseSlot()
		return nil, err
	}
	if l.limits.MaxSteps > 0 && l.steps >= l.limits.MaxSteps {
		l.mu.Unlock()
		releaseSlot()
		return nil, l.trip(ErrDelegationStepBudget)
	}
	if l.limits.MaxTokens > 0 && l.tokens >= l.limits.MaxTokens {
		l.mu.Unlock()
		releaseSlot()
		return nil, l.trip(ErrDelegationTokenBudget)
	}
	l.steps++
	l.active++
	if l.active > l.peak {
		l.peak = l.active
	}
	l.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.active > 0 {
				l.active--
			}
			l.mu.Unlock()
			releaseSlot()
		})
	}, nil
}

// RecordUsage adds one provider receipt to the shared token total. Providers
// report usage after a response, so one response may cross the configured cap;
// that crossing atomically cancels every still-running descendant.
func (l *DelegationLedger) RecordUsage(usage *provider.Usage) error {
	if l == nil || usage == nil || usage.TotalTokens <= 0 {
		return nil
	}
	l.mu.Lock()
	l.tokens += int64(usage.TotalTokens)
	exceeded := l.limits.MaxTokens > 0 && l.tokens > l.limits.MaxTokens
	l.mu.Unlock()
	if exceeded {
		return l.trip(ErrDelegationTokenBudget)
	}
	return nil
}

// Cause returns the first root cancellation cause, if any.
func (l *DelegationLedger) Cause() error {
	if l == nil {
		return nil
	}
	if err := context.Cause(l.ctx); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cause
}

func (l *DelegationLedger) Snapshot() DelegationSnapshot {
	if l == nil {
		return DelegationSnapshot{}
	}
	l.mu.Lock()
	snapshot := DelegationSnapshot{
		MaxConcurrent: l.limits.MaxConcurrent,
		ActiveRounds:  l.active,
		PeakRounds:    l.peak,
		MaxSteps:      l.limits.MaxSteps,
		Steps:         l.steps,
		MaxTokens:     l.limits.MaxTokens,
		Tokens:        l.tokens,
		Deadline:      l.deadline,
	}
	l.mu.Unlock()
	if cause := l.Cause(); cause != nil {
		snapshot.Cause = cause.Error()
	}
	return snapshot
}

func (l *DelegationLedger) trip(cause error) error {
	if cause == nil {
		cause = context.Canceled
	}
	if existing := context.Cause(l.ctx); existing != nil {
		return existing
	}
	l.mu.Lock()
	if l.cause == nil {
		l.cause = cause
	}
	first := l.cause
	l.mu.Unlock()
	l.cancel(first)
	return first
}

func (l *DelegationLedger) contextError(ctx context.Context) error {
	if cause := context.Cause(l.ctx); cause != nil {
		return cause
	}
	if err := ctx.Err(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return err
	}
	return nil
}

func (a *Agent) acquireDelegationRound(ctx context.Context) (func(), error) {
	if a == nil || a.delegation == nil {
		return func() {}, nil
	}
	release, err := a.delegation.AcquireRound(ctx)
	if err != nil {
		return nil, a.delegation.budgetError(err)
	}
	return release, nil
}

func (l *DelegationLedger) budgetError(kind error) error {
	snapshot := l.Snapshot()
	switch {
	case errors.Is(kind, ErrDelegationStepBudget):
		return fmt.Errorf("%w (steps=%d max_steps=%d)", kind, snapshot.Steps, snapshot.MaxSteps)
	case errors.Is(kind, ErrDelegationTokenBudget):
		return fmt.Errorf("%w (tokens=%d max_tokens=%d)", kind, snapshot.Tokens, snapshot.MaxTokens)
	case errors.Is(kind, ErrDelegationTimeBudget):
		return fmt.Errorf("%w (deadline=%s)", kind, snapshot.Deadline.Format(time.RFC3339Nano))
	default:
		return kind
	}
}
