package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	repo := t.TempDir()
	gitTest(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	return repo
}

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmdArgs := []string{"-c", "core.longpaths=true", "-c", "user.name=Reames Test", "-c", "user.email=test@example.invalid", "-C", dir}
	cmd := exec.Command("git", append(cmdArgs, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCreateSealApplyRollbackAndReject(t *testing.T) {
	repo := initRepo(t)
	managed := t.TempDir()
	a, err := Create(context.Background(), repo, managed, "sa_test")
	if err != nil {
		t.Fatal(err)
	}
	if a.WorkspaceRoot == repo || !IsManagedPath(a.WorktreeRoot, managed) {
		t.Fatalf("assignment = %+v", a)
	}
	if err := os.WriteFile(filepath.Join(a.WorkspaceRoot, "README.md"), []byte("delivery\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.WorkspaceRoot, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit, err := Seal(context.Background(), a, "sa_test")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := SnapshotDelivery(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Dirty || snapshot.Head != commit || len(snapshot.Changes) != 2 || snapshot.PatchDigest == "" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	tx, err := Apply(context.Background(), a, commit)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != "delivery\n" {
		t.Fatalf("applied README = %q err=%v", got, err)
	}
	if tx.Kind != "apply" || tx.StatusAfter == tx.StatusBefore {
		t.Fatalf("transaction = %+v", tx)
	}
	if _, err := Rollback(context.Background(), a, tx); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != "base\n" {
		t.Fatalf("rolled back README = %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new.txt survived rollback: %v", err)
	}
	if err := Remove(context.Background(), a, managed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.WorktreeRoot); !os.IsNotExist(err) {
		t.Fatalf("worktree survived reject: %v", err)
	}
}

func TestMergeAndRollbackRefusesDrift(t *testing.T) {
	repo := initRepo(t)
	managed := t.TempDir()
	a, err := Create(context.Background(), repo, managed, "sa_merge")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Remove(context.Background(), a, managed) }()
	if err := os.WriteFile(filepath.Join(a.WorkspaceRoot, "README.md"), []byte("merged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit, err := Seal(context.Background(), a, "sa_merge")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := Merge(context.Background(), a, commit)
	if err != nil {
		t.Fatal(err)
	}
	if tx.SourceHeadAfter == tx.SourceHeadBefore {
		t.Fatalf("merge did not advance HEAD: %+v", tx)
	}
	if err := os.WriteFile(filepath.Join(repo, "drift.txt"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Rollback(context.Background(), a, tx); err == nil || !strings.Contains(err.Error(), "changed after delivery") {
		t.Fatalf("Rollback drift error = %v", err)
	}
	if err := os.Remove(filepath.Join(repo, "drift.txt")); err != nil {
		t.Fatal(err)
	}
	tx, err = Rollback(context.Background(), a, tx)
	if err != nil {
		t.Fatal(err)
	}
	if tx.RollbackCommit == "" {
		t.Fatalf("rollback transaction = %+v", tx)
	}
}

func TestSourceDirtyIsReportedButNotCopied(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("dirty source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	managed := t.TempDir()
	a, err := Create(context.Background(), repo, managed, "sa_dirty")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Remove(context.Background(), a, managed) }()
	if !a.SourceDirty {
		t.Fatal("SourceDirty = false")
	}
	got, err := os.ReadFile(filepath.Join(a.WorkspaceRoot, "README.md"))
	if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != "base\n" {
		t.Fatalf("worktree inherited dirty source: %q err=%v", got, err)
	}
}

func TestGitCommandArgsAndTimeouts(t *testing.T) {
	hasLongPaths := func(args []string) bool {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "-c" && args[i+1] == "core.longpaths=true" {
				return true
			}
		}
		return false
	}
	if !hasLongPaths(gitCommandArgs("windows", `C:\repo`, "status")) {
		t.Fatal("Windows Git args omit core.longpaths=true")
	}
	if hasLongPaths(gitCommandArgs("linux", "/repo", "status")) {
		t.Fatal("non-Windows Git args override longpaths")
	}
	if got := gitTimeout([]string{"worktree", "add", "x"}); got != gitWorktreeAddTimeout || got < 2*time.Minute {
		t.Fatalf("worktree add timeout = %v", got)
	}
	if got := gitTimeout([]string{"status"}); got != gitProbeTimeout {
		t.Fatalf("probe timeout = %v", got)
	}
}

func TestSafePathComponentHandlesWindowsNames(t *testing.T) {
	for _, name := range []string{"CON", "nul.txt", "LPT1", "bad:name", "trailing. "} {
		got := safePathComponent(name)
		if got == "" || strings.ContainsAny(got, `\\/:*?"<>|`) || strings.HasSuffix(got, ".") || strings.HasSuffix(got, " ") {
			t.Fatalf("safePathComponent(%q) = %q", name, got)
		}
	}
}

func TestCreateSupportsPathsWithSpaces(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo with spaces")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	requireGit(t)
	gitTest(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	managed := filepath.Join(parent, "managed worktrees")
	a, err := Create(context.Background(), repo, managed, "sa_spaces")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Remove(context.Background(), a, managed) }()
	if _, err := os.Stat(filepath.Join(a.WorkspaceRoot, "README.md")); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsLongCheckoutPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows long path semantics")
	}
	repo := initRepo(t)
	rel := ""
	for len(filepath.Join(repo, rel, "deep.txt")) <= 320 {
		rel = filepath.Join(rel, "deep-repository-segment")
	}
	path := filepath.Join(repo, rel, "deep.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("long\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "long")
	managed := t.TempDir()
	a, err := Create(context.Background(), repo, managed, "sa_long")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Remove(context.Background(), a, managed) }()
	if _, err := os.Stat(filepath.Join(a.WorkspaceRoot, rel, "deep.txt")); err != nil {
		t.Fatal(err)
	}
}
