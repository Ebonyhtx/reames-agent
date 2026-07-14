// Package checkpoint is reamesAgent's snapshot-based edit safety net. Before a writer
// tool changes a file, the agent records the file's pre-edit content here, keyed
// to the current user turn; a frontend can then rewind the workspace (and, via the
// controller, the conversation) to an earlier turn.
//
// It is deliberately git-free (like Claude Code's rewind): snapshots live beside
// the session, never touch the user's git, and work in a non-git directory. Only
// edit-tool changes are tracked — bash side effects are not (a shell command's
// targets can't be known in advance), which is why the capture hook only fires for
// tools that can Preview their change.
package checkpoint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/diff"
	"reames-agent/internal/fileutil"
	fileenc "reames-agent/internal/fileutil/encoding"
)

// FileSnap is one file's state at the moment it was first touched in a turn.
// Content == nil means the file did not exist then, so a restore deletes it.
type FileSnap struct {
	Path     string        `json:"path"`
	Content  *string       `json:"content"`
	Encoding *fileenc.Kind `json:"encoding,omitempty"`
	Mode     *uint32       `json:"mode,omitempty"`
}

// Checkpoint anchors the pre-edit state of every distinct file touched during one
// user turn. MsgIndex is len(Session.Messages) at the turn's start — the
// conversation-rewind boundary — persisted so a resumed session can rewind the
// conversation and fork, not just the code.
type Checkpoint struct {
	Turn             int             `json:"turn"`
	Time             time.Time       `json:"time"`
	Prompt           string          `json:"prompt"`
	MsgIndex         int             `json:"msgIndex"`
	TranscriptDigest string          `json:"transcriptDigest,omitempty"`
	Runtime          json.RawMessage `json:"runtime,omitempty"`
	Files            []FileSnap      `json:"files"`
}

// Meta is the picker-facing summary of a checkpoint (no file contents).
type Meta struct {
	Turn   int
	Time   time.Time
	Prompt string
	Paths  []string
}

// Store holds a session's checkpoints in memory and, when dir is set, persists one
// JSON file per turn under it (cheap delete, corruption-isolated). All methods are
// safe for concurrent use — the agent snapshots from tool goroutines.
type Store struct {
	dir  string // <session>.ckpt/, or "" for in-memory only
	root string // workspace root, for restore path-escape guards

	mu           sync.Mutex
	done         []*Checkpoint   // finalized turns
	cur          *Checkpoint     // the active turn's checkpoint
	seen         map[string]bool // normalized paths already snapshotted this turn
	seenFiles    []seenFile      // existing file identities for hard-link aliases
	nextTurn     int             // durable monotonic allocation watermark
	retiredFrom  int             // inclusive tombstone range after a rewind
	retiredTo    int             // inclusive tombstone range after a rewind
	hasRetired   bool
	stateCorrupt bool // existing manifest was unreadable; old records fail closed
	// restoreWrite is a fault-injection seam for transaction tests.
	restoreWrite func(string, []byte, os.FileMode) error
	// restoreBeforeApply is a test seam for path-component replacement races.
	restoreBeforeApply func()
	// stateWrite is a fault-injection seam for durable truncate tests.
	stateWrite func(string, []byte, os.FileMode) error
	// recordWrite is a fault-injection seam for turn/runtime snapshot tests.
	recordWrite func(string, []byte, os.FileMode) error
	// recordRemove is a fault-injection seam for truncate garbage collection.
	recordRemove func(string) error
}

type seenFile struct {
	identity string
	info     os.FileInfo
	snap     FileSnap
}

const checkpointStateFilename = ".state.json"

type checkpointDiskState struct {
	Version     int  `json:"version"`
	NextTurn    int  `json:"nextTurn"`
	RetiredFrom int  `json:"retiredFrom,omitempty"`
	RetiredTo   int  `json:"retiredTo,omitempty"`
	HasRetired  bool `json:"hasRetired,omitempty"`
}

// New returns a store for the given checkpoint dir and workspace root, loading any
// checkpoints already persisted under dir. A "" dir disables persistence (the
// store still works in memory for the session).
func New(dir, root string) *Store {
	if root == "" {
		root, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	s := &Store{dir: dir, root: root, seen: map[string]bool{}}
	if dir != "" {
		s.load()
	}
	return s
}

func (s *Store) load() {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	if b, readErr := os.ReadFile(filepath.Join(s.dir, checkpointStateFilename)); readErr == nil {
		var state checkpointDiskState
		if json.Unmarshal(b, &state) == nil && state.Version == 1 && state.NextTurn >= 0 &&
			(!state.HasRetired || (state.RetiredFrom >= 0 && state.RetiredTo >= state.RetiredFrom && state.RetiredTo < state.NextTurn)) {
			s.nextTurn = state.NextTurn
			s.retiredFrom, s.retiredTo, s.hasRetired = state.RetiredFrom, state.RetiredTo, state.HasRetired
		} else {
			slog.Warn("checkpoint: ignoring invalid state manifest", "dir", s.dir)
			s.stateCorrupt = true
		}
	} else if !os.IsNotExist(readErr) {
		slog.Warn("checkpoint: read state manifest", "dir", s.dir, "err", readErr)
		s.stateCorrupt = true
	}
	seenTurns := map[int]bool{}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		filenameTurn, ok := checkpointTurnFromFilename(e.Name())
		if !ok {
			continue
		}
		if filenameTurn >= s.nextTurn {
			s.nextTurn = filenameTurn + 1
		}
		b, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			slog.Warn("checkpoint: read record", "path", e.Name(), "err", err)
			continue
		}
		var c Checkpoint
		if err := json.Unmarshal(b, &c); err != nil || c.Turn < 0 || c.MsgIndex < 0 || c.Turn != filenameTurn || seenTurns[c.Turn] {
			slog.Warn("checkpoint: rejecting invalid record", "path", e.Name(), "turn", c.Turn, "msg_index", c.MsgIndex, "err", err)
			continue
		}
		seenTurns[c.Turn] = true
		if s.stateCorrupt {
			continue
		}
		if s.hasRetired && c.Turn >= s.retiredFrom && c.Turn <= s.retiredTo {
			continue
		}
		s.done = append(s.done, &c)
	}
	sort.Slice(s.done, func(i, j int) bool { return s.done[i].Turn < s.done[j].Turn })
}

func checkpointTurnFromFilename(name string) (int, bool) {
	if !strings.HasPrefix(name, "turn-") || !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(name, "turn-"), ".json")
	turn, err := strconv.Atoi(value)
	return turn, err == nil && turn >= 0 && name == fmt.Sprintf("turn-%d.json", turn)
}

// Begin opens a checkpoint for a new user turn, finalizing the previous one. The
// prompt labels it in the picker; msgIndex is the conversation-rewind boundary.
func (s *Store) Begin(turn int, prompt string, msgIndex int, runtime ...json.RawMessage) error {
	var state json.RawMessage
	if len(runtime) > 0 {
		state = runtime[0]
	}
	return s.BeginAnchored(turn, prompt, msgIndex, "", state)
}

// BeginAnchored records the digest of the transcript prefix ending at msgIndex.
// Conversation rewind and fork require this anchor to detect same-length
// rewrites that an integer boundary alone cannot distinguish.
func (s *Store) BeginAnchored(turn int, prompt string, msgIndex int, transcriptDigest string, runtime json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if turn < 0 || msgIndex < 0 || turn != s.nextTurn {
		return fmt.Errorf("invalid checkpoint boundary turn=%d message_index=%d next_turn=%d", turn, msgIndex, s.nextTurn)
	}
	if s.cur != nil {
		s.done = append(s.done, s.cur)
	}
	s.cur = nil
	s.seen = map[string]bool{}
	s.seenFiles = nil
	oldNext := s.nextTurn
	oldFrom, oldTo, oldHas := s.retiredFrom, s.retiredTo, s.hasRetired
	if turn >= s.nextTurn {
		s.nextTurn = turn + 1
	}
	if s.stateCorrupt {
		// The missing/corrupt tombstone boundary makes every old record
		// untrustworthy. Retire the full allocated range before admitting a new
		// turn; this loses rewind history but cannot resurrect future state.
		if turn > 0 {
			s.retiredFrom, s.retiredTo, s.hasRetired = 0, turn-1, true
		}
	}
	if err := s.persistStateLocked(); err != nil {
		s.nextTurn = oldNext
		s.retiredFrom, s.retiredTo, s.hasRetired = oldFrom, oldTo, oldHas
		slog.Warn("checkpoint: persist allocation state", "turn", turn, "err", err)
		return fmt.Errorf("persist checkpoint allocation state: %w", err)
	}
	s.stateCorrupt = false
	candidate := &Checkpoint{
		Turn: turn, Time: time.Now(), Prompt: prompt, MsgIndex: msgIndex,
		TranscriptDigest: transcriptDigest, Runtime: append(json.RawMessage(nil), runtime...),
	}
	if err := s.persist(candidate); err != nil {
		slog.Warn("checkpoint: persist turn boundary", "turn", turn, "err", err)
		return err
	}
	s.cur = candidate
	return nil
}

// Runtime returns the session runtime projection captured at turn start.
func (s *Store) Runtime(turn int) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.done {
		if c.Turn == turn && len(c.Runtime) > 0 {
			return append(json.RawMessage(nil), c.Runtime...), true
		}
	}
	if s.cur != nil && s.cur.Turn == turn && len(s.cur.Runtime) > 0 {
		return append(json.RawMessage(nil), s.cur.Runtime...), true
	}
	return nil, false
}

// TranscriptDigest returns the persisted digest for a checkpoint's message
// prefix. An empty digest denotes a legacy checkpoint and cannot authorize a
// conversation rewrite.
func (s *Store) TranscriptDigest(turn int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.done {
		if c.Turn == turn {
			return c.TranscriptDigest, c.TranscriptDigest != ""
		}
	}
	if s.cur != nil && s.cur.Turn == turn {
		return s.cur.TranscriptDigest, s.cur.TranscriptDigest != ""
	}
	return "", false
}

// Bounds returns turn → MsgIndex over all checkpoints (persisted + current), so
// the controller can rebuild its conversation-rewind boundaries after loading a
// resumed session's checkpoints from disk.
func (s *Store) Bounds() map[int]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[int]int, len(s.done)+1)
	for _, c := range s.done {
		m[c.Turn] = c.MsgIndex
	}
	if s.cur != nil {
		m[s.cur.Turn] = s.cur.MsgIndex
	}
	return m
}

// Snapshot records the pre-edit state of the file a writer is about to change.
// Only the first touch of a path in the current turn is kept (that is its
// turn-start content). It fails when no durable turn is active.
func (s *Store) Snapshot(ch diff.Change) error {
	return s.snapshot(ch, nil)
}

// CurrentTurn returns the active checkpoint turn. Callers can use the value
// with SnapshotForTurn to bind delayed background effects to their origin turn.
func (s *Store) CurrentTurn() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return 0, false
	}
	return s.cur.Turn, true
}

// SnapshotForTurn records ch only while turn remains the active checkpoint.
// It prevents a late background writer from landing in a newer turn.
func (s *Store) SnapshotForTurn(turn int, ch diff.Change) error {
	return s.snapshot(ch, &turn)
}

func (s *Store) snapshot(ch diff.Change, expectedTurn *int) error {
	if ch.Path == "" {
		return fmt.Errorf("checkpoint snapshot path is empty")
	}
	abs, rel, pathErr := workspaceRelativePath(s.root, ch.Path)
	if pathErr != nil {
		return fmt.Errorf("checkpoint snapshot path %q: %w", ch.Path, pathErr)
	}
	identity := canonicalPathIdentity(rel)
	var identityInfo os.FileInfo
	identityInfo, _ = os.Stat(abs)
	var enc *fileenc.Kind
	var mode *uint32
	if ch.Kind != diff.Create {
		enc = s.detectEncoding(abs)
		if info, err := os.Lstat(abs); err == nil && info.Mode().IsRegular() {
			m := uint32(info.Mode().Perm())
			mode = &m
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return fmt.Errorf("checkpoint snapshot has no active turn")
	}
	if expectedTurn != nil && s.cur.Turn != *expectedTurn {
		return fmt.Errorf("checkpoint turn %d is no longer active (current turn %d)", *expectedTurn, s.cur.Turn)
	}
	if s.seen[identity] {
		return nil
	}
	var content *string
	if ch.Kind != diff.Create { // create == file didn't exist → leave nil (restore deletes)
		old := ch.OldText
		content = &old
	}
	snap := FileSnap{Path: rel, Content: content, Encoding: enc, Mode: mode}
	if existing, ok := sameSeenFile(identityInfo, s.seenFiles); ok {
		// A second hard-link name denotes the same pre-edit inode. Preserve the
		// earliest bytes for every alias so later snapshots cannot restore an
		// intermediate state through another name.
		snap = existing.snap
		snap.Path = rel
	}
	s.seen[identity] = true
	s.seenFiles = append(s.seenFiles, seenFile{identity: identity, info: identityInfo, snap: snap})
	s.cur.Files = append(s.cur.Files, snap)
	if err := s.persist(s.cur); err != nil {
		delete(s.seen, identity)
		s.seenFiles = s.seenFiles[:len(s.seenFiles)-1]
		s.cur.Files = s.cur.Files[:len(s.cur.Files)-1]
		return err
	}
	return nil
}

func (s *Store) detectEncoding(p string) *fileenc.Kind {
	abs, err := safePath(s.root, p)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	enc, _ := fileenc.Detect(b)
	return &enc
}

func (s *Store) persist(c *Checkpoint) error {
	if s.dir == "" {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal checkpoint turn %d: %w", c.Turn, err)
	}
	writeFile := s.recordWrite
	if writeFile == nil {
		writeFile = fileutil.AtomicWriteFile
	}
	if err := writeFile(filepath.Join(s.dir, fmt.Sprintf("turn-%d.json", c.Turn)), b, 0o600); err != nil {
		return fmt.Errorf("persist checkpoint turn %d: %w", c.Turn, err)
	}
	return nil
}

func (s *Store) persistStateLocked() error {
	if s.dir == "" {
		return nil
	}
	b, err := json.Marshal(checkpointDiskState{
		Version: 1, NextTurn: s.nextTurn,
		RetiredFrom: s.retiredFrom, RetiredTo: s.retiredTo, HasRetired: s.hasRetired,
	})
	if err != nil {
		return err
	}
	writeFile := s.stateWrite
	if writeFile == nil {
		writeFile = fileutil.AtomicWriteFile
	}
	return writeFile(filepath.Join(s.dir, checkpointStateFilename), b, 0o600)
}

// NextTurn returns the turn number a new checkpoint should take: one past the
// highest existing turn (0 when empty), so a resumed session keeps numbering
// without colliding with checkpoints loaded from disk.
func (s *Store) NextTurn() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextTurn
}

// List returns every checkpoint's metadata, oldest turn first.
func (s *Store) List() []Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Meta, 0, len(s.done)+1)
	for _, c := range s.all() {
		paths := make([]string, len(c.Files))
		for i, f := range c.Files {
			paths[i] = f.Path
		}
		out = append(out, Meta{Turn: c.Turn, Time: c.Time, Prompt: c.Prompt, Paths: paths})
	}
	return out
}

// all returns done + cur in turn order. Caller holds the lock.
func (s *Store) all() []*Checkpoint {
	cps := append([]*Checkpoint(nil), s.done...)
	if s.cur != nil {
		cps = append(cps, s.cur)
	}
	sort.Slice(cps, func(i, j int) bool { return cps[i].Turn < cps[j].Turn })
	return cps
}

// TruncateFrom discards checkpoints at or after fromTurn. Conversation rewind
// removes those future turns from the transcript, so their file snapshots must
// not remain visible or collide with newly-created checkpoints that reuse the
// same turn numbers after the rewrite.
func (s *Store) TruncateFrom(fromTurn int) error {
	if fromTurn < 0 {
		return fmt.Errorf("checkpoint truncate turn must be non-negative")
	}
	s.mu.Lock()
	deleteTurns := map[int]bool{}
	retiredTo := -1
	for _, c := range s.done {
		if c.Turn >= fromTurn {
			deleteTurns[c.Turn] = true
			if c.Turn > retiredTo {
				retiredTo = c.Turn
			}
		}
	}
	if s.cur != nil && s.cur.Turn >= fromTurn {
		deleteTurns[s.cur.Turn] = true
		if s.cur.Turn > retiredTo {
			retiredTo = s.cur.Turn
		}
	}
	if retiredTo < 0 {
		s.mu.Unlock()
		return nil
	}
	oldFrom, oldTo, oldHas := s.retiredFrom, s.retiredTo, s.hasRetired
	if !s.hasRetired || fromTurn < s.retiredFrom {
		s.retiredFrom = fromTurn
	}
	if !s.hasRetired || retiredTo > s.retiredTo {
		s.retiredTo = retiredTo
	}
	s.hasRetired = true
	if err := s.persistStateLocked(); err != nil {
		s.retiredFrom, s.retiredTo, s.hasRetired = oldFrom, oldTo, oldHas
		s.mu.Unlock()
		return fmt.Errorf("persist checkpoint truncate state: %w", err)
	}
	done := s.done[:0]
	for _, c := range s.done {
		if c.Turn >= fromTurn {
			continue
		}
		done = append(done, c)
	}
	for i := len(done); i < len(s.done); i++ {
		s.done[i] = nil
	}
	s.done = done
	if s.cur != nil && s.cur.Turn >= fromTurn {
		s.cur = nil
		s.seen = map[string]bool{}
		s.seenFiles = nil
	}
	dir := s.dir
	s.mu.Unlock()

	if dir == "" || len(deleteTurns) == 0 {
		return nil
	}
	removeRecord := s.recordRemove
	if removeRecord == nil {
		removeRecord = os.Remove
	}
	for turn := range deleteTurns {
		if err := removeRecord(filepath.Join(dir, fmt.Sprintf("turn-%d.json", turn))); err != nil && !os.IsNotExist(err) {
			slog.Warn("checkpoint: truncate failed", "turn", turn, "err", err)
		}
	}
	return nil
}

// RestoreCode reverts the workspace to its state at the start of turn `fromTurn`:
// for every file touched in turn fromTurn or later, it writes back that file's
// earliest recorded content (or deletes it when the earliest snapshot was nil).
// Returns the paths written and deleted.
func (s *Store) RestoreCode(fromTurn int) (written, deleted []string, err error) {
	if fromTurn < 0 {
		return nil, nil, fmt.Errorf("checkpoint restore turn must be non-negative")
	}
	s.mu.Lock()
	// earliest snapshot per path across checkpoints >= fromTurn (turn order → first wins).
	type earliestSnap struct {
		identity string
		info     os.FileInfo
		snap     FileSnap
	}
	var earliest []earliestSnap
	for _, c := range s.all() {
		if c.Turn < fromTurn {
			continue
		}
		for _, f := range c.Files {
			abs, rel, pathErr := workspaceRelativePath(s.root, f.Path)
			identity := canonicalPathIdentity(f.Path)
			var info os.FileInfo
			if pathErr == nil {
				identity = canonicalPathIdentity(rel)
				info, _ = os.Stat(abs)
			}
			var hardlinkBase *FileSnap
			duplicatePath := false
			for i := range earliest {
				existing := &earliest[i]
				if existing.identity == identity {
					duplicatePath = true
					break
				}
				if info != nil && existing.info != nil && os.SameFile(info, existing.info) {
					hardlinkBase = &existing.snap
					break
				}
			}
			if duplicatePath {
				continue
			}
			if hardlinkBase != nil {
				f.Content, f.Encoding, f.Mode = hardlinkBase.Content, hardlinkBase.Encoding, hardlinkBase.Mode
			}
			earliest = append(earliest, earliestSnap{identity: identity, info: info, snap: f})
		}
	}
	root := s.root
	writeFile := s.restoreWrite
	beforeApply := s.restoreBeforeApply
	s.mu.Unlock()
	sort.Slice(earliest, func(i, j int) bool { return earliest[i].snap.Path < earliest[j].snap.Path })
	rootHandle, openErr := os.OpenRoot(root)
	if openErr != nil {
		return nil, nil, fmt.Errorf("open checkpoint workspace root %q: %w", root, openErr)
	}
	defer rootHandle.Close()
	rootInfo, statRootErr := os.Stat(root)
	if statRootErr != nil {
		return nil, nil, fmt.Errorf("inspect checkpoint workspace root %q: %w", root, statRootErr)
	}
	if !rootInfo.IsDir() {
		return nil, nil, fmt.Errorf("checkpoint workspace root %q is not a directory", root)
	}

	type diskState struct {
		exists bool
		data   []byte
		mode   os.FileMode
		info   os.FileInfo
	}
	type restoreOp struct {
		path        string
		abs         string
		rel         string
		parentRel   string
		base        string
		parent      *os.Root
		parentInfo  os.FileInfo
		closeParent bool
		snap        FileSnap
		before      diskState
	}
	ops := make([]restoreOp, 0, len(earliest))
	defer func() {
		for i := range ops {
			if ops[i].closeParent && ops[i].parent != nil {
				_ = ops[i].parent.Close()
			}
		}
	}()
	for _, entry := range earliest {
		p := entry.snap.Path
		abs, rel, pathErr := restoreRelativePath(root, p)
		if pathErr != nil {
			return nil, nil, pathErr
		}
		parentRel, base := filepath.Dir(rel), filepath.Base(rel)
		parent := rootHandle
		parentInfo := rootInfo
		closeParent := false
		if parentRel != "." {
			parent, pathErr = rootHandle.OpenRoot(parentRel)
			if pathErr != nil && !os.IsNotExist(pathErr) {
				return nil, nil, fmt.Errorf("checkpoint parent %q cannot be pinned: %w", parentRel, pathErr)
			}
			if pathErr == nil {
				closeParent = true
				parentInfo, pathErr = parent.Stat(".")
				if pathErr != nil {
					_ = parent.Close()
					return nil, nil, fmt.Errorf("checkpoint parent %q cannot be inspected: %w", parentRel, pathErr)
				}
			} else {
				parent = nil
				parentInfo = nil
			}
		}
		closePinnedParent := func() {
			if closeParent && parent != nil {
				_ = parent.Close()
			}
		}
		before := diskState{}
		var info os.FileInfo
		var statErr error
		if parent != nil {
			info, statErr = parent.Lstat(base)
		} else {
			info, statErr = rootHandle.Lstat(rel)
		}
		switch {
		case statErr == nil:
			if !info.Mode().IsRegular() {
				closePinnedParent()
				return nil, nil, fmt.Errorf("checkpoint path %q is not a regular file", p)
			}
			if parent == nil {
				return nil, nil, fmt.Errorf("checkpoint parent %q appeared during preflight", parentRel)
			}
			before.exists = true
			before.mode = info.Mode().Perm()
			before.info = info
			before.data, statErr = parent.ReadFile(base)
			if statErr != nil {
				closePinnedParent()
				return nil, nil, fmt.Errorf("checkpoint preflight read %q: %w", p, statErr)
			}
		case os.IsNotExist(statErr):
		default:
			closePinnedParent()
			return nil, nil, fmt.Errorf("checkpoint preflight stat %q: %w", p, statErr)
		}
		ops = append(ops, restoreOp{
			path: p, abs: abs, rel: rel, parentRel: parentRel, base: base,
			parent: parent, parentInfo: parentInfo, closeParent: closeParent,
			snap: entry.snap, before: before,
		})
	}
	writeRestore := func(op *restoreOp, data []byte, mode os.FileMode) error {
		if writeFile != nil {
			return writeFile(op.abs, data, mode)
		}
		return fileutil.AtomicWriteRootFile(op.parent, op.base, data, mode)
	}

	parentStillMapped := func(op *restoreOp) error {
		var current os.FileInfo
		var err error
		if op.parentRel == "." {
			current, err = os.Stat(root)
		} else {
			current, err = rootHandle.Stat(op.parentRel)
		}
		if err != nil || current == nil || op.parentInfo == nil || !os.SameFile(current, op.parentInfo) {
			if err == nil {
				err = fmt.Errorf("directory identity changed")
			}
			return fmt.Errorf("checkpoint parent %q changed during restore: %w", op.parentRel, err)
		}
		return nil
	}

	ensureParent := func(op *restoreOp) error {
		if op.parent != nil {
			return parentStillMapped(op)
		}
		if err := rootHandle.MkdirAll(op.parentRel, 0o755); err != nil {
			return fmt.Errorf("create checkpoint parent %q: %w", op.parentRel, err)
		}
		parent, err := rootHandle.OpenRoot(op.parentRel)
		if err != nil {
			return fmt.Errorf("pin checkpoint parent %q: %w", op.parentRel, err)
		}
		info, err := parent.Stat(".")
		if err != nil {
			_ = parent.Close()
			return fmt.Errorf("inspect checkpoint parent %q: %w", op.parentRel, err)
		}
		op.parent, op.parentInfo, op.closeParent = parent, info, true
		return parentStillMapped(op)
	}

	validateTarget := func(op *restoreOp) error {
		if op.parent == nil {
			if _, err := rootHandle.Lstat(op.rel); err == nil {
				return fmt.Errorf("checkpoint path %q appeared during restore", op.path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("checkpoint path %q cannot be revalidated: %w", op.path, err)
			}
			return nil
		}
		if err := parentStillMapped(op); err != nil {
			return err
		}
		info, err := op.parent.Lstat(op.base)
		if !op.before.exists {
			if err == nil {
				return fmt.Errorf("checkpoint path %q appeared during restore", op.path)
			}
			if !os.IsNotExist(err) {
				return fmt.Errorf("checkpoint path %q cannot be revalidated: %w", op.path, err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("checkpoint path %q disappeared during restore: %w", op.path, err)
		}
		if !info.Mode().IsRegular() || !os.SameFile(info, op.before.info) {
			return fmt.Errorf("checkpoint path %q changed identity during restore", op.path)
		}
		data, err := op.parent.ReadFile(op.base)
		if err != nil {
			return fmt.Errorf("checkpoint path %q cannot be re-read: %w", op.path, err)
		}
		if !bytes.Equal(data, op.before.data) || info.Mode().Perm() != op.before.mode {
			return fmt.Errorf("checkpoint path %q changed content or mode during restore", op.path)
		}
		return nil
	}

	rollback := func(applied []*restoreOp) error {
		var rollbackErr error
		for i := len(applied) - 1; i >= 0; i-- {
			op := applied[i]
			if op.before.exists {
				if e := fileutil.AtomicWriteRootFile(op.parent, op.base, op.before.data, op.before.mode); e != nil {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %q: %w", op.path, e))
				}
				continue
			}
			if e := op.parent.Remove(op.base); e != nil && !os.IsNotExist(e) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove %q: %w", op.path, e))
			}
		}
		return rollbackErr
	}

	if beforeApply != nil {
		beforeApply()
	}
	for i := range ops {
		if err := validateTarget(&ops[i]); err != nil {
			return nil, nil, err
		}
	}
	applied := make([]*restoreOp, 0, len(ops))
	for i := range ops {
		op := &ops[i]
		var applyErr error
		touched := false
		if op.snap.Content == nil {
			if op.before.exists {
				if applyErr = validateTarget(op); applyErr == nil {
					touched = true
					applyErr = op.parent.Remove(op.base)
				}
			}
		} else {
			if applyErr = ensureParent(op); applyErr == nil {
				applyErr = validateTarget(op)
			}
			enc := fileenc.UTF8
			if op.snap.Encoding != nil {
				enc = *op.snap.Encoding
			} else if op.before.exists {
				enc, _ = fileenc.Detect(op.before.data)
			}
			mode := os.FileMode(0)
			if op.snap.Mode != nil {
				mode = os.FileMode(*op.snap.Mode).Perm()
			} else {
				mode = op.before.mode
				if mode == 0 {
					mode = 0o644
				}
			}
			if applyErr == nil {
				touched = true
				applyErr = writeRestore(op, fileenc.Encode(*op.snap.Content, enc), mode)
			}
		}
		if applyErr == nil && touched {
			applyErr = parentStillMapped(op)
		}
		if applyErr != nil {
			// A writer may report failure after touching the destination (for
			// example a degraded cross-device copy). Restore the current op as
			// well as every earlier one so the transaction returns to preflight.
			rollbackOps := applied
			if touched {
				rollbackOps = append(append([]*restoreOp(nil), applied...), op)
			}
			if rbErr := rollback(rollbackOps); rbErr != nil {
				applyErr = errors.Join(applyErr, fmt.Errorf("rollback: %w", rbErr))
			}
			return nil, nil, fmt.Errorf("checkpoint restore %q: %w", op.path, applyErr)
		}
		if touched {
			applied = append(applied, op)
		}
		if op.snap.Content == nil {
			if op.before.exists {
				deleted = append(deleted, op.path)
			}
		} else {
			written = append(written, op.path)
		}
	}
	return written, deleted, nil
}

func restoreRelativePath(root, path string) (abs, rel string, err error) {
	return workspaceRelativePath(root, path)
}

func workspaceRelativePath(root, path string) (abs, rel string, err error) {
	abs, err = safePath(root, path)
	if err != nil {
		return "", "", err
	}
	rel, err = filepath.Rel(filepath.Clean(root), abs)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return "", "", fmt.Errorf("checkpoint path %q escapes workspace %q", path, root)
	}
	return abs, filepath.Clean(rel), nil
}

func canonicalPathIdentity(path string) string {
	identity := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	return identity
}

func sameSeenFile(info os.FileInfo, existing []seenFile) (seenFile, bool) {
	if info == nil {
		return seenFile{}, false
	}
	for _, other := range existing {
		if other.info != nil && os.SameFile(info, other.info) {
			return other, true
		}
	}
	return seenFile{}, false
}

// safePath resolves p against root and rejects anything escaping it — restore
// must never write outside the workspace, even if a snapshot path is hostile or
// the project moved since it was taken.
func safePath(root, p string) (string, error) {
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	abs = filepath.Clean(abs)
	if root != "" {
		r := filepath.Clean(root)
		rel, err := filepath.Rel(r, abs)
		if err != nil || !filepath.IsLocal(rel) {
			return "", fmt.Errorf("checkpoint path %q escapes workspace %q", p, root)
		}
	}
	return abs, nil
}
