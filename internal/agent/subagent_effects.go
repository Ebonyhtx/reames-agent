package agent

import (
	"context"

	"reames-agent/internal/diff"
	"reames-agent/internal/evidence"
)

type subagentEffectsContextKey struct{}

type subagentEffectLedger struct {
	ledger     *evidence.Ledger
	generation uint64
}

// SubagentEffects is a prompt-invisible bridge from a child Agent to all of its
// ancestor Agents. It carries only structured evidence ledgers and pre-edit
// snapshot callbacks; model text, tool output and child transcripts never cross
// this boundary.
type SubagentEffects struct {
	ledgers      []subagentEffectLedger
	preEditHooks []func(diff.Change)
	parentCallID string
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
	effects.ledgers = appendUniqueLedger(effects.ledgers, a.evidence, a.evidence.Generation())
	if a.subagentPreEditHook != nil {
		if hook := a.subagentPreEditHook(); hook != nil {
			effects.preEditHooks = append(effects.preEditHooks, hook)
		}
	} else if a.onPreEdit != nil {
		effects.preEditHooks = append(effects.preEditHooks, a.onPreEdit)
	}
	if a.subagentEffects != nil {
		for _, target := range a.subagentEffects.ledgers {
			effects.ledgers = appendUniqueLedger(effects.ledgers, target.ledger, target.generation)
		}
		effects.preEditHooks = append(effects.preEditHooks, a.subagentEffects.preEditHooks...)
	}
	if len(effects.ledgers) == 0 && len(effects.preEditHooks) == 0 {
		return nil
	}
	return effects
}

func appendUniqueLedger(dst []subagentEffectLedger, ledger *evidence.Ledger, generation uint64) []subagentEffectLedger {
	if ledger == nil {
		return dst
	}
	for _, existing := range dst {
		if existing.ledger == ledger {
			return dst
		}
	}
	return append(dst, subagentEffectLedger{ledger: ledger, generation: generation})
}

func (e *SubagentEffects) snapshot(change diff.Change) {
	if e == nil {
		return
	}
	for _, hook := range e.preEditHooks {
		if hook != nil {
			hook(change)
		}
	}
}

func (e *SubagentEffects) record(receipt evidence.Receipt, depth int) {
	if e == nil || (!receipt.Read && !receipt.Write && receipt.Command == "") {
		return
	}
	receipt.Source = "subagent"
	receipt.ParentToolCallID = e.parentCallID
	receipt.SubagentDepth = depth
	for _, target := range e.ledgers {
		target.ledger.RecordAtGeneration(receipt, target.generation)
	}
}
