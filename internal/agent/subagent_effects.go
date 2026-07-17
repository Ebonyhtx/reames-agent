package agent

import (
	"context"
	"errors"

	"reames-agent/internal/diff"
	"reames-agent/internal/evidence"
)

type subagentEffectsContextKey struct{}

type subagentEffectLedger struct {
	ledger       *evidence.Ledger
	generation   uint64
	parentCallID string
	journal      *subagentEffectJournalTarget
	observer     func(evidence.SubagentEffectCursor, evidence.Receipt) error
}

// SubagentEffects is a prompt-invisible bridge from a child Agent to all of its
// ancestor Agents. It carries only structured evidence ledgers and pre-edit
// snapshot callbacks; model text, tool output and child transcripts never cross
// this boundary.
type SubagentEffects struct {
	ledgers      []subagentEffectLedger
	preEditHooks []PreEditHook
	parentCallID string
	isolated     bool
}

// WithSubagentEffects carries an immutable ancestor bridge into a child runner.
func WithSubagentEffects(ctx context.Context, effects *SubagentEffects) context.Context {
	if effects == nil {
		return ctx
	}
	return context.WithValue(ctx, subagentEffectsContextKey{}, effects)
}

// SubagentEffectsFromContext returns the bridge prepared by the invoking Agent.
func SubagentEffectsFromContext(ctx context.Context) (*SubagentEffects, bool) {
	if ctx == nil {
		return nil, false
	}
	effects, ok := ctx.Value(subagentEffectsContextKey{}).(*SubagentEffects)
	return effects, ok && effects != nil
}

func (a *Agent) effectsForChild(parentCallID string) *SubagentEffects {
	if a == nil {
		return nil
	}
	effects := &SubagentEffects{parentCallID: parentCallID}
	effects.ledgers = appendUniqueLedger(effects.ledgers, subagentEffectLedger{
		ledger:       a.evidence,
		generation:   a.evidence.Generation(),
		parentCallID: parentCallID,
		observer:     a.observeDurableSubagentEffect,
	})
	if a.subagentPreEditHook != nil {
		if hook := a.subagentPreEditHook(); hook != nil {
			effects.preEditHooks = append(effects.preEditHooks, hook)
		}
	} else if a.onPreEdit != nil {
		effects.preEditHooks = append(effects.preEditHooks, a.onPreEdit)
	}
	if a.subagentEffects != nil {
		for _, target := range a.subagentEffects.ledgers {
			effects.ledgers = appendUniqueLedger(effects.ledgers, target)
		}
		effects.preEditHooks = append(effects.preEditHooks, a.subagentEffects.preEditHooks...)
	}
	if len(effects.ledgers) == 0 && len(effects.preEditHooks) == 0 {
		return nil
	}
	return effects
}

func appendUniqueLedger(dst []subagentEffectLedger, target subagentEffectLedger) []subagentEffectLedger {
	if target.ledger == nil {
		return dst
	}
	for _, existing := range dst {
		if existing.ledger == target.ledger {
			return dst
		}
	}
	return append(dst, target)
}

// withJournal returns a copy whose outermost ancestor target persists every
// descendant effect under the top-level subagent ref. Nested task calls inherit
// that target instead of creating journals whose anchors exist only in a child
// transcript and cannot be validated by the root session on recovery.
func (e *SubagentEffects) withJournal(run *SubagentRun) (*SubagentEffects, error) {
	if e == nil || run == nil || run.Ref == "" || run.store == nil {
		return e, nil
	}
	clone := &SubagentEffects{
		ledgers:      append([]subagentEffectLedger(nil), e.ledgers...),
		preEditHooks: append([]PreEditHook(nil), e.preEditHooks...),
		parentCallID: e.parentCallID,
		isolated:     e.isolated,
	}
	for _, target := range clone.ledgers {
		if target.journal != nil {
			return clone, nil
		}
	}
	if len(clone.ledgers) == 0 {
		return clone, nil
	}
	index := 0
	for i := range clone.ledgers {
		callID := clone.ledgers[i].parentCallID
		if callID == "" {
			callID = clone.parentCallID
		}
		if callID == run.Meta.ParentToolCallID {
			index = i
			break
		}
	}
	callID := clone.ledgers[index].parentCallID
	if callID == "" {
		callID = clone.parentCallID
	}
	target, err := run.store.newSubagentEffectJournalTarget(run, callID)
	if err != nil {
		return nil, err
	}
	clone.ledgers[index].journal = target
	return clone, nil
}

// isolatedWorkspaceEffects keeps the durable child audit journal but suppresses
// ancestor checkpoints and evidence forwarding while mutations remain inside a
// delivery worktree. The later apply/merge tool records the real source change.
func (e *SubagentEffects) isolatedWorkspaceEffects() *SubagentEffects {
	if e == nil {
		return nil
	}
	clone := &SubagentEffects{
		ledgers:      append([]subagentEffectLedger(nil), e.ledgers...),
		parentCallID: e.parentCallID,
		isolated:     true,
	}
	for i := range clone.ledgers {
		clone.ledgers[i].observer = nil
	}
	return clone
}

// IsolateSubagentEffects keeps only the durable audit journal side of a child
// effects bridge while the child operates in a separate worktree.
func IsolateSubagentEffects(e *SubagentEffects) *SubagentEffects {
	return e.isolatedWorkspaceEffects()
}

// BindSubagentEffectJournal attaches the outermost persisted subagent ref to an
// ancestor effects bridge. Task and writer-capable skill runners share this
// boundary; read-only and ephemeral subagents do not create a journal.
func BindSubagentEffectJournal(effects *SubagentEffects, run *SubagentRun) (*SubagentEffects, error) {
	if effects == nil {
		return nil, nil
	}
	return effects.withJournal(run)
}

func (e *SubagentEffects) snapshot(change diff.Change) error {
	if e == nil || e.isolated {
		return nil
	}
	var snapshotErr error
	for _, hook := range e.preEditHooks {
		if hook != nil {
			snapshotErr = errors.Join(snapshotErr, hook(change))
		}
	}
	return snapshotErr
}

func (e *SubagentEffects) prepare(receipt evidence.Receipt, depth int) error {
	if e == nil || !receipt.Write || !receipt.MutationAttempt {
		return nil
	}
	for _, target := range e.ledgers {
		if target.journal == nil {
			continue
		}
		event, err := target.journal.append(subagentEffectPhaseIntent, receipt, depth)
		if err != nil {
			return err
		}
		if target.observer != nil {
			if err := target.observer(event.Cursor, event.Receipt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *SubagentEffects) record(receipt evidence.Receipt, depth int) error {
	if e == nil || (!receipt.Read && !receipt.Write && receipt.Command == "") {
		return nil
	}
	for _, target := range e.ledgers {
		if target.journal == nil {
			continue
		}
		event, err := target.journal.append(subagentEffectPhaseReceipt, receipt, depth)
		if err != nil {
			return err
		}
		if target.observer != nil {
			if err := target.observer(event.Cursor, event.Receipt); err != nil {
				return err
			}
		}
	}
	if e.isolated {
		return nil
	}
	for _, target := range e.ledgers {
		forwarded := receipt
		forwarded.Source = "subagent"
		forwarded.ParentToolCallID = target.parentCallID
		if forwarded.ParentToolCallID == "" {
			forwarded.ParentToolCallID = e.parentCallID
		}
		forwarded.SubagentDepth = depth
		target.ledger.RecordAtGeneration(forwarded, target.generation)
	}
	return nil
}
