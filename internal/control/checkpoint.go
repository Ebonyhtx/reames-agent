package control

import (
	"encoding/json"
	"fmt"
	"sync"

	"reames-agent/internal/agent"
	"reames-agent/internal/checkpoint"
	"reames-agent/internal/diff"
)

// checkpointManager owns the snapshot-based rewind bookkeeping: the per-session
// checkpoint store, the monotonic turn counter, and the conversation-rewind
// boundary map. Like approvalManager it holds only the bookkeeping behind its own
// lock, off the controller's c.mu — the Controller keeps the rewind/fork
// orchestration (truncating the session, restoring code, emitting events) that
// needs its other collaborators.
//
// turn is decoupled from the store so it never collides after a log restructure;
// bound[turn] records len(Session.Messages) at that turn's start — the truncation
// boundary for a conversation rewind/fork. Boundaries are persisted in each
// checkpoint and rebuilt from the store on resume (so a reopened session can still
// rewind conversation / fork), but dropped after a summarize restructures the log
// so those operations report "unavailable" rather than mis-truncating; code
// rewind (file-based) is unaffected. Every store call does its disk I/O off mu —
// mu is taken only to read/swap the store pointer and mutate turn/bound.
type checkpointManager struct {
	// mu guards store, turn, and bound; every critical section under it is short
	// and non-blocking (no disk I/O).
	mu    sync.Mutex
	store *checkpoint.Store
	turn  int
	bound map[int]int
}

// rebind points the store at the (possibly new) session, loading any checkpoints
// already on disk, and resets the turn counter and boundaries from them. root is
// the workspace root used to guard restore writes. Called on construction and
// whenever the session path changes (NewSession/Resume/SetSessionPath/fork).
func (m *checkpointManager) rebind(dir, root string) {
	store := checkpoint.New(dir, root)
	next := store.NextTurn() // continue numbering past any checkpoints on disk
	bound := store.Bounds()  // rebuilt from persisted checkpoints so a resumed
	if bound == nil {        // session can still rewind conversation / fork
		bound = map[int]int{}
	}
	m.mu.Lock()
	m.store = store
	m.turn = next
	m.bound = bound
	m.mu.Unlock()
}

// enabled reports whether a checkpoint store is bound.
func (m *checkpointManager) enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store != nil
}

// begin opens a checkpoint for the turn about to run, recording msgIndex as the
// conversation-rewind boundary. No-op when checkpoints are disabled.
func (m *checkpointManager) begin(input string, msgIndex int, transcriptDigest string, runtime json.RawMessage) error {
	m.mu.Lock()
	store := m.store
	if store == nil {
		m.mu.Unlock()
		return nil
	}
	turn := m.turn
	m.mu.Unlock()
	err := store.BeginAnchored(turn, input, msgIndex, transcriptDigest, runtime)
	next := store.NextTurn()
	m.mu.Lock()
	if m.store == store {
		m.turn = next
		if err == nil {
			m.bound[turn] = msgIndex
		} else {
			delete(m.bound, turn)
		}
	}
	m.mu.Unlock()
	return err
}

func (m *checkpointManager) runtime(turn int) (json.RawMessage, bool) {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return nil, false
	}
	return store.Runtime(turn)
}

func (m *checkpointManager) transcriptDigest(turn int) (string, bool) {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return "", false
	}
	return store.TranscriptDigest(turn)
}

// turnsByMessageIndex returns message-log index -> checkpoint turn over live
// boundaries. The desktop transcript uses this authoritative map instead of
// recounting visible user bubbles, which can diverge when synthetic user-role
// messages are hidden from the UI.
func (m *checkpointManager) turnsByMessageIndex() map[int]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[int]int, len(m.bound))
	for turn, index := range m.bound {
		if existing, ok := out[index]; ok && existing < turn {
			continue
		}
		out[index] = turn
	}
	return out
}

// boundary returns the recorded turn-start message index, if any.
func (m *checkpointManager) boundary(turn int) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bound[turn]
	return b, ok
}

// list returns the checkpoint metadata (nil when disabled).
func (m *checkpointManager) list() []checkpoint.Meta {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.List()
}

// restoreCode reverts every file changed at or after turn to its pre-turn
// content. Errors when checkpoints are disabled.
func (m *checkpointManager) restoreCode(turn int) (written, deleted []string, err error) {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return nil, nil, fmt.Errorf("checkpoints unavailable")
	}
	return store.RestoreCode(turn)
}

// snapshot records a pre-edit file change into the open checkpoint — the
// executor's pre-edit hook. No-op when disabled.
func (m *checkpointManager) snapshot(ch diff.Change) error {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.Snapshot(ch)
}

// scopedSnapshot captures the current store and checkpoint turn for delegated
// writers. A background child that finishes after a later turn starts cannot
// append its pre-edit bytes to that later turn's checkpoint.
func (m *checkpointManager) scopedSnapshot() agent.PreEditHook {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store == nil {
		return nil
	}
	turn, ok := store.CurrentTurn()
	if !ok {
		return func(diff.Change) error {
			return fmt.Errorf("checkpoint has no active turn")
		}
	}
	return func(ch diff.Change) error {
		return store.SnapshotForTurn(turn, ch)
	}
}

// persistWriterRecoveryState is the root writer's fail-closed pre-edit gate.
// The checkpoint captures workspace bytes and turn-start runtime; the sidecar
// refresh captures the latest transcript anchor. The in-flight marker lets a
// resumed process distinguish a partial turn from a completed one.
func (c *Controller) persistWriterRecoveryState(ch diff.Change) error {
	// A mid-turn autosave may recover onto a new session path and rebind the
	// checkpoint store. Keep the checkpoint, sidecar, and marker on one path by
	// sharing the same handoff lock used by snapshot recovery and session swaps.
	c.snapshotMu.Lock()
	defer c.snapshotMu.Unlock()
	if err := c.checkpoints.snapshot(ch); err != nil {
		return fmt.Errorf("checkpoint snapshot: %w", err)
	}
	if err := c.goals.persistRuntime(c.goalRuntimeProjection()); err != nil {
		return fmt.Errorf("runtime sidecar: %w", err)
	}
	if err := c.ensureInFlightTurnPersisted(); err != nil {
		return fmt.Errorf("in-flight turn marker: %w", err)
	}
	return nil
}

// truncateFrom renumbers future turns from `turn` and drops every boundary at or
// after it — the conversation-rewind renumber after the message log is cut back.
func (m *checkpointManager) truncateFrom(turn int) error {
	m.mu.Lock()
	store := m.store
	m.mu.Unlock()
	if store != nil {
		if err := store.TruncateFrom(turn); err != nil {
			return err
		}
	}
	m.mu.Lock()
	for k := range m.bound {
		if k >= turn {
			delete(m.bound, k)
		}
	}
	m.mu.Unlock()
	return nil
}

// clearBounds drops every boundary after a summarize restructures the log (so
// conversation rewind degrades to "unavailable" until fresh turns rebuild them)
// while keeping turn monotonic so new turns don't collide with the store.
func (m *checkpointManager) clearBounds() {
	m.mu.Lock()
	m.bound = map[int]int{}
	m.mu.Unlock()
}
