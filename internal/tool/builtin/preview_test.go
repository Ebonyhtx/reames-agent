package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/diff"
	"reames-agent/internal/tool"
)

// TestWritersImplementPreviewer locks in that every file-writer exposes the
// optional Previewer capability the front-end type-asserts on. A new writer
// that forgets Preview fails here.
func TestWritersImplementPreviewer(t *testing.T) {
	for _, tl := range []tool.Tool{writeFile{}, editFile{}, multiEdit{}, notebookEdit{}, deleteRange{}, deleteSymbol{}} {
		if _, ok := tl.(tool.Previewer); !ok {
			t.Errorf("%s does not implement tool.Previewer", tl.Name())
		}
	}
	for _, tl := range []tool.Tool{moveFile{}, applyPatch{}} {
		if _, ok := tl.(tool.MultiPreviewer); !ok {
			t.Errorf("%s does not implement tool.MultiPreviewer", tl.Name())
		}
	}
}

func TestMovePreviewAggregatesSourceAndDestinationDiffs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "source.txt"), []byte("move me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mv := moveFile{workDir: dir, roots: realRoots([]string{dir})}
	args := argsJSON(t, map[string]any{"source_path": "source.txt", "destination_path": "destination.txt"})
	changes, ok, err := tool.PreviewFileChanges(mv, args)
	if err != nil || !ok || len(changes) != 2 {
		t.Fatalf("changes = %v, ok = %v, err = %v", changes, ok, err)
	}
	if changes[0].Kind != diff.Delete || changes[1].Kind != diff.Create {
		t.Fatalf("move kinds = %s, %s", changes[0].Kind, changes[1].Kind)
	}
	aggregated, ok := tool.PreviewChange(mv, args)
	if !ok || aggregated.Added != 1 || aggregated.Removed != 1 || !strings.Contains(aggregated.Diff, "source.txt") || !strings.Contains(aggregated.Diff, "destination.txt") {
		t.Fatalf("aggregate = %+v, ok = %v", aggregated, ok)
	}
}

// TestPreviewMatchesExecute is the anti-drift guarantee: for each writer,
// Preview's NewText must equal the bytes Execute actually persists. It runs
// both against an identical starting file so a future change to an Execute body
// that isn't mirrored into Preview fails this test instead of silently making
// the approval card lie.
func TestPreviewMatchesExecute(t *testing.T) {
	cases := []struct {
		name string
		tool tool.Tool
		// seed is the file's content before the call ("" means create fresh).
		seed string
		args func(path string) map[string]any
	}{
		{
			name: "write_file create",
			tool: writeFile{},
			seed: "",
			args: func(p string) map[string]any {
				return map[string]any{"path": p, "content": "fresh\nfile\n"}
			},
		},
		{
			name: "write_file overwrite",
			tool: writeFile{},
			seed: "old content\n",
			args: func(p string) map[string]any {
				return map[string]any{"path": p, "content": "new content\n"}
			},
		},
		{
			name: "edit_file",
			tool: editFile{},
			seed: "hello world\n",
			args: func(p string) map[string]any {
				return map[string]any{"path": p, "old_string": "world", "new_string": "reames-agent"}
			},
		},
		{
			name: "edit_file fuzzy",
			tool: editFile{},
			seed: "alpha   \nbeta   \n",
			args: func(p string) map[string]any {
				return map[string]any{"path": p, "old_string": "alpha\nbeta", "new_string": "ALPHA\nBETA"}
			},
		},
		{
			name: "multi_edit",
			tool: multiEdit{},
			seed: "package old\n\nfunc old() {\n\told()\n}\n",
			args: func(p string) map[string]any {
				return map[string]any{"path": p, "edits": []map[string]any{
					{"old_string": "package old", "new_string": "package new"},
					{"old_string": "old", "new_string": "reames-agent", "replace_all": true},
				}}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Preview against one copy.
			pf := filepath.Join(t.TempDir(), "f.txt")
			if tc.seed != "" {
				os.WriteFile(pf, []byte(tc.seed), 0o644)
			}
			prev, ok := tc.tool.(tool.Previewer)
			if !ok {
				t.Fatalf("%s not a Previewer", tc.tool.Name())
			}
			change, err := prev.Preview(argsJSON(t, tc.args(pf)))
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}
			// Preview must not have touched disk.
			if tc.seed == "" {
				if _, statErr := os.Stat(pf); statErr == nil {
					t.Fatal("Preview created the file (should be side-effect free)")
				}
			} else if b, _ := os.ReadFile(pf); string(b) != tc.seed {
				t.Fatalf("Preview mutated the file: %q", b)
			}

			// Execute against a second identical copy.
			ef := filepath.Join(t.TempDir(), "f.txt")
			if tc.seed != "" {
				os.WriteFile(ef, []byte(tc.seed), 0o644)
			}
			if _, err := tc.tool.Execute(context.Background(), argsJSON(t, tc.args(ef))); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			got, _ := os.ReadFile(ef)
			if string(got) != change.NewText {
				t.Fatalf("Preview.NewText != Execute result\n preview: %q\n execute: %q", change.NewText, got)
			}
			if change.OldText != tc.seed {
				t.Fatalf("Preview.OldText = %q, want seed %q", change.OldText, tc.seed)
			}
		})
	}
}

// TestPreviewKindAndTally checks the metadata a UI shows: create vs modify and
// the +N/-M tallies.
func TestPreviewKindAndTally(t *testing.T) {
	// write_file to a nonexistent path is a create.
	nf := filepath.Join(t.TempDir(), "new.txt")
	c, err := writeFile{}.Preview(argsJSON(t, map[string]any{"path": nf, "content": "a\nb\nc\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != diff.Create {
		t.Errorf("kind = %q, want create", c.Kind)
	}
	if c.Added != 3 || c.Removed != 0 {
		t.Errorf("+%d/-%d, want +3/-0", c.Added, c.Removed)
	}

	// edit_file on an existing file is a modify with balanced tallies.
	ef := filepath.Join(t.TempDir(), "e.txt")
	os.WriteFile(ef, []byte("one\ntwo\nthree\n"), 0o644)
	c, err = editFile{}.Preview(argsJSON(t, map[string]any{"path": ef, "old_string": "two", "new_string": "TWO"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != diff.Modify {
		t.Errorf("kind = %q, want modify", c.Kind)
	}
	if c.Added != 1 || c.Removed != 1 {
		t.Errorf("+%d/-%d, want +1/-1", c.Added, c.Removed)
	}
}

// TestPreviewMirrorsErrors confirms an unworkable call fails in Preview the
// same way it would in Execute — so a UI never previews an impossible change.
func TestPreviewMirrorsErrors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(f, []byte("x x x"), 0o644)

	if _, err := (editFile{}).Preview(argsJSON(t, map[string]any{"path": f, "old_string": "x", "new_string": "y"})); err == nil {
		t.Error("expected not-unique error from Preview")
	}
	missing := filepath.Join(t.TempDir(), "nope.txt")
	if _, err := (editFile{}).Preview(argsJSON(t, map[string]any{"path": missing, "old_string": "a", "new_string": "b"})); err == nil {
		t.Error("expected read error for missing file")
	}
}
