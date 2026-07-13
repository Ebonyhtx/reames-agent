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
	// stateWrite is a fault-injection seam for durable truncate tests.
	stateWrite func(string, []byte, os.FileMode) error
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
	return turn, err == nil && turn >= 0
}

// Begin opens a checkpoint for a new user turn, finalizing the previous one. The
// prompt labels it in the picker; msgIndex is the conversation-rewind boundary.
func (s *Store) Begin(turn int, prompt string, msgIndex int, runtime ...json.RawMessage) {
	var state json.RawMessage
	if len(runtime) > 0 {
		state = runtime[0]
	}
	s.BeginAnchored(turn, prompt, msgIndex, "", state)
}

// BeginAnchored records the digest of the transcript prefix ending at msgIndex.
// Conversation rewind and fork require this anchor to detect same-length
// rewrites that an integer boundary alone cannot distinguish.
func (s *Store) BeginAnchored(turn int, prompt string, msgIndex int, transcriptDigest string, runtime json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if turn < 0 || msgIndex < 0 || turn < s.nextTurn {
		return
	}
	if s.cur != nil {
		s.done = append(s.done, s.cur)
	}
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
		slog.Warn("checkpoint: persist allocation state", "turn", turn, "err", err)
		return
	}
	s.stateCorrupt = false
	s.cur = &Checkpoint{
		Turn: turn, Time: time.Now(), Prompt: prompt, MsgIndex: msgIndex,
		TranscriptDigest: transcriptDigest, Runtime: append(json.RawMessage(nil), runtime...),
	}
	s.seen = map[string]bool{}
	s.seenFiles = nil
	s.persist(s.cur)
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
// turn-start content). A no-op before the first Begin.
func (s *Store) Snapshot(ch diff.Change) {
	s.snapshot(ch, nil)
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
func (s *Store) SnapshotForTurn(turn int, ch diff.Change) {
	s.snapshot(ch, &turn)
}

func (s *Store) snapshot(ch diff.Change, expectedTurn *int) {
	if ch.Path == "" {
		return
	}
	abs, pathErr := safePath(s.root, ch.Path)
	identity := canonicalPathIdentity(ch.Path)
	var identityInfo os.FileInfo
	if pathErr == nil {
		identity = canonicalPathIdentity(abs)
		identityInfo, _ = os.Stat(abs)
	}
	var enc *fileenc.Kind
	var mode *uint32
	if ch.Kind != diff.Create {
		if pathErr == nil {
			enc = s.detectEncoding(abs)
		}
		if info, err := os.Lstat(abs); pathErr == nil && err == nil && info.Mode().IsRegular() {
			m := uint32(info.Mode().Perm())
			mode = &m
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil || (expectedTurn != nil && s.cur.Turn != *expectedTurn) || s.seen[identity] {
		return
	}
	var content *string
	if ch.Kind != diff.Create { // create == file didn't exist → leave nil (restore deletes)
		old := ch.OldText
		content = &old
	}
	snap := FileSnap{Path: ch.Path, Content: content, Encoding: enc, Mode: mode}
	if existing, ok := sameSeenFile(identityInfo, s.seenFiles); ok {
		// A second hard-link name denotes the same pre-edit inode. Preserve the
		// earliest bytes for every alias so later snapshots cannot restore an
		// intermediate state through another name.
		snap = existing.snap
		snap.Path = ch.Path
	}
	s.seen[identity] = true
	s.seenFiles = append(s.seenFiles, seenFile{identity: identity, info: identityInfo, snap: snap})
	s.cur.Files = append(s.cur.Files, snap)
	s.persist(s.cur)
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

func (s *Store) persist(c *Checkpoint) {
	if s.dir == "" {
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	if err := fileutil.AtomicWriteFile(filepath.Join(s.dir, fmt.Sprintf("turn-%d.json", c.Turn)), b, 0o644); err != nil {
		slog.Warn("checkpoint: persist failed", "turn", c.Turn, "err", err)
	}
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
	return writeFile(filepath.Join(s.dir, checkpointStateFilename), b, 0o644)
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
	for turn := range deleteTurns {
		if err := os.Remove(filepath.Join(dir, fmt.Sprintf("turn-%d.json", turn))); err != nil && !os.IsNotExist(err) {
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
			abs, pathErr := safePath(s.root, f.Path)
			identity := canonicalPathIdentity(f.Path)
			var info os.FileInfo
			if pathErr == nil {
				identity = canonicalPathIdentity(abs)
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
	s.mu.Unlock()
	sort.Slice(earliest, func(i, j int) bool { return earliest[i].snap.Path < earliest[j].snap.Path })
	if writeFile == nil {
		writeFile = fileutil.AtomicWriteFile
	}

	type diskState struct {
		exists bool
		data   []byte
		mode   os.FileMode
	}
	type restoreOp struct {
		path   string
		abs    string
		snap   FileSnap
		before diskState
	}
	ops := make([]restoreOp, 0, len(earliest))
	for _, entry := range earliest {
		p := entry.snap.Path
		abs, pathErr := safeRestorePath(root, p)
		if pathErr != nil {
			return nil, nil, pathErr
		}
		before := diskState{}
		info, statErr := os.Lstat(abs)
		switch {
		case statErr == nil:
			if !info.Mode().IsRegular() {
				return nil, nil, fmt.Errorf("checkpoint path %q is not a regular file", p)
			}
			before.exists = true
			before.mode = info.Mode().Perm()
			before.data, statErr = os.ReadFile(abs)
			if statErr != nil {
				return nil, nil, fmt.Errorf("checkpoint preflight read %q: %w", p, statErr)
			}
		case os.IsNotExist(statErr):
		default:
			return nil, nil, fmt.Errorf("checkpoint preflight stat %q: %w", p, statErr)
		}
		ops = append(ops, restoreOp{path: p, abs: abs, snap: entry.snap, before: before})
	}

	rollback := func(applied []restoreOp) error {
		var rollbackErr error
		for i := len(applied) - 1; i >= 0; i-- {
			op := applied[i]
			if op.before.exists {
				if e := fileutil.AtomicWriteFile(op.abs, op.before.data, op.before.mode); e != nil {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore %q: %w", op.path, e))
				}
				continue
			}
			if e := os.Remove(op.abs); e != nil && !os.IsNotExist(e) {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove %q: %w", op.path, e))
			}
		}
		return rollbackErr
	}

	applied := make([]restoreOp, 0, len(ops))
	for _, op := range ops {
		// Recheck immediately before applying to catch a path component swapped
		// for a symlink after preflight.
		abs, pathErr := safeRestorePath(root, op.path)
		if pathErr != nil || abs != op.abs {
			if pathErr == nil {
				pathErr = fmt.Errorf("checkpoint path %q changed during restore", op.path)
			}
			if rbErr := rollback(applied); rbErr != nil {
				pathErr = errors.Join(pathErr, fmt.Errorf("rollback: %w", rbErr))
			}
			return nil, nil, pathErr
		}
		var applyErr error
		if op.snap.Content == nil {
			if op.before.exists {
				applyErr = os.Remove(op.abs)
			}
		} else {
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
			applyErr = writeFile(op.abs, fileenc.Encode(*op.snap.Content, enc), mode)
		}
		if applyErr != nil {
			// A writer may report failure after touching the destination (for
			// example a degraded cross-device copy). Restore the current op as
			// well as every earlier one so the transaction returns to preflight.
			if rbErr := rollback(append(applied, op)); rbErr != nil {
				applyErr = errors.Join(applyErr, fmt.Errorf("rollback: %w", rbErr))
			}
			return nil, nil, fmt.Errorf("checkpoint restore %q: %w", op.path, applyErr)
		}
		applied = append(applied, op)
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

func detectCurrentEncoding(path string) *fileenc.Kind {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	enc, _ := fileenc.Detect(b)
	return &enc
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

// safeRestorePath adds filesystem-aware confinement to safePath. Existing
// symlink components are rejected, and resolved ancestors must remain under the
// resolved workspace root. This also catches Windows junction/reparse escapes
// that filepath.Rel alone cannot see.
func safeRestorePath(root, p string) (string, error) {
	abs, err := safePath(root, p)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("checkpoint workspace %q cannot be resolved: %w", root, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("checkpoint path %q escapes workspace %q", p, root)
	}
	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		info, statErr := os.Lstat(cur)
		if os.IsNotExist(statErr) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("checkpoint path %q cannot be inspected: %w", p, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("checkpoint path %q crosses symlink %q", p, cur)
		}
	}
	ancestor := abs
	for {
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			break
		} else if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("checkpoint path %q cannot be inspected: %w", p, statErr)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("checkpoint path %q has no existing workspace ancestor", p)
		}
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("checkpoint path %q cannot be resolved: %w", p, err)
	}
	resolvedRel, err := filepath.Rel(rootResolved, resolved)
	if err != nil || (!filepath.IsLocal(resolvedRel) && resolvedRel != ".") {
		return "", fmt.Errorf("checkpoint path %q resolves outside workspace %q", p, root)
	}
	return abs, nil
}
