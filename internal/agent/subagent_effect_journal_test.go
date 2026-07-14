package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/event"
	"reames-agent/internal/evidence"
	"reames-agent/internal/instruction"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestSubagentEffectJournalCrashRecoveryInvalidatesOldRootChecks(t *testing.T) {
	workspace := t.TempDir()
	check := instruction.VerifyCheck{Command: "go test ./..."}
	beforeCrash := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ProjectChecks: []instruction.VerifyCheck{check}}, event.Discard)
	beforeCrash.evidence.Record(evidence.Receipt{ToolName: "write_file", ToolCallID: "root-write", Success: true, Write: true})
	beforeCrash.evidence.Record(evidence.Receipt{ToolName: "bash", ToolCallID: "root-check", Success: true, Command: check.Command})
	oldState := beforeCrash.DurableEvidenceState()
	if !oldState.WritePending || len(oldState.VerifiedChecks) != 1 {
		t.Fatalf("old root state = %+v", oldState)
	}

	store, run, effects := prepareEffectJournal(t, workspace, beforeCrash, "task-root")
	intent := evidence.Receipt{ToolName: "write_file", ToolCallID: "child-write", Write: true, MutationAttempt: true, Paths: []string{"child.go"}}
	if err := effects.prepare(intent, 1); err != nil {
		t.Fatalf("prepare child intent: %v", err)
	}
	run.Session.Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "child-write", Name: "write_file", Arguments: `{}`}}})
	if err := store.SaveRunning(run); err != nil {
		t.Fatalf("persist child tool envelope: %v", err)
	}
	run.Release()

	parentMessages := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "task-root", Name: "task", Arguments: `{}`}}}}
	recovered, err := store.RecoverSubagentEffects("parent-session", workspace, parentMessages, nil)
	if err != nil {
		t.Fatalf("RecoverSubagentEffects: %v", err)
	}
	if len(recovered) != 1 || !recovered[0].Receipt.MutationAttempt || !recovered[0].Receipt.Write {
		t.Fatalf("recovered effects = %+v", recovered)
	}

	afterCrash := New(nil, tool.NewRegistry(), NewSession("sys"), Options{ProjectChecks: []instruction.VerifyCheck{check}}, event.Discard)
	afterCrash.durableEvidence = oldState.Clone()
	if err := afterCrash.ApplyRecoveredSubagentEffects(recovered); err != nil {
		t.Fatalf("ApplyRecoveredSubagentEffects: %v", err)
	}
	state := afterCrash.DurableEvidenceState()
	if !state.WritePending || len(state.VerifiedChecks) != 0 || len(state.SubagentEffects) != 1 {
		t.Fatalf("recovered root state = %+v", state)
	}

	// A root check after recovery may satisfy readiness. Replaying the same
	// journal cursor must not invalidate that newer check again.
	afterCrash.evidence.Record(evidence.Receipt{ToolName: "bash", ToolCallID: "root-check-after-recovery", Success: true, Command: check.Command})
	if got := afterCrash.DurableEvidenceState(); len(got.VerifiedChecks) != 1 {
		t.Fatalf("post-recovery root check = %+v", got)
	}
	if err := afterCrash.ApplyRecoveredSubagentEffects(recovered); err != nil {
		t.Fatalf("replay recovered effects: %v", err)
	}
	if got := afterCrash.DurableEvidenceState(); len(got.VerifiedChecks) != 1 {
		t.Fatalf("duplicate replay invalidated newer root check: %+v", got)
	}
	acknowledged := afterCrash.DurableEvidenceState().SubagentEffects
	if got, err := store.RecoverSubagentEffects("parent-session", workspace, nil, acknowledged); err != nil || len(got) != 0 {
		t.Fatalf("acknowledged effects after parent compaction = %+v, %v", got, err)
	}
}

func TestSubagentEffectJournalDeduplicatesAndRejectsConflictingReplay(t *testing.T) {
	workspace := t.TempDir()
	parent := New(nil, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	store, run, effects := prepareEffectJournal(t, workspace, parent, "task-dedupe")
	defer run.Release()
	receipt := evidence.Receipt{ToolName: "write_file", ToolCallID: "child-write", Write: true, MutationAttempt: true, Paths: []string{"a.go"}}
	receipt.Args = []byte(`{"content":"must-not-enter-journal"}`)
	if err := effects.prepare(receipt, 1); err != nil {
		t.Fatal(err)
	}
	if err := effects.prepare(receipt, 1); err != nil {
		t.Fatalf("idempotent prepare replay: %v", err)
	}
	journal, err := store.loadEffectJournal(run.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if journal.NextSequence != 1 || len(journal.Events) != 1 {
		t.Fatalf("deduplicated journal = %+v", journal)
	}
	secret := "sk-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"
	if err := effects.record(evidence.Receipt{ToolName: "bash", ToolCallID: "child-command", Success: true, Command: "curl -H " + secret}, 1); err != nil {
		t.Fatalf("record command receipt: %v", err)
	}
	if data, err := os.ReadFile(store.effectPath(run.Ref)); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(data), "must-not-enter-journal") || strings.Contains(string(data), secret) || !strings.Contains(string(data), "[REDACTED:OpenAI]") {
		t.Fatalf("journal exposed arguments/secrets or omitted redaction: %s", data)
	}
	conflict := receipt
	conflict.Paths = []string{"different.go"}
	if err := effects.prepare(conflict, 1); err == nil || !strings.Contains(err.Error(), "conflicting replay") {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestSubagentEffectJournalCompactionIsBoundedAndConservative(t *testing.T) {
	journal := subagentEffectJournal{NextSequence: maxSubagentEffectEvents + 3}
	for i := 1; i <= maxSubagentEffectEvents+3; i++ {
		journal.Events = append(journal.Events, subagentEffectEvent{
			Sequence: uint64(i),
			Phase:    subagentEffectPhaseReceipt,
			Receipt: durableSubagentReceipt{
				ToolName: "read_file", ToolCallID: "call", Read: true, SubagentDepth: 1,
			},
		})
	}
	journal.Events[0].Receipt.Write = true
	journal.Events[0].Receipt.MutationAttempt = true
	compactSubagentEffectJournal(&journal)
	if len(journal.Events) != maxSubagentEffectEvents || journal.CompactedThrough != 3 || !journal.CompactedMutation {
		t.Fatalf("compacted journal = %+v", journal)
	}
}

func TestSubagentEffectJournalPersistenceFailureBlocksChildWriter(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "blocked.txt")
	parent := New(nil, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	store, run, effects := prepareEffectJournal(t, workspace, parent, "task-blocked")
	defer run.Release()
	store.effectWrite = func(string, []byte, os.FileMode) error {
		return errors.New("injected journal persistence failure")
	}
	reg := tool.NewRegistry()
	reg.Add(&effectsWriter{})
	child := New(nil, reg, NewSession("child"), Options{SubagentDepth: 1, SubagentEffects: effects}, event.Discard)
	outcome := child.executeOne(testTaskContext(), provider.ToolCall{
		ID: "child-blocked", Name: "write_file", Arguments: mustEffectWriterArgs(t, path, "must not land\n"),
	})
	if !outcome.blocked || !strings.Contains(outcome.output, "injected journal persistence failure") {
		t.Fatalf("writer outcome = %+v", outcome)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("writer touched disk after journal failure: %v", err)
	}
}

func TestWriterSkillSubagentEffectJournalUsesRunSkillAnchor(t *testing.T) {
	workspace := t.TempDir()
	parent := New(nil, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	store, run, effects := prepareEffectJournalKind(t, workspace, parent, "run-skill", "skill", "format")
	defer run.Release()
	if err := effects.prepare(evidence.Receipt{ToolName: "write_file", ToolCallID: "skill-write", Write: true, MutationAttempt: true}, 1); err != nil {
		t.Fatal(err)
	}
	messages := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "run-skill", Name: "run_skill"}}}}
	if recovered, err := store.RecoverSubagentEffects("parent-session", workspace, messages, nil); err != nil || len(recovered) != 1 {
		t.Fatalf("writer skill recovery = %+v, %v", recovered, err)
	}
}

func TestSubagentEffectRecoveryRejectsCorruptStaleAndBranchCrossingJournals(t *testing.T) {
	workspace := t.TempDir()
	parent := New(nil, tool.NewRegistry(), NewSession("sys"), Options{}, event.Discard)
	store, run, effects := prepareEffectJournal(t, workspace, parent, "task-owned")
	receipt := evidence.Receipt{ToolName: "write_file", ToolCallID: "child-write", Write: true, MutationAttempt: true}
	if err := effects.prepare(receipt, 1); err != nil {
		t.Fatal(err)
	}
	ref := run.Ref
	run.Release()
	validMeta, err := store.LoadMeta(ref)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("stale parent tool anchor", func(t *testing.T) {
		messages := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "task-other", Name: "task"}}}}
		if _, err := store.RecoverSubagentEffects("parent-session", workspace, messages, nil); err == nil || !strings.Contains(err.Error(), "not anchored") {
			t.Fatalf("stale anchor error = %v", err)
		}
	})

	t.Run("branch crossing", func(t *testing.T) {
		meta := validMeta
		meta.ParentSession = "other-session"
		if err := store.saveMeta(meta); err != nil {
			t.Fatal(err)
		}
		messages := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "task-owned", Name: "task"}}}}
		if _, err := store.RecoverSubagentEffects("other-session", workspace, messages, nil); err == nil || !strings.Contains(err.Error(), "ownership") {
			t.Fatalf("branch crossing error = %v", err)
		}
		if err := store.saveMeta(validMeta); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("corrupt owner metadata", func(t *testing.T) {
		if err := os.WriteFile(store.metaPath(ref), []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		messages := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "task-owned", Name: "task"}}}}
		if _, err := store.RecoverSubagentEffects("parent-session", workspace, messages, nil); err == nil || !strings.Contains(err.Error(), "validate owner") {
			t.Fatalf("corrupt owner error = %v", err)
		}
		if err := store.saveMeta(validMeta); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("corrupt journal", func(t *testing.T) {
		if err := os.WriteFile(store.effectPath(ref), []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
		messages := []provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "task-owned", Name: "task"}}}}
		if _, err := store.RecoverSubagentEffects("parent-session", workspace, messages, nil); err == nil || !strings.Contains(err.Error(), "decode") {
			t.Fatalf("corrupt journal error = %v", err)
		}
	})
}

func prepareEffectJournal(t *testing.T, workspace string, parent *Agent, parentCallID string) (*SubagentStore, *SubagentRun, *SubagentEffects) {
	return prepareEffectJournalKind(t, workspace, parent, parentCallID, "task", "task")
}

func prepareEffectJournalKind(t *testing.T, workspace string, parent *Agent, parentCallID, kind, name string) (*SubagentStore, *SubagentRun, *SubagentEffects) {
	t.Helper()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	spec := SubagentSpec{
		Kind:             kind,
		Name:             name,
		WorkspaceRoot:    workspace,
		ParentSession:    "parent-session",
		ParentToolCallID: parentCallID,
		SystemPrompt:     "child sys",
		Registry:         tool.NewRegistry(),
		Model:            "child-model",
	}
	run, err := store.PrepareFresh(spec)
	if err != nil {
		t.Fatalf("PrepareFresh: %v", err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatalf("MarkRunning: %v", err)
	}
	effects, err := parent.effectsForChild(parentCallID).withJournal(run)
	if err != nil {
		run.Release()
		t.Fatalf("withJournal: %v", err)
	}
	return store, run, effects
}
