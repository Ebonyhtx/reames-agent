package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"reames-agent/internal/diff"
	"reames-agent/internal/tool"
	"reames-agent/internal/worktree"
)

// SubagentDeliveryView is the transport-neutral projection shared by the
// model tool, Controller, CLI, serve, and Desktop.
type SubagentDeliveryView struct {
	Ref       string                 `json:"ref"`
	Kind      string                 `json:"kind"`
	Name      string                 `json:"name"`
	Status    SubagentDeliveryStatus `json:"status"`
	Workspace SubagentWorkspace      `json:"workspace"`
	Delivery  SubagentDelivery       `json:"delivery"`
	Changes   []diff.Change          `json:"changes,omitempty"`
}

func (s *SubagentStore) Delivery(ref, sourceRoot string) (SubagentDeliveryView, error) {
	if s == nil {
		return SubagentDeliveryView{}, errors.New("subagent delivery store is unavailable")
	}
	meta, err := s.LoadMeta(strings.TrimSpace(ref))
	if err != nil {
		return SubagentDeliveryView{}, err
	}
	if err := authorizeDeliverySource(meta, sourceRoot); err != nil {
		return SubagentDeliveryView{}, err
	}
	view := SubagentDeliveryView{Ref: meta.Ref, Kind: meta.Kind, Name: meta.Name, Status: meta.Delivery.Status, Workspace: meta.Workspace, Delivery: meta.Delivery}
	if meta.Workspace.Mode != SubagentWorkspaceGitWorktree || !workspaceAvailable(meta.Workspace.WorktreeRoot) {
		return view, nil
	}
	snapshot, err := worktree.SnapshotDelivery(context.Background(), meta.Workspace.assignment())
	if err != nil {
		return view, err
	}
	view.Changes = snapshot.Changes
	view.Delivery.Head = snapshot.Head
	view.Delivery.PatchDigest = snapshot.PatchDigest
	view.Delivery.Commits = snapshot.Commits
	view.Delivery.Registered = snapshot.Registered
	view.Delivery.WorktreeLive = true
	view.Delivery.Tests = s.deliveryTests(meta.Ref)
	return view, nil
}

func (s *SubagentStore) ListDeliveries(parentSession, sourceRoot string) ([]SubagentDeliveryView, error) {
	if s == nil || strings.TrimSpace(parentSession) == "" {
		return nil, nil
	}
	artifacts, err := listSubagentsInDir(s.dir, strings.TrimSpace(parentSession))
	if err != nil {
		return nil, err
	}
	views := make([]SubagentDeliveryView, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
			continue
		}
		view, viewErr := s.Delivery(artifact.Ref, sourceRoot)
		if viewErr != nil {
			view = SubagentDeliveryView{Ref: artifact.Ref, Kind: artifact.Meta.Kind, Name: artifact.Meta.Name, Status: artifact.Meta.Delivery.Status, Workspace: artifact.Meta.Workspace, Delivery: artifact.Meta.Delivery}
			view.Delivery.LastError = viewErr.Error()
		}
		views = append(views, view)
	}
	return views, nil
}

// ReconcilePendingDeliveries resolves acceptance intents left by a process
// exit between the durable pre-mutation record and the terminal metadata write.
func (s *SubagentStore) ReconcilePendingDeliveries() (int, error) {
	if s == nil {
		return 0, nil
	}
	artifacts, err := listAllSubagentsInDir(s.dir)
	if err != nil {
		return 0, err
	}
	reconciled := 0
	for _, artifact := range artifacts {
		if artifact.Meta.Delivery.Status != SubagentDeliveryApplying && artifact.Meta.Delivery.Status != SubagentDeliveryMerging {
			continue
		}
		release, lockErr := s.lock(artifact.Ref)
		if lockErr != nil {
			return reconciled, lockErr
		}
		meta, loadErr := s.LoadMeta(artifact.Ref)
		if loadErr == nil {
			loadErr = s.reconcilePendingDelivery(&meta)
		}
		release()
		if loadErr != nil {
			return reconciled, loadErr
		}
		reconciled++
	}
	return reconciled, nil
}

func (s *SubagentStore) reconcilePendingDelivery(meta *SubagentMeta) error {
	if meta == nil || (meta.Delivery.Status != SubagentDeliveryApplying && meta.Delivery.Status != SubagentDeliveryMerging) {
		return nil
	}
	now := time.Now().UTC()
	if meta.Delivery.Transaction == nil {
		meta.Delivery.Status = SubagentDeliveryAcceptanceInterrupted
		meta.Delivery.LastError = "delivery acceptance intent is missing its recovery transaction; inspect the source workspace manually"
		meta.Delivery.UpdatedAt = now
		meta.UpdatedAt = now
		return s.saveMeta(*meta)
	}
	state, tx, err := worktree.RecoverPrepared(context.Background(), meta.Workspace.assignment(), *meta.Delivery.Transaction)
	if err != nil {
		return fmt.Errorf("reconcile delivery acceptance %q: %w", meta.Ref, err)
	}
	switch state {
	case worktree.RecoveryBefore:
		meta.Delivery.Status = SubagentDeliveryReady
		meta.Delivery.Transaction = nil
		meta.Delivery.LastError = "previous delivery acceptance stopped before changing the source workspace; retry when ready"
	case worktree.RecoveryCompleted:
		meta.Delivery.Transaction = &tx
		if tx.Kind == "merge" {
			meta.Delivery.Status = SubagentDeliveryMerged
		} else {
			meta.Delivery.Status = SubagentDeliveryApplied
		}
		meta.Delivery.LastError = ""
	case worktree.RecoveryAmbiguous:
		meta.Delivery.Status = SubagentDeliveryAcceptanceInterrupted
		meta.Delivery.LastError = "delivery acceptance stopped after the source workspace may have changed; automatic rollback is refused because later user edits cannot be distinguished; inspect Git state and resolve manually"
	default:
		return fmt.Errorf("reconcile delivery acceptance %q returned unknown state %q", meta.Ref, state)
	}
	meta.Delivery.UpdatedAt = now
	meta.UpdatedAt = now
	return s.saveMeta(*meta)
}

func (s *SubagentStore) MutateDelivery(ctx context.Context, ref, sourceRoot, op string) (SubagentDeliveryView, error) {
	if s == nil || s.workspace == nil {
		return SubagentDeliveryView{}, errors.New("subagent delivery workspace manager is unavailable")
	}
	ref = strings.TrimSpace(ref)
	op = strings.ToLower(strings.TrimSpace(op))
	release, err := s.lock(ref)
	if err != nil {
		return SubagentDeliveryView{}, err
	}
	defer release()
	meta, err := s.LoadMeta(ref)
	if err != nil {
		return SubagentDeliveryView{}, err
	}
	if err := authorizeDeliverySource(meta, sourceRoot); err != nil {
		return SubagentDeliveryView{}, err
	}
	if meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
		return SubagentDeliveryView{}, fmt.Errorf("subagent %q has no isolated Git delivery", ref)
	}
	assignment := meta.Workspace.assignment()
	switch op {
	case "apply", "merge":
		if meta.Status != SubagentCompleted || meta.Delivery.Status != SubagentDeliveryReady {
			return SubagentDeliveryView{}, fmt.Errorf("delivery %q is %s/%s, not ready", ref, meta.Status, meta.Delivery.Status)
		}
		tx, prepareErr := worktree.PrepareTransaction(ctx, assignment, meta.Delivery.Commit, op)
		if prepareErr != nil {
			meta.Delivery.LastError = prepareErr.Error()
			meta.Delivery.UpdatedAt = time.Now().UTC()
			_ = s.saveMeta(meta)
			return SubagentDeliveryView{}, prepareErr
		}
		meta.Delivery.Transaction = &tx
		if op == "apply" {
			meta.Delivery.Status = SubagentDeliveryApplying
		} else {
			meta.Delivery.Status = SubagentDeliveryMerging
		}
		meta.Delivery.LastError = ""
		meta.Delivery.UpdatedAt = time.Now().UTC()
		meta.UpdatedAt = meta.Delivery.UpdatedAt
		if err := s.saveMeta(meta); err != nil {
			return SubagentDeliveryView{}, fmt.Errorf("persist delivery %s intent: %w", op, err)
		}
		if op == "apply" {
			tx, err = worktree.ApplyPrepared(ctx, assignment, tx)
		} else {
			tx, err = worktree.MergePrepared(ctx, assignment, tx)
		}
		if err != nil {
			meta.Delivery.Status = SubagentDeliveryReady
			meta.Delivery.Transaction = nil
			meta.Delivery.LastError = err.Error()
			meta.Delivery.UpdatedAt = time.Now().UTC()
			_ = s.saveMeta(meta)
			return SubagentDeliveryView{}, err
		}
		meta.Delivery.Transaction = &tx
		if op == "apply" {
			meta.Delivery.Status = SubagentDeliveryApplied
		} else {
			meta.Delivery.Status = SubagentDeliveryMerged
		}
		meta.Delivery.LastError = ""
	case "rollback":
		if meta.Delivery.Transaction == nil || (meta.Delivery.Status != SubagentDeliveryApplied && meta.Delivery.Status != SubagentDeliveryMerged) {
			return SubagentDeliveryView{}, fmt.Errorf("delivery %q has no active apply/merge transaction to roll back", ref)
		}
		tx, rollbackErr := worktree.Rollback(ctx, assignment, *meta.Delivery.Transaction)
		if rollbackErr != nil {
			meta.Delivery.LastError = rollbackErr.Error()
			meta.Delivery.UpdatedAt = time.Now().UTC()
			_ = s.saveMeta(meta)
			return SubagentDeliveryView{}, rollbackErr
		}
		meta.Delivery.Transaction = &tx
		meta.Delivery.Status = SubagentDeliveryRolledBack
		meta.Delivery.LastError = ""
	case "reject":
		if meta.Delivery.Status == SubagentDeliveryApplying || meta.Delivery.Status == SubagentDeliveryMerging || meta.Delivery.Status == SubagentDeliveryAcceptanceInterrupted {
			return SubagentDeliveryView{}, fmt.Errorf("delivery %q acceptance is %s; inspect and resolve the source workspace before rejecting its recovery worktree", ref, meta.Delivery.Status)
		}
		if meta.Delivery.Status == SubagentDeliveryApplied || meta.Delivery.Status == SubagentDeliveryMerged {
			return SubagentDeliveryView{}, fmt.Errorf("delivery %q is %s; roll it back before rejecting its retained worktree", ref, meta.Delivery.Status)
		}
		if err := worktree.Remove(ctx, assignment, s.workspace.managedRoot); err != nil {
			meta.Delivery.LastError = err.Error()
			meta.Delivery.UpdatedAt = time.Now().UTC()
			_ = s.saveMeta(meta)
			return SubagentDeliveryView{}, err
		}
		meta.Delivery.Status = SubagentDeliveryRejected
		meta.Delivery.WorktreeLive = false
		meta.Delivery.Registered = false
		meta.Delivery.LastError = ""
	default:
		return SubagentDeliveryView{}, fmt.Errorf("unsupported delivery operation %q", op)
	}
	meta.Delivery.UpdatedAt = time.Now().UTC()
	meta.UpdatedAt = meta.Delivery.UpdatedAt
	if err := s.saveMeta(meta); err != nil {
		return SubagentDeliveryView{}, err
	}
	return s.Delivery(ref, sourceRoot)
}

func authorizeDeliverySource(meta SubagentMeta, sourceRoot string) error {
	if meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
		return nil
	}
	want, err := exactCanonicalPath(meta.Workspace.SourceRoot)
	if err != nil {
		return err
	}
	got, err := exactCanonicalPath(sourceRoot)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("delivery %q belongs to source workspace %q, current workspace is %q", meta.Ref, meta.Workspace.SourceRoot, sourceRoot)
	}
	return nil
}

func exactCanonicalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("workspace root is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = filepath.Clean(resolved)
	} else if !os.IsNotExist(resolveErr) {
		return "", resolveErr
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(filepath.ToSlash(abs))
	}
	return abs, nil
}

type SubagentDeliveryPreviewTool struct {
	store         *SubagentStore
	workspaceRoot string
}

func NewSubagentDeliveryPreviewTool(store *SubagentStore, workspaceRoot string) tool.Tool {
	return &SubagentDeliveryPreviewTool{store: store, workspaceRoot: strings.TrimSpace(workspaceRoot)}
}

func (*SubagentDeliveryPreviewTool) Name() string { return "subagent_delivery_preview" }
func (*SubagentDeliveryPreviewTool) Description() string {
	return "Preview an isolated writer subagent delivery: branch/worktree identity, changed files, commits, patch digest, and recorded test commands. This never changes source files."
}
func (*SubagentDeliveryPreviewTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"ref":{"type":"string","description":"Subagent reference (sa_...)."}},"required":["ref"]}`)
}
func (*SubagentDeliveryPreviewTool) ReadOnly() bool     { return true }
func (*SubagentDeliveryPreviewTool) PlanModeSafe() bool { return true }
func (t *SubagentDeliveryPreviewTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	view, err := t.store.Delivery(p.Ref, WorkspaceRootFromContext(ctx, t.workspaceRoot))
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(view, "", "  ")
	return string(data), err
}

type SubagentDeliveryTool struct {
	store         *SubagentStore
	workspaceRoot string
}

func NewSubagentDeliveryTool(store *SubagentStore, workspaceRoot string) tool.Tool {
	return &SubagentDeliveryTool{store: store, workspaceRoot: strings.TrimSpace(workspaceRoot)}
}

func (*SubagentDeliveryTool) Name() string { return "subagent_delivery" }
func (*SubagentDeliveryTool) Description() string {
	return "Accept or dispose of an isolated writer subagent delivery. apply stages its patch in the current source workspace; merge creates a no-fast-forward merge commit; rollback is allowed only while source state exactly matches the recorded post-action state; reject deletes only the managed worktree and delivery branch. Preview first with subagent_delivery_preview."
}
func (*SubagentDeliveryTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"ref":{"type":"string","description":"Subagent reference (sa_...)."},"op":{"type":"string","enum":["apply","merge","rollback","reject"]}},"required":["ref","op"]}`)
}
func (*SubagentDeliveryTool) ReadOnly() bool { return false }

func (t *SubagentDeliveryTool) PreviewChanges(args json.RawMessage) ([]diff.Change, error) {
	var p struct {
		Ref string `json:"ref"`
		Op  string `json:"op"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	view, err := t.store.Delivery(p.Ref, t.workspaceRoot)
	if err != nil {
		return nil, err
	}
	if p.Op != "rollback" {
		if p.Op == "reject" {
			return nil, nil
		}
		return view.Changes, nil
	}
	reversed := make([]diff.Change, len(view.Changes))
	for i, change := range view.Changes {
		kind := diff.Modify
		if change.Kind == diff.Create {
			kind = diff.Delete
		} else if change.Kind == diff.Delete {
			kind = diff.Create
		}
		reversed[i] = diff.Build(change.Path, change.NewText, change.OldText, kind)
	}
	return reversed, nil
}

func (t *SubagentDeliveryTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Ref string `json:"ref"`
		Op  string `json:"op"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	view, err := t.store.MutateDelivery(ctx, p.Ref, WorkspaceRootFromContext(ctx, t.workspaceRoot), p.Op)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(view, "", "  ")
	return string(data), err
}
