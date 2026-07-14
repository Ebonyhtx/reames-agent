package builtin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/diff"
)

func TestApplyPatchSchema(t *testing.T) {
	ap := applyPatch{}
	if ap.Name() != "apply_patch" {
		t.Fatalf("name: %s", ap.Name())
	}
	if ap.ReadOnly() {
		t.Fatal("apply_patch should not be read-only")
	}
}

func TestParseUnifiedDiffEmpty(t *testing.T) {
	_, err := parseUnifiedDiff("")
	if err == nil {
		t.Fatal("expected error for empty diff")
	}
}

func TestParseUnifiedDiffNoHeader(t *testing.T) {
	_, err := parseUnifiedDiff("just some text\nno diff here\n")
	if err == nil {
		t.Fatal("expected error for no diff headers")
	}
}

func TestParseUnifiedDiffValid(t *testing.T) {
	diff := `--- a/hello.txt
+++ b/hello.txt
@@ -1,1 +1,1 @@
-Hello
+World
`
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "hello.txt" {
		t.Fatalf("path: %s", files[0].Path)
	}
	if files[0].Added != 1 || files[0].Removed != 1 {
		t.Fatalf("added=%d removed=%d", files[0].Added, files[0].Removed)
	}
}

func TestParseUnifiedDiffRejectsImplicitRename(t *testing.T) {
	patch := "--- a/old.txt\n+++ b/new.txt\n@@ -1 +1 @@\n-old\n+new\n"
	if _, err := parseUnifiedDiff(patch); err == nil || !strings.Contains(err.Error(), "use move_file") {
		t.Fatalf("rename error = %v", err)
	}
}

func TestApplyPatchRootedTransactionCreateUpdateDelete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "update.txt"), []byte("old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "delete.txt"), []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- a/update.txt\n+++ b/update.txt\n@@ -1 +1 @@\n-old\n+new\n" +
		"--- /dev/null\n+++ b/create.txt\n@@ -0,0 +1 @@\n+created\n" +
		"--- a/delete.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-gone\n"
	ap := applyPatch{workDir: dir, roots: realRoots([]string{dir})}
	changes, err := ap.PreviewChanges(argsJSON(t, map[string]any{"patch": patch}))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].Kind != diff.Modify || changes[1].Kind != diff.Create || changes[2].Kind != diff.Delete {
		t.Fatalf("preview changes = %#v", changes)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "update.txt")); string(got) != "old\n" {
		t.Fatalf("preview mutated update.txt: %q", got)
	}

	out, err := ap.Execute(context.Background(), argsJSON(t, map[string]any{"patch": patch}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "applied: 3 file(s), 3 hunk(s)") {
		t.Fatalf("summary = %q", out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "update.txt")); string(got) != "new\n" {
		t.Fatalf("update = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "create.txt")); string(got) != "created\n" {
		t.Fatalf("create = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete target still exists or stat failed: %v", err)
	}
}

func TestApplyPatchRejectsContextMismatchBeforeAnyWrite(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- a/first.txt\n+++ b/first.txt\n@@ -1 +1 @@\n-one\n+ONE\n" +
		"--- a/second.txt\n+++ b/second.txt\n@@ -1 +1 @@\n-stale\n+TWO\n"
	ap := applyPatch{workDir: dir, roots: realRoots([]string{dir})}
	if _, err := ap.Execute(context.Background(), argsJSON(t, map[string]any{"patch": patch})); err == nil || !strings.Contains(err.Error(), "removal mismatch") {
		t.Fatalf("error = %v, want removal mismatch", err)
	}
	if got, _ := os.ReadFile(first); string(got) != "one\n" {
		t.Fatalf("first file changed during failed preflight: %q", got)
	}
}

func TestApplyPatchRollsBackEarlierFileWhenLaterWriteFails(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writes := 0
	ap := applyPatch{workDir: dir, roots: realRoots([]string{dir})}
	ap.writePlan = func(plan *patchPlan) error {
		writes++
		if writes == 2 {
			return errors.New("injected write failure")
		}
		return writePatchPlan(plan)
	}
	patch := "--- a/first.txt\n+++ b/first.txt\n@@ -1 +1 @@\n-one\n+ONE\n" +
		"--- a/second.txt\n+++ b/second.txt\n@@ -1 +1 @@\n-two\n+TWO\n"
	if _, err := ap.Execute(context.Background(), argsJSON(t, map[string]any{"patch": patch})); err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error = %v, want rollback failure report", err)
	}
	if got, _ := os.ReadFile(first); string(got) != "one\n" {
		t.Fatalf("first file was not rolled back: %q", got)
	}
	if got, _ := os.ReadFile(second); string(got) != "two\n" {
		t.Fatalf("second file changed: %q", got)
	}
}

func TestApplyPatchRejectsWorkspaceAndSessionEscapes(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	ap := applyPatch{workDir: dir, roots: realRoots([]string{dir})}
	absolutePatch := "--- /dev/null\n+++ " + filepath.ToSlash(filepath.Join(outside, "escape.txt")) + "\n@@ -0,0 +1 @@\n+no\n"
	if _, err := ap.Execute(context.Background(), argsJSON(t, map[string]any{"patch": absolutePatch})); err == nil || !strings.Contains(err.Error(), "outside the writable roots") {
		t.Fatalf("outside error = %v", err)
	}

	stateRoot := filepath.Join(dir, "state")
	sessionDir := filepath.Join(stateRoot, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ap = applyPatch{workDir: dir, roots: realRoots([]string{dir}), guard: NewSessionDataGuard(stateRoot, nil)}
	sessionPatch := "--- /dev/null\n+++ state/sessions/tamper.jsonl\n@@ -0,0 +1 @@\n+no\n"
	if _, err := ap.Execute(context.Background(), argsJSON(t, map[string]any{"patch": sessionPatch})); err == nil || !strings.Contains(err.Error(), "session/state data") {
		t.Fatalf("session error = %v", err)
	}
}

func TestApplyPatchDryRunValidatesWithoutCheckpointChangesOrWrites(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ap := applyPatch{workDir: dir, roots: realRoots([]string{dir})}
	args := argsJSON(t, map[string]any{"patch": "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n", "dry_run": true})
	if changes, err := ap.PreviewChanges(args); err != nil || len(changes) != 0 {
		t.Fatalf("dry-run changes = %v, err = %v", changes, err)
	}
	out, err := ap.Execute(context.Background(), args)
	if err != nil || !strings.Contains(out, "preview") {
		t.Fatalf("dry-run output = %q, err = %v", out, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "old\n" {
		t.Fatalf("dry-run wrote file: %q", got)
	}
}
