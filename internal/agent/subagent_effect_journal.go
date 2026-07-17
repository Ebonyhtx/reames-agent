package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"reames-agent/internal/evidence"
	"reames-agent/internal/fileutil"
	"reames-agent/internal/provider"
	"reames-agent/internal/trust"
)

const (
	subagentEffectJournalVersion  = 1
	maxSubagentEffectEvents       = 256
	maxSubagentEffectJournalBytes = 1 << 20
	subagentEffectPhaseIntent     = "intent"
	subagentEffectPhaseReceipt    = "receipt"
)

type subagentEffectJournal struct {
	Version           int                   `json:"version"`
	Ref               string                `json:"ref"`
	JournalID         string                `json:"journalID"`
	WorkspaceRoot     string                `json:"workspaceRoot"`
	ParentSession     string                `json:"parentSession"`
	NextSequence      uint64                `json:"nextSequence"`
	CompactedThrough  uint64                `json:"compactedThrough,omitempty"`
	CompactedMutation bool                  `json:"compactedMutation,omitempty"`
	Events            []subagentEffectEvent `json:"events,omitempty"`
}

type subagentEffectEvent struct {
	Sequence         uint64                 `json:"sequence"`
	Phase            string                 `json:"phase"`
	ParentToolCallID string                 `json:"parentToolCallID"`
	Receipt          durableSubagentReceipt `json:"receipt"`
}

// durableSubagentReceipt deliberately excludes args, model text, tool output,
// Todo/step bodies and provider messages. The sidecar carries only the minimum
// structured effect metadata needed for recovery and audit.
type durableSubagentReceipt struct {
	ToolName        string   `json:"toolName"`
	ToolCallID      string   `json:"toolCallID"`
	Success         bool     `json:"success"`
	Command         string   `json:"command,omitempty"`
	Paths           []string `json:"paths,omitempty"`
	Read            bool     `json:"read,omitempty"`
	Write           bool     `json:"write,omitempty"`
	MutationAttempt bool     `json:"mutationAttempt,omitempty"`
	SubagentDepth   int      `json:"subagentDepth"`
}

type subagentEffectJournalTarget struct {
	store            *SubagentStore
	ref              string
	journalID        string
	workspaceRoot    string
	parentSession    string
	parentToolCallID string
}

// RecoveredSubagentEffect is a prompt-invisible journal event validated against
// the owning parent session and its top-level task call.
type RecoveredSubagentEffect struct {
	Cursor  evidence.SubagentEffectCursor
	Receipt evidence.Receipt
}

func (s *SubagentStore) newSubagentEffectJournalTarget(run *SubagentRun, parentCallID string) (*subagentEffectJournalTarget, error) {
	if s == nil || run == nil || run.Ref == "" {
		return nil, nil
	}
	parentCallID = strings.TrimSpace(parentCallID)
	if parentCallID == "" || strings.TrimSpace(run.Meta.ParentSession) == "" {
		return nil, fmt.Errorf("subagent effect journal requires parent session and tool-call anchors")
	}
	s.effectsMu.Lock()
	defer s.effectsMu.Unlock()
	journal, err := s.loadOrCreateEffectJournal(run)
	if err != nil {
		return nil, err
	}
	return &subagentEffectJournalTarget{
		store:            s,
		ref:              run.Ref,
		journalID:        journal.JournalID,
		workspaceRoot:    journal.WorkspaceRoot,
		parentSession:    journal.ParentSession,
		parentToolCallID: parentCallID,
	}, nil
}

func (t *subagentEffectJournalTarget) append(phase string, receipt evidence.Receipt, depth int) (RecoveredSubagentEffect, error) {
	if t == nil || t.store == nil {
		return RecoveredSubagentEffect{}, nil
	}
	if t.store.destroyed != nil && t.store.destroyed(t.parentSession) {
		return RecoveredSubagentEffect{}, fmt.Errorf("parent session %q was destroyed", t.parentSession)
	}
	t.store.effectsMu.Lock()
	defer t.store.effectsMu.Unlock()
	journal, err := t.store.loadEffectJournal(t.ref)
	if err != nil {
		return RecoveredSubagentEffect{}, err
	}
	if journal.JournalID != t.journalID || journal.WorkspaceRoot != t.workspaceRoot || journal.ParentSession != t.parentSession {
		return RecoveredSubagentEffect{}, fmt.Errorf("subagent effect journal %q identity changed", t.ref)
	}
	durable, err := makeDurableSubagentReceipt(receipt, depth)
	if err != nil {
		return RecoveredSubagentEffect{}, err
	}
	event := subagentEffectEvent{
		Phase:            phase,
		ParentToolCallID: t.parentToolCallID,
		Receipt:          durable,
	}
	for _, current := range journal.Events {
		if current.Phase != event.Phase || current.ParentToolCallID != event.ParentToolCallID || current.Receipt.ToolCallID != event.Receipt.ToolCallID {
			continue
		}
		if !reflect.DeepEqual(current.Receipt, event.Receipt) {
			return RecoveredSubagentEffect{}, fmt.Errorf("conflicting replay for subagent effect %q/%s/%s", t.ref, phase, durable.ToolCallID)
		}
		return recoveredEffect(journal, current), nil
	}
	event.Sequence = journal.NextSequence + 1
	journal.NextSequence = event.Sequence
	journal.Events = append(journal.Events, event)
	compactSubagentEffectJournal(&journal)
	if err := t.store.saveEffectJournal(journal); err != nil {
		return RecoveredSubagentEffect{}, err
	}
	return recoveredEffect(journal, event), nil
}

func makeDurableSubagentReceipt(receipt evidence.Receipt, depth int) (durableSubagentReceipt, error) {
	toolName := strings.TrimSpace(receipt.ToolName)
	toolCallID := strings.TrimSpace(receipt.ToolCallID)
	if toolName == "" || toolCallID == "" {
		return durableSubagentReceipt{}, fmt.Errorf("subagent effect receipt requires tool name and call ID")
	}
	if len(toolName) > 256 || len(toolCallID) > 512 {
		return durableSubagentReceipt{}, fmt.Errorf("subagent effect receipt identity is too long")
	}
	if depth < 1 {
		return durableSubagentReceipt{}, fmt.Errorf("subagent effect receipt has invalid depth %d", depth)
	}
	command := strings.TrimSpace(receipt.Command)
	if len(command) > 8*1024 {
		return durableSubagentReceipt{}, fmt.Errorf("subagent command receipt exceeds 8 KiB")
	}
	command = trust.RedactSecrets(command)
	paths, err := boundedEffectPaths(receipt.Paths)
	if err != nil {
		return durableSubagentReceipt{}, err
	}
	return durableSubagentReceipt{
		ToolName:        toolName,
		ToolCallID:      toolCallID,
		Success:         receipt.Success,
		Command:         command,
		Paths:           paths,
		Read:            receipt.Read,
		Write:           receipt.Write,
		MutationAttempt: receipt.MutationAttempt,
		SubagentDepth:   depth,
	}, nil
}

func boundedEffectPaths(paths []string) ([]string, error) {
	if len(paths) > 256 {
		return nil, fmt.Errorf("subagent effect receipt has too many paths: %d", len(paths))
	}
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	total := 0
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		if len(path) > 4096 {
			return nil, fmt.Errorf("subagent effect path exceeds 4096 bytes")
		}
		total += len(path)
		if total > 32*1024 {
			return nil, fmt.Errorf("subagent effect paths exceed 32 KiB")
		}
		seen[path] = true
		out = append(out, path)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func compactSubagentEffectJournal(journal *subagentEffectJournal) {
	if journal == nil || len(journal.Events) <= maxSubagentEffectEvents {
		return
	}
	drop := len(journal.Events) - maxSubagentEffectEvents
	for _, event := range journal.Events[:drop] {
		journal.CompactedThrough = event.Sequence
		if event.Receipt.Write && (event.Receipt.Success || event.Receipt.MutationAttempt) {
			journal.CompactedMutation = true
		}
	}
	journal.Events = append([]subagentEffectEvent(nil), journal.Events[drop:]...)
}

func recoveredEffect(journal subagentEffectJournal, event subagentEffectEvent) RecoveredSubagentEffect {
	receipt := event.Receipt.toReceipt(event.ParentToolCallID)
	return RecoveredSubagentEffect{
		Cursor:  evidence.SubagentEffectCursor{Ref: journal.Ref, JournalID: journal.JournalID, Sequence: event.Sequence},
		Receipt: receipt,
	}
}

func (r durableSubagentReceipt) toReceipt(parentCallID string) evidence.Receipt {
	return evidence.Receipt{
		ToolName:         r.ToolName,
		ToolCallID:       r.ToolCallID,
		Success:          r.Success,
		Command:          r.Command,
		Paths:            append([]string(nil), r.Paths...),
		Read:             r.Read,
		Write:            r.Write,
		MutationAttempt:  r.MutationAttempt,
		Source:           "subagent",
		ParentToolCallID: parentCallID,
		SubagentDepth:    r.SubagentDepth,
	}
}

func (s *SubagentStore) loadOrCreateEffectJournal(run *SubagentRun) (subagentEffectJournal, error) {
	journal, err := s.loadEffectJournal(run.Ref)
	if err == nil {
		if journal.WorkspaceRoot != strings.TrimSpace(run.Meta.WorkspaceRoot) || journal.ParentSession != strings.TrimSpace(run.Meta.ParentSession) {
			return subagentEffectJournal{}, fmt.Errorf("subagent effect journal %q belongs to a different workspace or parent session", run.Ref)
		}
		return journal, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return subagentEffectJournal{}, err
	}
	journalID, err := newSubagentEffectJournalID()
	if err != nil {
		return subagentEffectJournal{}, err
	}
	journal = subagentEffectJournal{
		Version:       subagentEffectJournalVersion,
		Ref:           run.Ref,
		JournalID:     journalID,
		WorkspaceRoot: strings.TrimSpace(run.Meta.WorkspaceRoot),
		ParentSession: strings.TrimSpace(run.Meta.ParentSession),
	}
	if err := s.saveEffectJournal(journal); err != nil {
		return subagentEffectJournal{}, err
	}
	return journal, nil
}

func newSubagentEffectJournalID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate subagent effect journal ID: %w", err)
	}
	return "sej_" + hex.EncodeToString(raw[:]), nil
}

func (s *SubagentStore) loadEffectJournal(ref string) (subagentEffectJournal, error) {
	var journal subagentEffectJournal
	if s == nil || !validSubagentRef(ref) {
		return journal, fmt.Errorf("invalid subagent effect journal reference %q", ref)
	}
	file, err := os.Open(s.effectPath(ref))
	if err != nil {
		return journal, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSubagentEffectJournalBytes+1))
	if err != nil {
		return journal, fmt.Errorf("read subagent effect journal %q: %w", ref, err)
	}
	if len(data) > maxSubagentEffectJournalBytes {
		return journal, fmt.Errorf("subagent effect journal %q exceeds %d bytes", ref, maxSubagentEffectJournalBytes)
	}
	if err := json.Unmarshal(data, &journal); err != nil {
		return journal, fmt.Errorf("decode subagent effect journal %q: %w", ref, err)
	}
	if err := validateSubagentEffectJournal(journal, ref); err != nil {
		return subagentEffectJournal{}, err
	}
	return journal, nil
}

func validateSubagentEffectJournal(journal subagentEffectJournal, ref string) error {
	if journal.Version != subagentEffectJournalVersion {
		return fmt.Errorf("subagent effect journal %q has unsupported version %d", ref, journal.Version)
	}
	if journal.Ref != ref || journal.JournalID == "" || journal.WorkspaceRoot == "" || journal.ParentSession == "" {
		return fmt.Errorf("subagent effect journal %q has invalid identity", ref)
	}
	if len(journal.Events) > maxSubagentEffectEvents || journal.CompactedThrough > journal.NextSequence {
		return fmt.Errorf("subagent effect journal %q has invalid bounds", ref)
	}
	want := journal.CompactedThrough + 1
	for _, event := range journal.Events {
		if event.Sequence != want || (event.Phase != subagentEffectPhaseIntent && event.Phase != subagentEffectPhaseReceipt) || strings.TrimSpace(event.ParentToolCallID) == "" {
			return fmt.Errorf("subagent effect journal %q has invalid event sequence or anchor", ref)
		}
		normalized, err := makeDurableSubagentReceipt(event.Receipt.toReceipt(event.ParentToolCallID), event.Receipt.SubagentDepth)
		if err != nil {
			return fmt.Errorf("subagent effect journal %q event %d: %w", ref, event.Sequence, err)
		}
		if !reflect.DeepEqual(normalized, event.Receipt) {
			return fmt.Errorf("subagent effect journal %q event %d is not canonical", ref, event.Sequence)
		}
		want++
	}
	if len(journal.Events) == 0 {
		if journal.NextSequence != journal.CompactedThrough {
			return fmt.Errorf("subagent effect journal %q has a sequence gap", ref)
		}
	} else if journal.Events[len(journal.Events)-1].Sequence != journal.NextSequence {
		return fmt.Errorf("subagent effect journal %q has a sequence gap", ref)
	}
	return nil
}

func (s *SubagentStore) saveEffectJournal(journal subagentEffectJournal) error {
	var data []byte
	for {
		if err := validateSubagentEffectJournal(journal, journal.Ref); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(journal, "", "  ")
		if err != nil {
			return err
		}
		data = append(encoded, '\n')
		if len(data) <= maxSubagentEffectJournalBytes {
			break
		}
		if len(journal.Events) == 0 {
			return fmt.Errorf("subagent effect journal %q exceeds %d bytes", journal.Ref, maxSubagentEffectJournalBytes)
		}
		dropped := journal.Events[0]
		journal.CompactedThrough = dropped.Sequence
		if dropped.Receipt.Write && (dropped.Receipt.Success || dropped.Receipt.MutationAttempt) {
			journal.CompactedMutation = true
		}
		journal.Events = append([]subagentEffectEvent(nil), journal.Events[1:]...)
	}
	write := s.effectWrite
	if write == nil {
		write = fileutil.AtomicWriteFile
	}
	if err := write(s.effectPath(journal.Ref), data, 0o600); err != nil {
		return fmt.Errorf("persist subagent effect journal %q: %w", journal.Ref, err)
	}
	return nil
}

// RecoverSubagentEffects validates owned top-level journals against task or
// writer-skill calls in the parent transcript. It never imports child text or
// child-only project checks; retained receipts only invalidate stale root checks.
func (s *SubagentStore) RecoverSubagentEffects(parentSession, workspaceRoot string, parentMessages []provider.Message, acknowledged []evidence.SubagentEffectCursor) ([]RecoveredSubagentEffect, error) {
	if s == nil || strings.TrimSpace(parentSession) == "" {
		return nil, nil
	}
	parentSession = strings.TrimSpace(parentSession)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	cursors, err := normalizeSubagentEffectCursors(acknowledged)
	if err != nil {
		return nil, err
	}
	acknowledgedByRef := make(map[string]evidence.SubagentEffectCursor, len(cursors))
	for _, cursor := range cursors {
		acknowledgedByRef[cursor.Ref] = cursor
	}
	anchors := make(map[string]string)
	for _, message := range parentMessages {
		for _, call := range message.ToolCalls {
			if (call.Name == "task" || call.Name == "run_skill") && strings.TrimSpace(call.ID) != "" {
				anchors[call.ID] = call.Name
			}
		}
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var refs []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".effects.json") {
			continue
		}
		ref := strings.TrimSuffix(entry.Name(), ".effects.json")
		if !validSubagentRef(ref) {
			return nil, fmt.Errorf("invalid subagent effect journal filename %q", entry.Name())
		}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	var recovered []RecoveredSubagentEffect
	for _, ref := range refs {
		meta, err := s.LoadMeta(ref)
		if err != nil {
			journal, journalErr := s.loadEffectJournal(ref)
			if journalErr == nil && journal.ParentSession != parentSession {
				continue
			}
			return nil, fmt.Errorf("validate owner for subagent effect journal %q: %w", ref, errors.Join(err, journalErr))
		}
		if strings.TrimSpace(meta.ParentSession) != parentSession {
			continue
		}
		if meta.Workspace.Mode == SubagentWorkspaceGitWorktree {
			// Isolated child mutations are not source-workspace effects. The
			// delivery apply/merge tool records the eventual real mutation.
			continue
		}
		journal, err := s.loadEffectJournal(ref)
		if err != nil {
			return nil, err
		}
		if journal.ParentSession != parentSession || journal.WorkspaceRoot != workspaceRoot || meta.WorkspaceRoot != workspaceRoot {
			return nil, fmt.Errorf("subagent effect journal %q ownership does not match the active session", ref)
		}
		anchorTool := "task"
		if meta.Kind == "skill" {
			anchorTool = "run_skill"
		} else if meta.Kind != "task" {
			return nil, fmt.Errorf("subagent effect journal %q has unsupported owner kind %q", ref, meta.Kind)
		}
		acknowledgedSequence := uint64(0)
		if cursor, ok := acknowledgedByRef[ref]; ok {
			if cursor.JournalID != journal.JournalID || cursor.Sequence > journal.NextSequence {
				return nil, fmt.Errorf("subagent effect journal %q does not match its acknowledged cursor", ref)
			}
			acknowledgedSequence = cursor.Sequence
		}
		if journal.CompactedThrough > acknowledgedSequence {
			receipt := evidence.Receipt{}
			if journal.CompactedMutation {
				receipt = evidence.Receipt{ToolName: "subagent_effect_compaction", Write: true, MutationAttempt: true, Source: "subagent"}
			}
			recovered = append(recovered, RecoveredSubagentEffect{
				Cursor:  evidence.SubagentEffectCursor{Ref: journal.Ref, JournalID: journal.JournalID, Sequence: journal.CompactedThrough},
				Receipt: receipt,
			})
		}
		for _, event := range journal.Events {
			if event.Sequence <= acknowledgedSequence {
				continue
			}
			if anchors[event.ParentToolCallID] != anchorTool {
				return nil, fmt.Errorf("subagent effect journal %q event %d is not anchored to a delegation call in parent session %q", ref, event.Sequence, parentSession)
			}
			recovered = append(recovered, recoveredEffect(journal, event))
		}
	}
	return recovered, nil
}

func (s *SubagentStore) deliveryTests(ref string) []SubagentDeliveryTest {
	if s == nil || !validSubagentRef(ref) {
		return nil
	}
	journal, err := s.loadEffectJournal(ref)
	if err != nil {
		return nil
	}
	var tests []SubagentDeliveryTest
	seen := map[string]bool{}
	for _, event := range journal.Events {
		if event.Phase != subagentEffectPhaseReceipt || strings.TrimSpace(event.Receipt.Command) == "" {
			continue
		}
		key := event.Receipt.Command + "\x00" + fmt.Sprint(event.Receipt.Success)
		if seen[key] {
			continue
		}
		seen[key] = true
		tests = append(tests, SubagentDeliveryTest{Command: event.Receipt.Command, Success: event.Receipt.Success})
	}
	if len(tests) > 32 {
		tests = append([]SubagentDeliveryTest(nil), tests[len(tests)-32:]...)
	}
	return tests
}
