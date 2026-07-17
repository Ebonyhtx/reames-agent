package control

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/tool"
	"reames-agent/internal/workspacelease"
)

func initControllerDeliveryRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	repo := t.TempDir()
	runControllerDeliveryGit(t, repo, "init")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runControllerDeliveryGit(t, repo, "add", ".")
	runControllerDeliveryGit(t, repo, "commit", "-m", "base")
	return repo
}

func runControllerDeliveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "core.longpaths=true", "-c", "user.name=Reames Test", "-c", "user.email=test@example.invalid", "-C", dir}
	cmd := exec.Command("git", append(base, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestControllerOwnsDeliveryLifecycleAndRuntimeReservation(t *testing.T) {
	repo := initControllerDeliveryRepo(t)
	managed := t.TempDir()
	leaseDir := t.TempDir()
	sessionDir := t.TempDir()
	parentSession := "parent-session"
	sessionPath := filepath.Join(sessionDir, parentSession+".jsonl")
	coordinator := agent.NewSubagentWorkspaceCoordinator(managed, leaseDir, func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error) {
		return agent.SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	})
	store := agent.NewSubagentStore(filepath.Join(sessionDir, "subagents")).WithWorkspaceCoordinator(coordinator)
	workspace, _, err := coordinator.CreateWriter(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.PrepareFresh(agent.SubagentSpec{
		Kind:          "task",
		Name:          "task",
		WorkspaceRoot: repo,
		ParentSession: parentSession,
		Registry:      tool.NewRegistry(),
		Workspace:     workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace.ExecutionRoot, "controller.txt"), []byte("controller\n"), 0o644); err != nil {
		run.Release()
		t.Fatal(err)
	}
	if err := store.SaveCompleted(run); err != nil {
		run.Release()
		t.Fatal(err)
	}
	run.Release()

	lease, err := workspacelease.New(repo, leaseDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctrl := New(Options{SessionDir: sessionDir, SessionPath: sessionPath, WorkspaceRoot: repo, WorkspaceLease: lease, SubagentStore: store})
	views, err := ctrl.SubagentDeliveries()
	if err != nil || len(views) != 1 || views[0].Ref != run.Ref || views[0].Status != agent.SubagentDeliveryReady {
		t.Fatalf("SubagentDeliveries = %+v, %v", views, err)
	}
	preview, err := ctrl.SubagentDelivery(run.Ref)
	if err != nil || len(preview.Changes) != 1 || preview.Changes[0].Path != "controller.txt" {
		t.Fatalf("SubagentDelivery = %+v, %v", preview, err)
	}
	releaseRuntime, err := ctrl.BeginRuntimeMutation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.MutateSubagentDelivery(context.Background(), run.Ref, "apply"); !errors.Is(err, ErrRuntimeMutationBusy) {
		releaseRuntime()
		t.Fatalf("busy mutate error = %v", err)
	}
	releaseRuntime()

	view, err := ctrl.MutateSubagentDelivery(context.Background(), run.Ref, "apply")
	if err != nil || view.Status != agent.SubagentDeliveryApplied {
		t.Fatalf("apply = %+v, %v", view, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "controller.txt")); err != nil {
		t.Fatalf("applied file missing: %v", err)
	}
	view, err = ctrl.MutateSubagentDelivery(context.Background(), run.Ref, "rollback")
	if err != nil || view.Status != agent.SubagentDeliveryRolledBack {
		t.Fatalf("rollback = %+v, %v", view, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "controller.txt")); !os.IsNotExist(err) {
		t.Fatalf("rolled-back file remains: %v", err)
	}
	view, err = ctrl.MutateSubagentDelivery(context.Background(), run.Ref, "reject")
	if err != nil || view.Status != agent.SubagentDeliveryRejected || view.Delivery.WorktreeLive {
		t.Fatalf("reject = %+v, %v", view, err)
	}
}
