package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/config"
)

// ErrSessionLeaseHeld is the stable control-layer ownership conflict used by
// frontends that must return it from a deferred lifecycle operation.
var ErrSessionLeaseHeld = agent.ErrSessionLeaseHeld

// SessionRemovalGuard holds the store's save and lease locks across one
// destructive operation without exposing lock-file implementation types.
type SessionRemovalGuard struct{ inner *agent.SessionRemovalGuard }

func TryAcquireSessionRemovalGuard(path string) (*SessionRemovalGuard, error) {
	guard, err := agent.TryAcquireSessionRemovalGuard(path)
	if err != nil {
		return nil, err
	}
	return &SessionRemovalGuard{inner: guard}, nil
}

func (g *SessionRemovalGuard) Release() {
	if g != nil && g.inner != nil {
		g.inner.Release()
	}
}

func (g *SessionRemovalGuard) RemoveSidecarsAndRelease() error {
	if g == nil || g.inner == nil {
		return nil
	}
	return g.inner.RemoveSidecarsAndRelease()
}

// SessionSubagentArtifact is the filesystem portion a transport needs when it
// moves a parent session and its owned sub-agent records together.
type SessionSubagentArtifact struct {
	SessionPath string
	MetaPath    string
	EffectPath  string
}

func ListSessionSubagentArtifacts(sessionDir, sessionPath string) ([]SessionSubagentArtifact, error) {
	artifacts, err := agent.ListSubagentsByParent(sessionDir, agent.BranchID(sessionPath))
	if err != nil {
		return nil, err
	}
	out := make([]SessionSubagentArtifact, len(artifacts))
	for i, artifact := range artifacts {
		out[i] = SessionSubagentArtifact{SessionPath: artifact.SessionPath, MetaPath: artifact.MetaPath, EffectPath: artifact.EffectPath}
	}
	return out, nil
}

// ContinueSessionPath returns the persistence target for a rebuilt controller.
// It keeps the store's naming and continuation policy behind the control layer.
func ContinueSessionPath(previousPath, sessionDir, label string) string {
	return agent.ContinueSessionPath(previousPath, sessionDir, label)
}

// SessionCleanupPending reports whether a logically removed session is hidden
// while its background artifacts finish tearing down.
func SessionCleanupPending(path string) bool { return agent.IsCleanupPending(path) }

// MarkSessionCleanupPending records a delayed destructive operation.
func MarkSessionCleanupPending(path, operation string) error {
	return agent.MarkCleanupPending(path, operation)
}

// ClearSessionCleanupPending removes a delayed-cleanup marker.
func ClearSessionCleanupPending(path string) error { return agent.ClearCleanupPending(path) }

// DeleteSessionSubagents permanently rejects managed writer worktrees before
// removing persisted sub-agent records owned by session.
func DeleteSessionSubagents(sessionDir, sessionPath string) error {
	return agent.DeleteSubagentsByParentWithWorktrees(context.Background(), sessionDir, agent.BranchID(sessionPath), config.ManagedWorktreeDir())
}

// DeleteTrashedSessionSubagents permanently rejects managed writer worktrees
// represented by metadata inside a validated desktop trash item.
func DeleteTrashedSessionSubagents(itemDir string) error {
	return agent.DeleteSubagentsInDirWithWorktrees(context.Background(), filepath.Join(itemDir, "subagents"), config.ManagedWorktreeDir())
}

// IsSessionLeaseHeld classifies the stable cross-runtime ownership conflict
// without requiring a transport to import the agent persistence package.
func IsSessionLeaseHeld(err error) bool { return errors.Is(err, agent.ErrSessionLeaseHeld) }

// IsSessionSnapshotConflict classifies a stale runtime save without exposing
// the persistence error sentinel to transports.
func IsSessionSnapshotConflict(err error) bool {
	return errors.Is(err, agent.ErrSessionSnapshotConflict)
}

func SessionLeaseHeldByOtherRuntime(path string) bool {
	return agent.SessionLeaseHeldByOtherRuntime(path)
}

func SessionLeaseHeld(path string) bool { return agent.SessionLeaseHeld(path) }

func CanonicalSessionPath(path string) string { return agent.CanonicalSessionPath(path) }

func NewSessionPath(dir, label string) string { return agent.NewSessionPath(dir, label) }

// SessionLeaseAvailableForRebuild probes path without changing a frontend's
// owned keeper. A lease held by this process is treated as retryable because it
// is normally the tab being rebuilt; the real bind still decides atomically.
func SessionLeaseAvailableForRebuild(path string) bool {
	lease, err := agent.TryAcquireSessionLease(path)
	if err != nil {
		var leaseErr *agent.SessionLeaseError
		if errors.As(err, &leaseErr) && leaseErr.Info != nil && leaseErr.Info.PID == os.Getpid() {
			if host, _ := os.Hostname(); leaseErr.Info.Hostname == host {
				return true
			}
		}
		return !errors.Is(err, agent.ErrSessionLeaseHeld)
	}
	lease.Release()
	return true
}

// ReclaimableRecoverySessions applies the store's fixed safety grace period.
func ReclaimableRecoverySessions(dir string, now time.Time) ([]string, error) {
	return agent.ReclaimableRecoveryBranches(dir, now, agent.RecoveryGCGracePeriod)
}

// SessionHasContent loads the canonical event-log-aware session view.
func SessionHasContent(path string) (bool, error) {
	session, err := agent.LoadSession(path)
	if err != nil {
		return false, err
	}
	return session.HasContent(), nil
}

func SessionsShareContent(firstPath, secondPath string) (bool, error) {
	return agent.SessionsShareContent(firstPath, secondPath)
}

// SessionCleanupPendingInfo is the transport-safe portion of a durable cleanup
// marker used by Desktop trash recovery.
type SessionCleanupPendingInfo struct {
	SessionPath string
	Operation   string
}

func ReconcileSessionCleanupPendingDetailed(dir string, remove func(SessionCleanupPendingInfo) error) error {
	return agent.ReconcileCleanupPending(dir, func(item agent.CleanupPendingInfo) error {
		return remove(SessionCleanupPendingInfo{SessionPath: item.SessionPath, Operation: item.Meta.Operation})
	})
}

// ReconcileSessionCleanupPending retries durable cleanup markers and exposes
// only the owning transcript path to the transport-specific remover.
func ReconcileSessionCleanupPending(dir string, remove func(sessionPath string) error) error {
	return agent.ReconcileCleanupPending(dir, func(item agent.CleanupPendingInfo) error {
		return remove(item.SessionPath)
	})
}

// SessionName returns the transcript filename without its extension.
func SessionName(path string) string {
	base := filepath.Base(path)
	return base[:len(base)-len(filepath.Ext(base))]
}
