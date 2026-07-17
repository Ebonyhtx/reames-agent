package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/fileutil"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
	"reames-agent/internal/tool/builtin"
	"reames-agent/internal/worktree"
)

func initSubagentDeliveryRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	repo := t.TempDir()
	runDeliveryGit(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runDeliveryGit(t, repo, "add", ".")
	runDeliveryGit(t, repo, "commit", "-m", "base")
	return repo
}

func runDeliveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "core.longpaths=true", "-c", "user.name=Reames Test", "-c", "user.email=test@example.invalid", "-C", dir}
	cmd := exec.Command("git", append(base, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestWriterTaskUsesIsolatedWorktreeAndExplicitDelivery(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	leaseDir := t.TempDir()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	parent := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: repo}).Tools("read_file", "write_file") {
		parent.Add(tl)
	}
	factory := func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		sub := SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth)
		for _, tl := range (builtin.Workspace{Dir: executionRoot}).Tools() {
			if _, ok := sub.Get(tl.Name()); ok {
				sub.Add(tl)
			}
		}
		return sub, nil
	}
	coordinator := NewSubagentWorkspaceCoordinator(managed, leaseDir, factory)
	store.WithWorkspaceCoordinator(coordinator)
	prov := &mockProvider{name: "writer", streams: [][]provider.Chunk{
		{{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "write-1", Name: "write_file", Arguments: `{"path":"child.txt","content":"child\n"}`}}},
		{{Type: provider.ChunkText, Text: "implemented in isolation"}, {Type: provider.ChunkDone}},
	}}
	task := NewTaskTool(prov, nil, parent, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, repo, "model", "").
		WithWorkspaceCoordinator(coordinator)
	ctx := WithWorkspaceRoot(WithParentSession(context.Background(), "parent-session"), repo)
	out, err := task.Execute(ctx, []byte(`{"prompt":"create child.txt","tools":["read_file","write_file"]}`))
	if err != nil {
		t.Fatal(err)
	}
	ref := subagentRefFromOutput(t, out)
	if _, err := os.Stat(filepath.Join(repo, "child.txt")); !os.IsNotExist(err) {
		t.Fatalf("source changed before delivery acceptance: %v", err)
	}
	view, err := store.Delivery(ref, repo)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != SubagentDeliveryReady || view.Workspace.Mode != SubagentWorkspaceGitWorktree || len(view.Changes) != 1 || view.Changes[0].Path != "child.txt" {
		t.Fatalf("delivery view = %+v", view)
	}
	if _, err := os.Stat(filepath.Join(view.Workspace.ExecutionRoot, "child.txt")); err != nil {
		t.Fatalf("isolated file missing: %v", err)
	}

	view, err = store.MutateDelivery(context.Background(), ref, repo, "apply")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != SubagentDeliveryApplied {
		t.Fatalf("applied status = %q", view.Status)
	}
	got, err := os.ReadFile(filepath.Join(repo, "child.txt"))
	if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != "child\n" {
		t.Fatalf("applied child = %q err=%v", got, err)
	}
	if _, err := store.MutateDelivery(context.Background(), ref, repo, "reject"); err == nil {
		t.Fatal("reject should require rollback after apply")
	}
	view, err = store.MutateDelivery(context.Background(), ref, repo, "rollback")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != SubagentDeliveryRolledBack {
		t.Fatalf("rollback status = %q", view.Status)
	}
	if _, err := os.Stat(filepath.Join(repo, "child.txt")); !os.IsNotExist(err) {
		t.Fatalf("child survived rollback: %v", err)
	}
	view, err = store.MutateDelivery(context.Background(), ref, repo, "reject")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != SubagentDeliveryRejected || view.Delivery.WorktreeLive {
		t.Fatalf("rejected view = %+v", view)
	}
}

func TestWriterTaskFailsClosedOutsideGit(t *testing.T) {
	root := t.TempDir()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	parent := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: root}).Tools("write_file") {
		parent.Add(tl)
	}
	coordinator := NewSubagentWorkspaceCoordinator(t.TempDir(), t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store.WithWorkspaceCoordinator(coordinator)
	task := NewTaskTool(&mockProvider{name: "writer"}, nil, parent, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, root, "model", "").
		WithWorkspaceCoordinator(coordinator)
	ctx := WithWorkspaceRoot(WithParentSession(context.Background(), "parent-session"), root)
	_, err := task.Execute(ctx, []byte(`{"prompt":"write","tools":["write_file"]}`))
	if err == nil || !strings.Contains(err.Error(), "isolated Git worktree") {
		t.Fatalf("Execute error = %v", err)
	}
}

func TestWriterTaskResolvesProfileBeforeAllocatingWorktree(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	parent := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: repo}).Tools("write_file") {
		parent.Add(tl)
	}
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store.WithWorkspaceCoordinator(coordinator)
	resolve := func(string, string) (provider.Provider, *provider.Pricing, int, error) {
		return nil, nil, 0, errors.New("invalid writer profile")
	}
	task := NewTaskTool(&mockProvider{name: "writer"}, nil, parent, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", resolve).
		WithTranscripts(store, repo, "model", "").
		WithWorkspaceCoordinator(coordinator)
	ctx := WithWorkspaceRoot(WithParentSession(context.Background(), "parent-session"), repo)
	_, err := task.Execute(ctx, []byte(`{"prompt":"write","tools":["write_file"],"model":"invalid"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid writer profile") {
		t.Fatalf("Execute error = %v, want profile failure", err)
	}
	entries, readErr := os.ReadDir(managed)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("profile failure allocated managed worktree entries: %v", entries)
	}
}

func TestAbortFreshPreparationRemovesWorktreeBranchAndArtifacts(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store.WithWorkspaceCoordinator(coordinator)
	workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareFresh(SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: "parent-session", Registry: tool.NewRegistry(), Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	defer run.Release()
	if err := store.MarkRunning(run); err != nil {
		t.Fatal(err)
	}
	if err := store.AbortFreshPreparation(run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.WorktreeRoot); !os.IsNotExist(err) {
		t.Fatalf("worktree survived abort: %v", err)
	}
	if got := runDeliveryGit(t, repo, "branch", "--list", workspace.Branch); got != "" {
		t.Fatalf("branch survived abort: %q", got)
	}
	for _, path := range []string{store.sessionPath(run.Ref), store.metaPath(run.Ref), store.effectPath(run.Ref)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artifact survived abort: %s (%v)", path, err)
		}
	}
}

func TestPermanentParentDeleteRemovesManagedWriterWorkspace(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	sessionDir := t.TempDir()
	store := NewSubagentStore(filepath.Join(sessionDir, "subagents"))
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store.WithWorkspaceCoordinator(coordinator)
	workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareFresh(SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: "parent-session", Registry: tool.NewRegistry(), Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	run.Release()
	if err := DeleteSubagentsByParentWithWorktrees(context.Background(), sessionDir, "parent-session", managed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.WorktreeRoot); !os.IsNotExist(err) {
		t.Fatalf("worktree survived parent purge: %v", err)
	}
	if got := runDeliveryGit(t, repo, "branch", "--list", workspace.Branch); got != "" {
		t.Fatalf("branch survived parent purge: %q", got)
	}
	if _, err := store.LoadMeta(run.Ref); err == nil {
		t.Fatal("subagent metadata survived parent purge")
	}
}

func TestContinuationPreparationFailureRetainsExistingWorktree(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	good := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store.WithWorkspaceCoordinator(good)
	workspace, _, err := good.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := worktree.Registered(context.Background(), workspace.RepoRoot, workspace.WorktreeRoot)
	if err != nil || !registered {
		t.Fatalf("new worktree registered=%t err=%v\n%s", registered, err, runDeliveryGit(t, repo, "worktree", "list", "--porcelain"))
	}
	run, err := store.PrepareFresh(SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: "parent-session", Registry: tool.NewRegistry(), Model: "model", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := store.SaveInterrupted(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	run.Release()

	parent := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: repo}).Tools("write_file") {
		parent.Add(tl)
	}
	bad := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return nil, errors.New("registry rebinding failed")
	})
	store.WithWorkspaceCoordinator(bad)
	task := NewTaskTool(&mockProvider{name: "writer"}, nil, parent, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, repo, "model", "").
		WithWorkspaceCoordinator(bad)
	ctx := WithWorkspaceRoot(WithParentSession(context.Background(), "parent-session"), repo)
	_, err = task.Execute(ctx, []byte(`{"prompt":"continue","tools":["write_file"],"continue_from":"`+run.Ref+`"}`))
	if err == nil || !strings.Contains(err.Error(), "registry rebinding failed") {
		t.Fatalf("Execute error = %v, want registry failure", err)
	}
	if _, err := os.Stat(workspace.WorktreeRoot); err != nil {
		t.Fatalf("continuation worktree was removed: %v", err)
	}
	if got := runDeliveryGit(t, repo, "branch", "--list", workspace.Branch); !strings.Contains(got, workspace.Branch) {
		t.Fatalf("continuation branch was removed: %q", got)
	}
}

type writeThenBlockProvider struct{ calls int }

func (p *writeThenBlockProvider) Name() string { return "write-then-block" }

func (p *writeThenBlockProvider) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	p.calls++
	if p.calls == 1 {
		ch := make(chan provider.Chunk, 1)
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "write-before-cancel", Name: "write_file", Arguments: `{"path":"retained.txt","content":"retained\n"}`}}
		close(ch)
		return ch, nil
	}
	return make(chan provider.Chunk), nil
}

func TestCanceledWriterRetainsDirtyInterruptedWorktree(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	sessionDir := t.TempDir()
	store := NewSubagentStore(filepath.Join(sessionDir, "subagents"))
	parent := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: repo}).Tools("write_file") {
		parent.Add(tl)
	}
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		sub := SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth)
		for _, tl := range (builtin.Workspace{Dir: executionRoot}).Tools() {
			if _, ok := sub.Get(tl.Name()); ok {
				sub.Add(tl)
			}
		}
		return sub, nil
	})
	store.WithWorkspaceCoordinator(coordinator)
	task := NewTaskTool(&writeThenBlockProvider{}, nil, parent, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, repo, "model", "").
		WithWorkspaceCoordinator(coordinator)
	ctx, cancel := context.WithCancel(WithWorkspaceRoot(WithParentSession(context.Background(), "parent-session"), repo))
	done := make(chan error, 1)
	go func() {
		_, err := task.Execute(ctx, []byte(`{"prompt":"write then wait","tools":["write_file"]}`))
		done <- err
	}()

	var artifact SubagentArtifact
	var lastListErr error
	fileObserved := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		artifacts, err := ListSubagentsByParent(sessionDir, "parent-session")
		if err != nil {
			lastListErr = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if len(artifacts) == 1 {
			artifact = artifacts[0]
			if _, err := os.Stat(filepath.Join(artifact.Meta.Workspace.ExecutionRoot, "retained.txt")); err == nil {
				fileObserved = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if artifact.Ref == "" || !fileObserved {
		cancel()
		t.Fatalf("writer subagent did not publish recovery metadata and retained file (last error: %v)", lastListErr)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer subagent did not stop after cancellation")
	}
	meta, err := store.LoadMeta(artifact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != SubagentInterrupted || meta.Delivery.Status != SubagentDeliveryInterrupted || !meta.Delivery.WorktreeLive || !meta.Delivery.Registered {
		t.Fatalf("cancelled writer metadata = %+v", meta)
	}
	if len(meta.Delivery.Files) != 1 || meta.Delivery.Files[0].Path != "retained.txt" {
		t.Fatalf("cancelled writer files = %+v", meta.Delivery.Files)
	}
	if _, err := os.Stat(filepath.Join(meta.Workspace.ExecutionRoot, "retained.txt")); err != nil {
		t.Fatalf("dirty worktree was not retained: %v", err)
	}
}

func TestStartupReconcilesDirtyRunningWriterAsInterrupted(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	subagentDir := filepath.Join(t.TempDir(), "subagents")
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store := NewSubagentStore(subagentDir).WithWorkspaceCoordinator(coordinator)
	workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareFresh(SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: "parent-session", Registry: tool.NewRegistry(), Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.ExecutionRoot, "crash.txt"), []byte("recover\n"), 0o644); err != nil {
		run.Release()
		t.Fatal(err)
	}
	run.Release()

	restarted := NewSubagentStore(subagentDir)
	if cleaned, err := restarted.CleanupStaleRunning(); err != nil || cleaned != 0 {
		t.Fatalf("pre-coordinator CleanupStaleRunning = %d, %v", cleaned, err)
	}
	preCoordinator, err := restarted.LoadMeta(run.Ref)
	if err != nil || preCoordinator.Status != SubagentRunning {
		t.Fatalf("pre-coordinator metadata = %+v, %v", preCoordinator, err)
	}
	restarted.WithWorkspaceCoordinator(coordinator)
	if cleaned, err := restarted.CleanupStaleRunning(); err != nil || cleaned != 1 {
		t.Fatalf("CleanupStaleRunning = %d, %v", cleaned, err)
	}
	meta, err := restarted.LoadMeta(run.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Status != SubagentInterrupted || meta.Delivery.Status != SubagentDeliveryInterrupted || !meta.Delivery.WorktreeLive || !meta.Delivery.Registered {
		t.Fatalf("reconciled metadata = %+v", meta)
	}
	if len(meta.Delivery.Files) != 1 || meta.Delivery.Files[0].Path != "crash.txt" {
		t.Fatalf("reconciled files = %+v", meta.Delivery.Files)
	}
}

func TestStartupClassifiesLostAndOrphanedWriterWorktrees(t *testing.T) {
	for _, tc := range []struct {
		name   string
		want   SubagentDeliveryStatus
		mutate func(t *testing.T, repo, managed string, workspace SubagentWorkspace)
	}{
		{
			name: "lost",
			want: SubagentDeliveryLost,
			mutate: func(t *testing.T, _ string, managed string, workspace SubagentWorkspace) {
				t.Helper()
				if err := worktree.Remove(context.Background(), workspace.assignment(), managed); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "orphaned",
			want: SubagentDeliveryOrphaned,
			mutate: func(t *testing.T, repo, _ string, workspace SubagentWorkspace) {
				t.Helper()
				runDeliveryGit(t, repo, "worktree", "remove", "--force", workspace.WorktreeRoot)
				if err := os.MkdirAll(workspace.WorktreeRoot, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := initSubagentDeliveryRepo(t)
			managed := t.TempDir()
			subagentDir := filepath.Join(t.TempDir(), "subagents")
			coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
				return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
			})
			store := NewSubagentStore(subagentDir).WithWorkspaceCoordinator(coordinator)
			workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			run, err := store.PrepareFresh(SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: "parent-session", Registry: tool.NewRegistry(), Workspace: workspace})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.MarkRunning(run); err != nil {
				run.Release()
				t.Fatal(err)
			}
			run.Release()
			tc.mutate(t, repo, managed, workspace)

			restarted := NewSubagentStore(subagentDir).WithWorkspaceCoordinator(coordinator)
			if cleaned, err := restarted.CleanupStaleRunning(); err != nil || cleaned != 1 {
				t.Fatalf("CleanupStaleRunning = %d, %v", cleaned, err)
			}
			meta, err := restarted.LoadMeta(run.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Status != SubagentInterrupted || meta.Delivery.Status != tc.want {
				t.Fatalf("reconciled status = %s/%s, want interrupted/%s", meta.Status, meta.Delivery.Status, tc.want)
			}
		})
	}
}

func TestNestedWriterDeliveryAppliesOnlyToParentWorktree(t *testing.T) {
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		sub := SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth)
		for _, tl := range (builtin.Workspace{Dir: executionRoot}).Tools() {
			if _, ok := sub.Get(tl.Name()); ok {
				sub.Add(tl)
			}
		}
		return sub, nil
	})
	outer, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = worktree.Remove(context.Background(), outer.assignment(), managed) }()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents")).WithWorkspaceCoordinator(coordinator)
	parent := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: outer.ExecutionRoot}).Tools("write_file") {
		parent.Add(tl)
	}
	prov := &mockProvider{name: "nested-writer", streams: [][]provider.Chunk{
		{{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "nested-write", Name: "write_file", Arguments: `{"path":"nested.txt","content":"nested\n"}`}}},
		{{Type: provider.ChunkText, Text: "nested delivery ready"}, {Type: provider.ChunkDone}},
	}}
	task := NewTaskTool(prov, nil, parent, 20, 0, 0, 0, 0, 0, 0, 0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(store, outer.ExecutionRoot, "model", "").
		WithWorkspaceCoordinator(coordinator).
		WithMaxSubagentDepth(2)
	ctx := WithSubagentDepth(WithWorkspaceRoot(WithParentSession(context.Background(), "outer-parent-session"), outer.ExecutionRoot), 1)
	out, err := task.Execute(ctx, []byte(`{"prompt":"create nested.txt","tools":["write_file"]}`))
	if err != nil {
		t.Fatal(err)
	}
	ref := subagentRefFromOutput(t, out)
	if _, err := os.Stat(filepath.Join(outer.ExecutionRoot, "nested.txt")); !os.IsNotExist(err) {
		t.Fatalf("nested delivery changed parent worktree before acceptance: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "nested.txt")); !os.IsNotExist(err) {
		t.Fatalf("nested delivery escaped into source repository: %v", err)
	}
	if _, err := store.MutateDelivery(context.Background(), ref, outer.ExecutionRoot, "apply"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outer.ExecutionRoot, "nested.txt")); err != nil {
		t.Fatalf("accepted nested delivery missing from parent worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "nested.txt")); !os.IsNotExist(err) {
		t.Fatalf("accepted nested delivery escaped into source repository: %v", err)
	}
	if _, err := store.MutateDelivery(context.Background(), ref, outer.ExecutionRoot, "rollback"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MutateDelivery(context.Background(), ref, outer.ExecutionRoot, "reject"); err != nil {
		t.Fatal(err)
	}
}

func readyAcceptanceRecoveryFixture(t *testing.T) (string, string, *SubagentStore, *SubagentWorkspaceCoordinator, *SubagentRun) {
	t.Helper()
	repo := initSubagentDeliveryRepo(t)
	managed := t.TempDir()
	store := NewSubagentStore(filepath.Join(t.TempDir(), "subagents"))
	coordinator := NewSubagentWorkspaceCoordinator(managed, t.TempDir(), func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store.WithWorkspaceCoordinator(coordinator)
	workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareFresh(SubagentSpec{Kind: "task", Name: "task", WorkspaceRoot: repo, ParentSession: "parent-session", Registry: tool.NewRegistry(), Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.ExecutionRoot, "acceptance.txt"), []byte("accepted\n"), 0o644); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := store.SaveCompleted(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	run.Release()
	return repo, managed, store, coordinator, run
}

func TestAcceptanceIntentRecoveryIsProofDriven(t *testing.T) {
	t.Run("intent persistence failure blocks mutation", func(t *testing.T) {
		repo, _, store, _, run := readyAcceptanceRecoveryFixture(t)
		store.metaWrite = func(string, []byte, os.FileMode) error {
			return errors.New("intent disk unavailable")
		}
		if _, err := store.MutateDelivery(context.Background(), run.Ref, repo, "apply"); err == nil || !strings.Contains(err.Error(), "persist delivery apply intent") {
			t.Fatalf("MutateDelivery error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo, "acceptance.txt")); !os.IsNotExist(err) {
			t.Fatalf("source changed without durable intent: %v", err)
		}
	})

	t.Run("terminal persistence failure remains recoverable intent", func(t *testing.T) {
		repo, _, store, coordinator, run := readyAcceptanceRecoveryFixture(t)
		writes := 0
		store.metaWrite = func(path string, data []byte, mode os.FileMode) error {
			writes++
			if writes == 2 {
				return errors.New("terminal metadata unavailable")
			}
			return fileutil.AtomicWriteFile(path, data, mode)
		}
		if _, err := store.MutateDelivery(context.Background(), run.Ref, repo, "apply"); err == nil || !strings.Contains(err.Error(), "terminal metadata unavailable") {
			t.Fatalf("MutateDelivery error = %v", err)
		}
		store.metaWrite = nil
		persisted, err := store.LoadMeta(run.Ref)
		if err != nil || persisted.Delivery.Status != SubagentDeliveryApplying || persisted.Delivery.Transaction == nil {
			t.Fatalf("persisted intent = %+v, %v", persisted, err)
		}
		restarted := NewSubagentStore(store.dir).WithWorkspaceCoordinator(coordinator)
		if n, err := restarted.ReconcilePendingDeliveries(); err != nil || n != 1 {
			t.Fatalf("ReconcilePendingDeliveries = %d, %v", n, err)
		}
		persisted, err = restarted.LoadMeta(run.Ref)
		if err != nil || persisted.Delivery.Status != SubagentDeliveryAcceptanceInterrupted {
			t.Fatalf("reconciled intent = %+v, %v", persisted, err)
		}
	})

	t.Run("before mutation returns to ready", func(t *testing.T) {
		repo, _, store, coordinator, run := readyAcceptanceRecoveryFixture(t)
		meta, err := store.LoadMeta(run.Ref)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := worktree.PrepareTransaction(context.Background(), meta.Workspace.assignment(), meta.Delivery.Commit, "apply")
		if err != nil {
			t.Fatal(err)
		}
		meta.Delivery.Status = SubagentDeliveryApplying
		meta.Delivery.Transaction = &tx
		if err := store.saveMeta(meta); err != nil {
			t.Fatal(err)
		}
		restarted := NewSubagentStore(store.dir).WithWorkspaceCoordinator(coordinator)
		if n, err := restarted.ReconcilePendingDeliveries(); err != nil || n != 1 {
			t.Fatalf("ReconcilePendingDeliveries = %d, %v", n, err)
		}
		meta, err = restarted.LoadMeta(run.Ref)
		if err != nil || meta.Delivery.Status != SubagentDeliveryReady || meta.Delivery.Transaction != nil {
			t.Fatalf("recovered metadata = %+v, %v", meta, err)
		}
		if _, err := os.Stat(filepath.Join(repo, "acceptance.txt")); !os.IsNotExist(err) {
			t.Fatalf("pre-mutation intent changed source: %v", err)
		}
	})

	t.Run("completed merge is recovered", func(t *testing.T) {
		_, _, store, coordinator, run := readyAcceptanceRecoveryFixture(t)
		meta, err := store.LoadMeta(run.Ref)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := worktree.PrepareTransaction(context.Background(), meta.Workspace.assignment(), meta.Delivery.Commit, "merge")
		if err != nil {
			t.Fatal(err)
		}
		meta.Delivery.Status = SubagentDeliveryMerging
		meta.Delivery.Transaction = &tx
		if err := store.saveMeta(meta); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.MergePrepared(context.Background(), meta.Workspace.assignment(), tx); err != nil {
			t.Fatal(err)
		}
		restarted := NewSubagentStore(store.dir).WithWorkspaceCoordinator(coordinator)
		if n, err := restarted.ReconcilePendingDeliveries(); err != nil || n != 1 {
			t.Fatalf("ReconcilePendingDeliveries = %d, %v", n, err)
		}
		meta, err = restarted.LoadMeta(run.Ref)
		if err != nil || meta.Delivery.Status != SubagentDeliveryMerged || meta.Delivery.Transaction == nil || meta.Delivery.Transaction.SourceHeadAfter == "" {
			t.Fatalf("recovered metadata = %+v, %v", meta, err)
		}
	})

	t.Run("post-apply ambiguity refuses automatic rollback", func(t *testing.T) {
		repo, _, store, coordinator, run := readyAcceptanceRecoveryFixture(t)
		meta, err := store.LoadMeta(run.Ref)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := worktree.PrepareTransaction(context.Background(), meta.Workspace.assignment(), meta.Delivery.Commit, "apply")
		if err != nil {
			t.Fatal(err)
		}
		meta.Delivery.Status = SubagentDeliveryApplying
		meta.Delivery.Transaction = &tx
		if err := store.saveMeta(meta); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.ApplyPrepared(context.Background(), meta.Workspace.assignment(), tx); err != nil {
			t.Fatal(err)
		}
		restarted := NewSubagentStore(store.dir).WithWorkspaceCoordinator(coordinator)
		if n, err := restarted.ReconcilePendingDeliveries(); err != nil || n != 1 {
			t.Fatalf("ReconcilePendingDeliveries = %d, %v", n, err)
		}
		meta, err = restarted.LoadMeta(run.Ref)
		if err != nil || meta.Delivery.Status != SubagentDeliveryAcceptanceInterrupted || meta.Delivery.Transaction == nil {
			t.Fatalf("recovered metadata = %+v, %v", meta, err)
		}
		if _, err := os.Stat(filepath.Join(repo, "acceptance.txt")); err != nil {
			t.Fatalf("ambiguous source change was overwritten: %v", err)
		}
	})
}
