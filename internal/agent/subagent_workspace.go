package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"reames-agent/internal/diff"
	"reames-agent/internal/tool"
	"reames-agent/internal/workspacelease"
	"reames-agent/internal/worktree"
)

type SubagentWorkspaceMode string

const (
	SubagentWorkspaceSharedReadOnly SubagentWorkspaceMode = "shared_read_only"
	SubagentWorkspaceGitWorktree    SubagentWorkspaceMode = "git_worktree"
)

type SubagentDeliveryStatus string

const (
	SubagentDeliveryActive                SubagentDeliveryStatus = "active"
	SubagentDeliveryReady                 SubagentDeliveryStatus = "ready"
	SubagentDeliveryApplying              SubagentDeliveryStatus = "applying"
	SubagentDeliveryMerging               SubagentDeliveryStatus = "merging"
	SubagentDeliveryAcceptanceInterrupted SubagentDeliveryStatus = "acceptance_interrupted"
	SubagentDeliveryEmpty                 SubagentDeliveryStatus = "empty"
	SubagentDeliveryFailed                SubagentDeliveryStatus = "failed"
	SubagentDeliveryInterrupted           SubagentDeliveryStatus = "interrupted"
	SubagentDeliveryApplied               SubagentDeliveryStatus = "applied"
	SubagentDeliveryMerged                SubagentDeliveryStatus = "merged"
	SubagentDeliveryRejected              SubagentDeliveryStatus = "rejected"
	SubagentDeliveryRolledBack            SubagentDeliveryStatus = "rolled_back"
	SubagentDeliveryLost                  SubagentDeliveryStatus = "lost"
	SubagentDeliveryOrphaned              SubagentDeliveryStatus = "orphaned"
)

// SubagentWorkspace is the durable identity of a child runtime. SourceRoot is
// the immediate parent workspace; ExecutionRoot is where child tools resolve.
type SubagentWorkspace struct {
	Mode          SubagentWorkspaceMode `json:"mode"`
	SourceRoot    string                `json:"sourceRoot"`
	ExecutionRoot string                `json:"executionRoot"`
	WorktreeRoot  string                `json:"worktreeRoot,omitempty"`
	RepoRoot      string                `json:"repoRoot,omitempty"`
	Branch        string                `json:"branch,omitempty"`
	BaseHead      string                `json:"baseHead,omitempty"`
	Prefix        string                `json:"prefix,omitempty"`
	SourceDirty   bool                  `json:"sourceDirty,omitempty"`
}

type SubagentDeliveryFile struct {
	Path    string    `json:"path"`
	Kind    diff.Kind `json:"kind"`
	Added   int       `json:"added,omitempty"`
	Removed int       `json:"removed,omitempty"`
	Binary  bool      `json:"binary,omitempty"`
}

type SubagentDeliveryTest struct {
	Command string `json:"command"`
	Success bool   `json:"success"`
}

// SubagentDelivery is the persisted, bounded delivery projection. Full before
// and after file contents are always derived live from Git and never stored in
// session metadata.
type SubagentDelivery struct {
	Status       SubagentDeliveryStatus `json:"status"`
	Commit       string                 `json:"commit,omitempty"`
	Head         string                 `json:"head,omitempty"`
	PatchDigest  string                 `json:"patchDigest,omitempty"`
	Files        []SubagentDeliveryFile `json:"files,omitempty"`
	Commits      []worktree.Commit      `json:"commits,omitempty"`
	Tests        []SubagentDeliveryTest `json:"tests,omitempty"`
	Transaction  *worktree.Transaction  `json:"transaction,omitempty"`
	LastError    string                 `json:"lastError,omitempty"`
	UpdatedAt    time.Time              `json:"updatedAt"`
	SourceDirty  bool                   `json:"sourceDirty,omitempty"`
	Registered   bool                   `json:"registered,omitempty"`
	WorktreeLive bool                   `json:"worktreeLive,omitempty"`
}

// WorkspaceRegistryFactory rebuilds workspace-bound tools for executionRoot.
// It must fail closed rather than leave tools bound to sourceRoot.
type WorkspaceRegistryFactory func(parent *tool.Registry, names []string, childDepth, maxDepth int, executionRoot string) (*tool.Registry, error)

// SubagentWorkspaceCoordinator owns worktree allocation, registry rebinding,
// per-worktree leases, sealing, and crash reconciliation.
type SubagentWorkspaceCoordinator struct {
	managedRoot string
	leaseDir    string
	registry    WorkspaceRegistryFactory
}

func NewSubagentWorkspaceCoordinator(managedRoot, leaseDir string, registry WorkspaceRegistryFactory) *SubagentWorkspaceCoordinator {
	if strings.TrimSpace(managedRoot) == "" || strings.TrimSpace(leaseDir) == "" || registry == nil {
		return nil
	}
	return &SubagentWorkspaceCoordinator{managedRoot: strings.TrimSpace(managedRoot), leaseDir: strings.TrimSpace(leaseDir), registry: registry}
}

func (c *SubagentWorkspaceCoordinator) SharedReadOnly(sourceRoot string) SubagentWorkspace {
	sourceRoot = strings.TrimSpace(sourceRoot)
	return SubagentWorkspace{Mode: SubagentWorkspaceSharedReadOnly, SourceRoot: sourceRoot, ExecutionRoot: sourceRoot}
}

// CreateWriter allocates an isolated Git worktree. Non-Git workspaces fail
// closed with an actionable message; read-only subagents remain available.
func (c *SubagentWorkspaceCoordinator) CreateWriter(ctx context.Context, sourceRoot string) (SubagentWorkspace, *workspacelease.Owner, error) {
	if c == nil {
		return SubagentWorkspace{}, nil, errors.New("writer subagent workspace isolation is unavailable")
	}
	a, err := worktree.Create(ctx, sourceRoot, c.managedRoot, "")
	if err != nil {
		return SubagentWorkspace{}, nil, fmt.Errorf("writer subagent requires an isolated Git worktree: %w; use read_only_task for research, or initialize and commit this repository before delegating writes", err)
	}
	workspace := subagentWorkspaceFromAssignment(a)
	lease, err := workspacelease.New(workspace.ExecutionRoot, c.leaseDir, nil)
	if err != nil {
		_ = worktree.Remove(context.Background(), a, c.managedRoot)
		return SubagentWorkspace{}, nil, fmt.Errorf("initialize child workspace lease: %w", err)
	}
	return workspace, lease, nil
}

func (c *SubagentWorkspaceCoordinator) Resume(meta SubagentMeta) (*workspacelease.Owner, error) {
	if meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
		return nil, nil
	}
	if c == nil {
		return nil, errors.New("writer subagent workspace isolation is unavailable")
	}
	exists, registered, err := worktree.Reconcile(context.Background(), meta.Workspace.assignment(), c.managedRoot)
	if err != nil {
		return nil, err
	}
	if !exists || !registered {
		return nil, fmt.Errorf("subagent worktree is unavailable (exists=%t registered=%t)", exists, registered)
	}
	return workspacelease.New(meta.Workspace.ExecutionRoot, c.leaseDir, nil)
}

func (c *SubagentWorkspaceCoordinator) Registry(parent *tool.Registry, names []string, childDepth, maxDepth int, workspace SubagentWorkspace) (*tool.Registry, error) {
	if c == nil || c.registry == nil {
		return SubagentToolRegistryForDepth(parent, names, childDepth, maxDepth), nil
	}
	return c.registry(parent, names, childDepth, maxDepth, workspace.ExecutionRoot)
}

func (c *SubagentWorkspaceCoordinator) RejectPreparation(workspace SubagentWorkspace) {
	if c == nil || workspace.Mode != SubagentWorkspaceGitWorktree {
		return
	}
	_ = worktree.Remove(context.Background(), workspace.assignment(), c.managedRoot)
}

func (c *SubagentWorkspaceCoordinator) seal(run *SubagentRun) error {
	if c == nil || run == nil || run.Meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
		return nil
	}
	commit, err := worktree.Seal(context.Background(), run.Meta.Workspace.assignment(), run.Ref)
	if err != nil {
		run.Meta.Delivery.LastError = err.Error()
		run.Meta.Delivery.UpdatedAt = time.Now().UTC()
		return err
	}
	run.Meta.Delivery.Commit = commit
	return c.refresh(run, SubagentDeliveryReady, "")
}

func (c *SubagentWorkspaceCoordinator) refresh(run *SubagentRun, status SubagentDeliveryStatus, lastError string) error {
	if c == nil || run == nil || run.Meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
		return nil
	}
	snapshot, err := worktree.SnapshotDelivery(context.Background(), run.Meta.Workspace.assignment())
	if err != nil {
		run.Meta.Delivery.Status = status
		run.Meta.Delivery.LastError = errors.Join(errors.New(lastError), err).Error()
		run.Meta.Delivery.UpdatedAt = time.Now().UTC()
		return err
	}
	files := make([]SubagentDeliveryFile, len(snapshot.Changes))
	for i, change := range snapshot.Changes {
		files[i] = SubagentDeliveryFile{Path: change.Path, Kind: change.Kind, Added: change.Added, Removed: change.Removed, Binary: change.Binary}
	}
	if status == SubagentDeliveryReady && len(files) == 0 && len(snapshot.Commits) == 0 {
		status = SubagentDeliveryEmpty
	}
	run.Meta.Delivery.Status = status
	run.Meta.Delivery.Head = snapshot.Head
	run.Meta.Delivery.PatchDigest = snapshot.PatchDigest
	run.Meta.Delivery.Files = files
	run.Meta.Delivery.Commits = snapshot.Commits
	run.Meta.Delivery.Tests = run.store.deliveryTests(run.Ref)
	run.Meta.Delivery.LastError = strings.TrimSpace(lastError)
	run.Meta.Delivery.UpdatedAt = time.Now().UTC()
	run.Meta.Delivery.SourceDirty = run.Meta.Workspace.SourceDirty
	run.Meta.Delivery.Registered = snapshot.Registered
	run.Meta.Delivery.WorktreeLive = true
	return nil
}

func (c *SubagentWorkspaceCoordinator) failed(run *SubagentRun, interrupted bool) error {
	status := SubagentDeliveryFailed
	if interrupted {
		status = SubagentDeliveryInterrupted
	}
	return c.refresh(run, status, "")
}

func (c *SubagentWorkspaceCoordinator) reconcile(meta *SubagentMeta) error {
	if c == nil || meta == nil || meta.Workspace.Mode != SubagentWorkspaceGitWorktree {
		return nil
	}
	exists, registered, err := worktree.Reconcile(context.Background(), meta.Workspace.assignment(), c.managedRoot)
	meta.Delivery.WorktreeLive = exists
	meta.Delivery.Registered = registered
	meta.Delivery.UpdatedAt = time.Now().UTC()
	if err != nil {
		meta.Delivery.LastError = err.Error()
		return err
	}
	if !exists && !registered {
		meta.Delivery.Status = SubagentDeliveryLost
		meta.Delivery.LastError = "managed worktree no longer exists"
		return nil
	}
	if exists && !registered {
		meta.Delivery.Status = SubagentDeliveryOrphaned
		meta.Delivery.LastError = "managed worktree exists but Git no longer registers it"
		return nil
	}
	run := &SubagentRun{Ref: meta.Ref, Meta: *meta, store: nil}
	snapshot, snapErr := worktree.SnapshotDelivery(context.Background(), meta.Workspace.assignment())
	if snapErr != nil {
		meta.Delivery.LastError = snapErr.Error()
		return snapErr
	}
	files := make([]SubagentDeliveryFile, len(snapshot.Changes))
	for i, change := range snapshot.Changes {
		files[i] = SubagentDeliveryFile{Path: change.Path, Kind: change.Kind, Added: change.Added, Removed: change.Removed, Binary: change.Binary}
	}
	run.Meta.Delivery.Files = files
	run.Meta.Delivery.Commits = snapshot.Commits
	run.Meta.Delivery.Head = snapshot.Head
	run.Meta.Delivery.PatchDigest = snapshot.PatchDigest
	run.Meta.Delivery.Status = SubagentDeliveryInterrupted
	run.Meta.Delivery.LastError = "previous runtime stopped before the child delivery reached a terminal state"
	run.Meta.Delivery.UpdatedAt = time.Now().UTC()
	*meta = run.Meta
	return nil
}

func subagentWorkspaceFromAssignment(a worktree.Assignment) SubagentWorkspace {
	return SubagentWorkspace{Mode: SubagentWorkspaceGitWorktree, SourceRoot: a.SourceWorkspaceRoot, ExecutionRoot: a.WorkspaceRoot, WorktreeRoot: a.WorktreeRoot, RepoRoot: a.RepoRoot, Branch: a.Branch, BaseHead: a.BaseHead, Prefix: a.Prefix, SourceDirty: a.SourceDirty}
}

func (w SubagentWorkspace) assignment() worktree.Assignment {
	return worktree.Assignment{WorkspaceRoot: w.ExecutionRoot, WorktreeRoot: w.WorktreeRoot, RepoRoot: w.RepoRoot, SourceWorkspaceRoot: w.SourceRoot, Branch: w.Branch, BaseHead: w.BaseHead, Prefix: w.Prefix, SourceDirty: w.SourceDirty}
}

// RegistryCanWrite reports whether a child registry contains any writer tool.
func RegistryCanWrite(reg *tool.Registry) bool {
	if reg == nil {
		return false
	}
	for _, name := range reg.Names() {
		if tl, ok := reg.Get(name); ok && !tl.ReadOnly() {
			return true
		}
	}
	return false
}

func workspaceAvailable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// DeliveryBlocksContinuation reports whether a delivery is already accepted,
// disposed, or in an unresolved acceptance transaction.
func DeliveryBlocksContinuation(status SubagentDeliveryStatus) bool {
	switch status {
	case SubagentDeliveryApplying, SubagentDeliveryMerging, SubagentDeliveryAcceptanceInterrupted,
		SubagentDeliveryApplied, SubagentDeliveryMerged, SubagentDeliveryRejected, SubagentDeliveryRolledBack:
		return true
	default:
		return false
	}
}
