package checkpoint

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"reames-agent/internal/diff"
	"reames-agent/internal/fileutil"
	fileenc "reames-agent/internal/fileutil/encoding"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func readBytes(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Two turns edit a.txt and create b.txt; rewinding restores each file to its
// state at the start of the chosen turn (b.txt being deleted when it post-dates it).
func TestRestoreToStartOfTurn(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "sub", "b.txt")
	write(t, a, "v0")
	s := New("", root)

	s.Begin(0, "first", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v0"})
	write(t, a, "v1") // the edit turn 0 made

	s.Begin(1, "second", 2)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v1"})
	s.Snapshot(diff.Change{Path: b, Kind: diff.Create})
	write(t, a, "v2")
	write(t, b, "new")

	// Rewind to the start of turn 1: a back to v1, b gone.
	if _, _, err := s.RestoreCode(1); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "v1" {
		t.Fatalf("a = %q, want v1", got)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Fatalf("b should have been deleted, stat err=%v", err)
	}
}

func TestRestoreToTurnZero(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	write(t, a, "v0")
	s := New("", root)
	s.Begin(0, "first", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v0"})
	write(t, a, "v1")
	s.Begin(1, "second", 2)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v1"})
	write(t, a, "v2")

	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "v0" {
		t.Fatalf("a = %q, want v0 (earliest snapshot)", got)
	}
}

func TestRestorePreservesGB18030Encoding(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "gbk.txt")
	original := "\u4f60\u597d\n\u65e7\u884c\n"
	edited := "\u4f60\u597d\n\u65b0\u884c\n"
	originalRaw := fileenc.Encode(original, fileenc.GB18030)
	if err := os.WriteFile(a, originalRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	s := New("", root)
	s.Begin(0, "edit gbk", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: original})
	if err := os.WriteFile(a, fileenc.Encode(edited, fileenc.GB18030), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	gotRaw := readBytes(t, a)
	if utf8.Valid(gotRaw) {
		t.Fatalf("restored GB18030 file became valid UTF-8 bytes: % x", gotRaw)
	}
	if !bytes.Equal(gotRaw, originalRaw) {
		t.Fatalf("restored bytes = % x, want original GB18030 bytes % x", gotRaw, originalRaw)
	}
}

func TestRestorePreservesGB18030EncodingAfterPersistence(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "sess.ckpt")
	a := filepath.Join(root, "gbk.txt")
	original := "\u4f60\u597d\n\u65e7\u884c\n"
	edited := "\u4f60\u597d\n\u65b0\u884c\n"
	originalRaw := fileenc.Encode(original, fileenc.GB18030)
	if err := os.WriteFile(a, originalRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(dir, root)
	s.Begin(0, "edit gbk", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: original})

	resumed := New(dir, root)
	if err := os.WriteFile(a, fileenc.Encode(edited, fileenc.GB18030), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resumed.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	if gotRaw := readBytes(t, a); !bytes.Equal(gotRaw, originalRaw) {
		t.Fatalf("restored bytes after persistence = % x, want original GB18030 bytes % x", gotRaw, originalRaw)
	}
}

func TestRestoreLegacySnapshotFallsBackToCurrentEncoding(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "sess.ckpt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "gbk.txt")
	original := "\u4f60\u597d\n\u65e7\u884c\n"
	edited := "\u4f60\u597d\n\u65b0\u884c\n"
	originalRaw := fileenc.Encode(original, fileenc.GB18030)
	if err := os.WriteFile(a, fileenc.Encode(edited, fileenc.GB18030), 0o644); err != nil {
		t.Fatal(err)
	}

	legacy := Checkpoint{
		Turn:     0,
		Time:     time.Now(),
		Prompt:   "legacy",
		MsgIndex: 0,
		Files: []FileSnap{{
			Path:    a,
			Content: &original,
		}},
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "turn-0.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	resumed := New(dir, root)
	if _, _, err := resumed.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	if gotRaw := readBytes(t, a); !bytes.Equal(gotRaw, originalRaw) {
		t.Fatalf("legacy restored bytes = % x, want original GB18030 bytes % x", gotRaw, originalRaw)
	}
}

func TestSnapshotDedupsFirstTouchWins(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	write(t, a, "orig")
	s := New("", root)
	s.Begin(0, "p", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "orig"})
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "edited-once"}) // ignored
	write(t, a, "edited-twice")
	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a); got != "orig" {
		t.Fatalf("a = %q, want orig (first snapshot wins)", got)
	}
}

func TestSnapshotDedupsLexicalPathAliases(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	alias := root + string(filepath.Separator) + "." + string(filepath.Separator) + "a.txt"
	write(t, path, "orig")
	s := New("", root)
	s.Begin(0, "p", 0)
	s.Snapshot(diff.Change{Path: alias, Kind: diff.Modify, OldText: "orig"})
	write(t, path, "edited-once")
	s.Snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "edited-once"})
	write(t, path, "edited-twice")

	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "orig" {
		t.Fatalf("path alias restored %q, want earliest content", got)
	}
}

func TestSnapshotHardLinkAliasesShareEarliestContent(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	write(t, first, "orig")
	if err := os.Link(first, second); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	s := New("", root)
	s.Begin(0, "p", 0)
	s.Snapshot(diff.Change{Path: first, Kind: diff.Modify, OldText: "orig"})
	write(t, first, "edited-once")
	s.Snapshot(diff.Change{Path: second, Kind: diff.Modify, OldText: "edited-once"})
	write(t, second, "edited-twice")

	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	if got := read(t, first); got != "orig" {
		t.Fatalf("first hard-link alias = %q, want earliest content", got)
	}
	if got := read(t, second); got != "orig" {
		t.Fatalf("second hard-link alias = %q, want earliest content", got)
	}
}

func TestRestoreRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.txt")
	write(t, outside, "keep")
	s := New("", root)
	s.Begin(0, "p", 0)
	s.Snapshot(diff.Change{Path: outside, Kind: diff.Modify, OldText: "hacked"})
	if _, _, err := s.RestoreCode(0); err == nil {
		t.Fatal("RestoreCode should reject a path outside the workspace")
	}
	if got := read(t, outside); got != "keep" {
		t.Fatalf("outside file was modified: %q", got)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "sess.ckpt")
	a := filepath.Join(root, "a.txt")

	s := New(dir, root)
	s.Begin(0, "hello", 1)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v0"})
	s.Begin(1, "world", 5)

	// A fresh store over the same dir must see both turns and their boundaries.
	s2 := New(dir, root)
	metas := s2.List()
	if len(metas) != 2 {
		t.Fatalf("loaded %d checkpoints, want 2", len(metas))
	}
	if metas[0].Prompt != "hello" || metas[1].Prompt != "world" {
		t.Fatalf("prompts = %q, %q", metas[0].Prompt, metas[1].Prompt)
	}
	// Boundaries must survive the round-trip so a resumed session can rewind/fork.
	b := s2.Bounds()
	if b[0] != 1 || b[1] != 5 {
		t.Fatalf("bounds = %v, want {0:1, 1:5}", b)
	}
	if s2.NextTurn() != 2 {
		t.Fatalf("NextTurn = %d, want 2", s2.NextTurn())
	}
}

func TestListExposesCurrentTurnFiles(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	write(t, a, "v0")
	s := New("", root)
	s.Begin(0, "edit current", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v0"})

	metas := s.List()
	if len(metas) != 1 {
		t.Fatalf("metas = %d, want 1", len(metas))
	}
	if len(metas[0].Paths) != 1 || metas[0].Paths[0] != a {
		t.Fatalf("current turn paths = %#v, want [%q]", metas[0].Paths, a)
	}
}

func TestTruncateFromDropsFutureCheckpointsAndFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "sess.ckpt")
	a := filepath.Join(root, "a.txt")
	write(t, a, "v0")
	s := New(dir, root)
	s.Begin(0, "first", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v0"})
	s.Begin(1, "second", 2)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "v1"})
	s.Begin(2, "third", 4)

	s.TruncateFrom(1)

	metas := s.List()
	if len(metas) != 1 || metas[0].Turn != 0 {
		t.Fatalf("metas after truncate = %+v, want only turn 0", metas)
	}
	if s.NextTurn() != 3 {
		t.Fatalf("NextTurn after truncate = %d, want monotonic watermark 3", s.NextTurn())
	}
	if _, err := os.Stat(filepath.Join(dir, "turn-1.json")); !os.IsNotExist(err) {
		t.Fatalf("turn-1 checkpoint should be deleted, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "turn-2.json")); !os.IsNotExist(err) {
		t.Fatalf("turn-2 checkpoint should be deleted, stat err=%v", err)
	}
	reloaded := New(dir, root)
	if got := reloaded.List(); len(got) != 1 || got[0].Turn != 0 {
		t.Fatalf("reloaded metas after truncate = %+v, want only turn 0", got)
	}
	if reloaded.NextTurn() != 3 {
		t.Fatalf("reloaded NextTurn = %d, want monotonic watermark 3", reloaded.NextTurn())
	}
}

func TestTruncateTombstoneFiltersUndeletedRecordsAfterRestart(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "session.ckpt")
	s := New(dir, root)
	s.Begin(0, "first", 0)
	s.Begin(1, "second", 2)
	retiredPath := filepath.Join(dir, "turn-1.json")
	retiredBytes, err := os.ReadFile(retiredPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TruncateFrom(1); err != nil {
		t.Fatal(err)
	}
	// Model a failed physical delete or a stale directory view after the durable
	// truncate state landed. Reload must not resurrect the retired checkpoint.
	if err := os.WriteFile(retiredPath, retiredBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	reloaded := New(dir, root)
	if got := reloaded.List(); len(got) != 1 || got[0].Turn != 0 {
		t.Fatalf("tombstoned checkpoint resurrected: %+v", got)
	}
	if reloaded.NextTurn() != 2 {
		t.Fatalf("NextTurn after tombstoned reload = %d, want 2", reloaded.NextTurn())
	}
	reloaded.Begin(2, "new future", 1)
	if got := reloaded.List(); len(got) != 2 || got[1].Turn != 2 {
		t.Fatalf("new monotonic checkpoint hidden by tombstone: %+v", got)
	}
}

func TestCorruptTruncateManifestFailsClosedAndHealsOnNextTurn(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "session.ckpt")
	s := New(dir, root)
	s.Begin(0, "first", 0)
	s.Begin(1, "second", 2)
	retiredPath := filepath.Join(dir, "turn-1.json")
	retiredBytes, err := os.ReadFile(retiredPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TruncateFrom(1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retiredPath, retiredBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, checkpointStateFilename), []byte(`{"version":`), 0o644); err != nil {
		t.Fatal(err)
	}

	reloaded := New(dir, root)
	if got := reloaded.List(); len(got) != 0 {
		t.Fatalf("corrupt manifest admitted untrusted checkpoints: %+v", got)
	}
	if reloaded.NextTurn() != 2 {
		t.Fatalf("NextTurn with corrupt manifest = %d, want filename watermark 2", reloaded.NextTurn())
	}
	reloaded.Begin(2, "new trusted turn", 1)
	healed := New(dir, root)
	if got := healed.List(); len(got) != 1 || got[0].Turn != 2 {
		t.Fatalf("healed store checkpoints = %+v, want only new turn 2", got)
	}
	if healed.NextTurn() != 3 {
		t.Fatalf("healed NextTurn = %d, want 3", healed.NextTurn())
	}
}

func TestTruncateManifestFailureLeavesStoreIntact(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "session.ckpt")
	s := New(dir, root)
	s.Begin(0, "first", 0)
	s.Begin(1, "second", 2)
	s.stateWrite = func(string, []byte, os.FileMode) error { return errors.New("injected manifest failure") }

	if err := s.TruncateFrom(1); err == nil || !strings.Contains(err.Error(), "injected manifest failure") {
		t.Fatalf("TruncateFrom error = %v", err)
	}
	if got := s.List(); len(got) != 2 {
		t.Fatalf("failed truncate mutated live store: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "turn-1.json")); err != nil {
		t.Fatalf("failed truncate removed checkpoint: %v", err)
	}
}

func TestLoadRejectsInvalidCheckpointRecords(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "session.ckpt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	records := map[string]Checkpoint{
		"turn-0.json": {Turn: -1, MsgIndex: 0},
		"turn-1.json": {Turn: 0, MsgIndex: 0},
		"turn-2.json": {Turn: 2, MsgIndex: -1},
		"turn-3.json": {Turn: 3, MsgIndex: 1, Prompt: "valid"},
	}
	for name, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := New(dir, root)
	if got := s.List(); len(got) != 1 || got[0].Turn != 3 {
		t.Fatalf("loaded invalid checkpoint records: %+v", got)
	}
	if s.NextTurn() != 4 {
		t.Fatalf("NextTurn = %d, want filename watermark 4", s.NextTurn())
	}
}

func BenchmarkRestoreGB18030Encoding(b *testing.B) {
	root := b.TempDir()
	a := filepath.Join(root, "gbk.txt")
	original := strings.Repeat("\u4f60\u597d\u4e16\u754c\n\u65e7\u884c\n", 8192)
	edited := strings.Repeat("\u4f60\u597d\u4e16\u754c\n\u65b0\u884c\n", 8192)
	originalRaw := fileenc.Encode(original, fileenc.GB18030)
	editedRaw := fileenc.Encode(edited, fileenc.GB18030)
	if err := os.WriteFile(a, originalRaw, 0o644); err != nil {
		b.Fatal(err)
	}

	s := New("", root)
	s.Begin(0, "edit gbk", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: original})

	b.SetBytes(int64(len(originalRaw)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := os.WriteFile(a, editedRaw, 0o644); err != nil {
			b.Fatal(err)
		}
		if _, _, err := s.RestoreCode(0); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLazyDirectoryCreation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "lazy-sess.ckpt")

	s := New(dir, root)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory should not exist yet: %v", err)
	}

	s.Begin(0, "lazy", 0)

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should now exist: %v", err)
	}
	turnPath := filepath.Join(dir, "turn-0.json")
	if _, err := os.Stat(turnPath); err != nil {
		t.Fatalf("turn file should now exist: %v", err)
	}
}

func TestRuntimeProjectionPersistsWithCheckpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session.ckpt")
	s := New(dir, t.TempDir())
	runtimeState := json.RawMessage(`{"version":2,"goal":"ship","status":"running","revision":3}`)
	s.Begin(0, "ship", 2, runtimeState)

	reloaded := New(dir, s.root)
	got, ok := reloaded.Runtime(0)
	if !ok || !bytes.Equal(got, runtimeState) {
		t.Fatalf("runtime projection = %s ok=%v, want %s", got, ok, runtimeState)
	}
}

func TestRestoreRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	target := filepath.Join(link, "outside.txt")
	outsideTarget := filepath.Join(outside, "outside.txt")
	write(t, outsideTarget, "original")
	s := New("", root)
	s.Begin(0, "edit", 0)
	s.Snapshot(diff.Change{Path: target, Kind: diff.Modify, OldText: "original"})
	write(t, outsideTarget, "edited")

	if _, _, err := s.RestoreCode(0); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("RestoreCode symlink error = %v", err)
	}
	if got := read(t, outsideTarget); got != "edited" {
		t.Fatalf("outside target changed through symlink: %q", got)
	}
}

func TestRestoreRollsBackEarlierFilesOnWriteFailure(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b := filepath.Join(root, "b.txt")
	write(t, a, "a-before")
	write(t, b, "b-before")
	s := New("", root)
	s.Begin(0, "edit both", 0)
	s.Snapshot(diff.Change{Path: a, Kind: diff.Modify, OldText: "a-before"})
	s.Snapshot(diff.Change{Path: b, Kind: diff.Modify, OldText: "b-before"})
	write(t, a, "a-edited")
	write(t, b, "b-edited")
	s.restoreWrite = func(path string, data []byte, mode os.FileMode) error {
		if filepath.Base(path) == "b.txt" {
			return errors.New("injected second write failure")
		}
		return fileutil.AtomicWriteFile(path, data, mode)
	}

	if _, _, err := s.RestoreCode(0); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("RestoreCode error = %v", err)
	}
	if got := read(t, a); got != "a-edited" {
		t.Fatalf("first file was not rolled back: %q", got)
	}
	if got := read(t, b); got != "b-edited" {
		t.Fatalf("failed file changed: %q", got)
	}
}

func TestRestoreRollsBackFailingFileAfterPartialWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	write(t, path, "before")
	s := New("", root)
	s.Begin(0, "edit", 0)
	s.Snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "before"})
	write(t, path, "edited")
	s.restoreWrite = func(path string, _ []byte, mode os.FileMode) error {
		if err := os.WriteFile(path, []byte("partial"), mode); err != nil {
			return err
		}
		return errors.New("injected failure after partial write")
	}

	if _, _, err := s.RestoreCode(0); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("RestoreCode error = %v", err)
	}
	if got := read(t, path); got != "edited" {
		t.Fatalf("partially written file was not rolled back: %q", got)
	}
}

func TestRestorePreservesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix executable bits")
	}
	root := t.TempDir()
	path := filepath.Join(root, "script.sh")
	if err := os.WriteFile(path, []byte("before"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New("", root)
	s.Begin(0, "edit script", 0)
	s.Snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "before"})
	if err := os.WriteFile(path, []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("restored mode = %o, want 755", info.Mode().Perm())
	}
}

func TestRestoreRelativePathPreservesExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix executable bits")
	}
	root := t.TempDir()
	const relative = "script.sh"
	path := filepath.Join(root, relative)
	if err := os.WriteFile(path, []byte("before"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New("", root)
	s.Begin(0, "edit relative script", 0)
	s.Snapshot(diff.Change{Path: relative, Kind: diff.Modify, OldText: "before"})
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("restored relative-path mode = %o, want 755", info.Mode().Perm())
	}
}

func TestRestorePreservesZeroMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix permission bits")
	}
	root := t.TempDir()
	path := filepath.Join(root, "locked.txt")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	s := New("", root)
	s.Begin(0, "edit locked file", 0)
	s.Snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "before"})
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RestoreCode(0); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0 {
		t.Fatalf("restored mode = %o, want 000", info.Mode().Perm())
	}
}
