// Package worktree manages durable Git workspaces used by writer-capable
// subagents. Worktrees live under Reames Agent state, outside source
// repositories, and remain explicit delivery artifacts until accepted or
// rejected.
package worktree

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/diff"
	"reames-agent/internal/proc"
)

const (
	gitProbeTimeout       = 15 * time.Second
	gitWorktreeAddTimeout = 5 * time.Minute
	gitMutationTimeout    = 2 * time.Minute
)

// Availability describes whether a workspace can be isolated with Git.
type Availability struct {
	Available           bool   `json:"available"`
	Reason              string `json:"reason,omitempty"`
	RepoRoot            string `json:"repoRoot,omitempty"`
	SourceWorkspaceRoot string `json:"sourceWorkspaceRoot,omitempty"`
	Branch              string `json:"branch,omitempty"`
	SourceDirty         bool   `json:"sourceDirty,omitempty"`
}

// Assignment identifies one durable writer workspace.
type Assignment struct {
	WorkspaceRoot       string `json:"workspaceRoot"`
	WorktreeRoot        string `json:"worktreeRoot"`
	RepoRoot            string `json:"repoRoot"`
	SourceWorkspaceRoot string `json:"sourceWorkspaceRoot"`
	Branch              string `json:"branch"`
	BaseHead            string `json:"baseHead"`
	Prefix              string `json:"prefix,omitempty"`
	SourceDirty         bool   `json:"sourceDirty,omitempty"`
}

// Commit describes a commit created inside a delivery branch.
type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// Snapshot is the repository-derived delivery manifest. Git is the source of
// truth; persisted metadata only caches this projection.
type Snapshot struct {
	Head        string        `json:"head"`
	Dirty       bool          `json:"dirty"`
	Registered  bool          `json:"registered"`
	Changes     []diff.Change `json:"changes,omitempty"`
	Commits     []Commit      `json:"commits,omitempty"`
	PatchDigest string        `json:"patchDigest,omitempty"`
}

// Transaction records a source-workspace acceptance action and the exact
// state required for fail-closed rollback.
type Transaction struct {
	Kind             string    `json:"kind"` // apply | merge
	CreatedAt        time.Time `json:"createdAt"`
	SourceHeadBefore string    `json:"sourceHeadBefore"`
	SourceHeadAfter  string    `json:"sourceHeadAfter"`
	StatusBefore     string    `json:"statusBefore"`
	StatusAfter      string    `json:"statusAfter"`
	DeliveryCommit   string    `json:"deliveryCommit"`
	RollbackCommit   string    `json:"rollbackCommit,omitempty"`
}

const (
	RecoveryBefore    = "before"
	RecoveryCompleted = "completed"
	RecoveryAmbiguous = "ambiguous"
)

type inspection struct {
	Availability
	head      string
	prefix    string
	commonDir string
}

var creationLocks = struct {
	sync.Mutex
	byRepo map[string]*sync.Mutex
}{byRepo: map[string]*sync.Mutex{}}

// Inspect checks Git and repository prerequisites without changing state.
func Inspect(ctx context.Context, workspaceRoot string) Availability {
	info, err := inspect(ctx, workspaceRoot)
	if err != nil {
		return Availability{Available: false, Reason: err.Error()}
	}
	return info.Availability
}

// Create creates a branch and linked worktree from committed HEAD. Source
// uncommitted changes are never copied; SourceDirty makes that omission visible.
func Create(ctx context.Context, workspaceRoot, managedRoot, ref string) (Assignment, error) {
	info, err := inspect(ctx, workspaceRoot)
	if err != nil {
		return Assignment{}, err
	}
	managedRoot = strings.TrimSpace(managedRoot)
	if managedRoot == "" {
		return Assignment{}, errors.New("managed worktree storage is unavailable")
	}
	if err := os.MkdirAll(managedRoot, 0o700); err != nil {
		return Assignment{}, fmt.Errorf("create managed worktree storage: %w", err)
	}

	creationLocks.Lock()
	lock := creationLocks.byRepo[info.commonDir]
	if lock == nil {
		lock = &sync.Mutex{}
		creationLocks.byRepo[info.commonDir] = lock
	}
	creationLocks.Unlock()
	lock.Lock()
	defer lock.Unlock()

	repoSum := sha256.Sum256([]byte(info.commonDir))
	repoKey := hex.EncodeToString(repoSum[:8])
	repoBase := safePathComponent(filepath.Base(info.RepoRoot))
	if repoBase == "" {
		repoBase = "repository"
	}
	refPart := safePathComponent(strings.TrimPrefix(strings.TrimSpace(ref), "sa_"))
	if len(refPart) > 48 {
		refPart = refPart[len(refPart)-48:]
	}

	for attempt := 0; attempt < 5; attempt++ {
		id, randomErr := randomID()
		if randomErr != nil {
			return Assignment{}, randomErr
		}
		branchSuffix := id
		if refPart != "" {
			branchSuffix = refPart + "-" + id
		}
		branch := "reames/subagent-" + branchSuffix
		worktreeRoot := filepath.Join(managedRoot, repoKey, id, repoBase)
		if !IsManagedPath(worktreeRoot, managedRoot) {
			return Assignment{}, errors.New("refusing worktree destination outside managed storage")
		}
		if _, statErr := os.Stat(worktreeRoot); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			return Assignment{}, fmt.Errorf("inspect worktree destination: %w", statErr)
		}
		if err := os.MkdirAll(filepath.Dir(worktreeRoot), 0o700); err != nil {
			return Assignment{}, fmt.Errorf("create worktree parent: %w", err)
		}

		_, stderr, addErr := runGit(ctx, info.RepoRoot, "worktree", "add", "-b", branch, worktreeRoot, info.head)
		if addErr != nil {
			if strings.Contains(strings.ToLower(stderr), "already exists") {
				continue
			}
			return Assignment{}, fmt.Errorf("create Git worktree: %w%s", addErr, stderrSuffix(stderr))
		}
		selectedRoot := worktreeRoot
		prefix := filepath.FromSlash(strings.Trim(strings.TrimSpace(info.prefix), "/"))
		if prefix != "" && prefix != "." {
			selectedRoot = filepath.Join(worktreeRoot, prefix)
			st, statErr := os.Stat(selectedRoot)
			if statErr != nil || !st.IsDir() {
				_ = Remove(context.Background(), Assignment{WorktreeRoot: worktreeRoot, RepoRoot: info.RepoRoot, Branch: branch}, managedRoot)
				return Assignment{}, fmt.Errorf("created worktree is missing selected project subdirectory %q", prefix)
			}
		}
		return Assignment{
			WorkspaceRoot:       selectedRoot,
			WorktreeRoot:        worktreeRoot,
			RepoRoot:            info.RepoRoot,
			SourceWorkspaceRoot: info.SourceWorkspaceRoot,
			Branch:              branch,
			BaseHead:            info.head,
			Prefix:              filepath.ToSlash(prefix),
			SourceDirty:         info.SourceDirty,
		}, nil
	}
	return Assignment{}, errors.New("could not allocate a unique subagent worktree")
}

// Seal stages all selected-workspace changes and creates a stable delivery
// commit. Existing child commits are preserved; the returned HEAD represents
// the complete base-to-delivery patch.
func Seal(ctx context.Context, a Assignment, ref string) (string, error) {
	if err := validateAssignment(ctx, a); err != nil {
		return "", err
	}
	paths, err := changedRepoPaths(ctx, a)
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		if !pathWithinPrefix(path, a.Prefix) {
			return "", fmt.Errorf("delivery changed path %q outside selected workspace %q", path, a.Prefix)
		}
	}
	status, _, err := runGit(ctx, a.WorktreeRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return "", fmt.Errorf("inspect delivery status: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		pathspec := "."
		if p := strings.Trim(strings.TrimSpace(a.Prefix), "/"); p != "" {
			pathspec = p
		}
		if _, stderr, err := runGit(ctx, a.WorktreeRoot, "add", "-A", "--", pathspec); err != nil {
			return "", fmt.Errorf("stage delivery: %w%s", err, stderrSuffix(stderr))
		}
		staged, _, err := runGit(ctx, a.WorktreeRoot, "diff", "--cached", "--quiet")
		if err != nil || strings.TrimSpace(staged) != "" {
			message := "reames: seal subagent delivery"
			if strings.TrimSpace(ref) != "" {
				message += " " + strings.TrimSpace(ref)
			}
			if _, stderr, err := runGit(ctx, a.WorktreeRoot, "-c", "user.name=Reames Agent", "-c", "user.email=reames-agent@localhost", "commit", "-m", message); err != nil {
				return "", fmt.Errorf("commit delivery: %w%s", err, stderrSuffix(stderr))
			}
		}
	}
	head, _, err := runGit(ctx, a.WorktreeRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve delivery HEAD: %w", err)
	}
	return strings.TrimSpace(head), nil
}

// Snapshot derives the current manifest directly from Git and the worktree.
func SnapshotDelivery(ctx context.Context, a Assignment) (Snapshot, error) {
	if err := validateAssignment(ctx, a); err != nil {
		return Snapshot{}, err
	}
	registered, err := Registered(ctx, a.RepoRoot, a.WorktreeRoot)
	if err != nil {
		return Snapshot{}, err
	}
	head, _, err := runGit(ctx, a.WorktreeRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve delivery HEAD: %w", err)
	}
	head = strings.TrimSpace(head)
	status, _, err := runGit(ctx, a.WorktreeRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect delivery status: %w", err)
	}
	paths, err := changedRepoPaths(ctx, a)
	if err != nil {
		return Snapshot{}, err
	}
	changes := make([]diff.Change, 0, len(paths))
	for _, repoPath := range paths {
		if !pathWithinPrefix(repoPath, a.Prefix) {
			return Snapshot{}, fmt.Errorf("delivery changed path %q outside selected workspace %q", repoPath, a.Prefix)
		}
		workspacePath := trimPrefixPath(repoPath, a.Prefix)
		oldBytes, _, oldErr := runGitBytes(ctx, a.WorktreeRoot, "show", a.BaseHead+":"+filepath.ToSlash(repoPath))
		if oldErr != nil {
			oldBytes = nil
		}
		newBytes, newErr := os.ReadFile(filepath.Join(a.WorktreeRoot, filepath.FromSlash(repoPath)))
		kind := diff.Modify
		switch {
		case oldErr != nil && newErr == nil:
			kind = diff.Create
		case oldErr == nil && os.IsNotExist(newErr):
			kind = diff.Delete
		case newErr != nil:
			return Snapshot{}, fmt.Errorf("read delivery file %q: %w", repoPath, newErr)
		}
		changes = append(changes, diff.Build(filepath.ToSlash(workspacePath), string(oldBytes), string(newBytes), kind))
	}
	commits, err := listCommits(ctx, a)
	if err != nil {
		return Snapshot{}, err
	}
	patch, _, err := runGitBytes(ctx, a.WorktreeRoot, "diff", "--binary", "--full-index", a.BaseHead, head)
	if err != nil {
		return Snapshot{}, fmt.Errorf("render delivery patch: %w", err)
	}
	digest := ""
	if len(patch) > 0 {
		sum := sha256.Sum256(patch)
		digest = hex.EncodeToString(sum[:])
	}
	return Snapshot{
		Head:        head,
		Dirty:       strings.TrimSpace(status) != "",
		Registered:  registered,
		Changes:     changes,
		Commits:     commits,
		PatchDigest: digest,
	}, nil
}

// Apply applies a sealed delivery to a clean source worktree without creating
// a source commit. The resulting changes are staged. Failures restore the exact
// clean pre-state before returning.
func Apply(ctx context.Context, a Assignment, deliveryCommit string) (Transaction, error) {
	tx, err := PrepareTransaction(ctx, a, deliveryCommit, "apply")
	if err != nil {
		return Transaction{}, err
	}
	return ApplyPrepared(ctx, a, tx)
}

// PrepareTransaction validates a ready delivery and captures the exact clean
// source state. Hosts persist this intent before mutating the source workspace.
func PrepareTransaction(ctx context.Context, a Assignment, deliveryCommit, kind string) (Transaction, error) {
	if err := validateReadyDelivery(ctx, a, deliveryCommit); err != nil {
		return Transaction{}, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "apply" && kind != "merge" {
		return Transaction{}, fmt.Errorf("unsupported delivery transaction %q", kind)
	}
	beforeHead, beforeStatus, err := cleanSourceState(ctx, a)
	if err != nil {
		return Transaction{}, err
	}
	return Transaction{Kind: kind, CreatedAt: time.Now().UTC(), SourceHeadBefore: beforeHead, StatusBefore: beforeStatus, DeliveryCommit: strings.TrimSpace(deliveryCommit)}, nil
}

// ApplyPrepared applies a transaction whose clean pre-state has already been
// persisted. Source drift between preparation and mutation is rejected.
func ApplyPrepared(ctx context.Context, a Assignment, tx Transaction) (Transaction, error) {
	if tx.Kind != "apply" {
		return tx, fmt.Errorf("prepared transaction kind is %q, want apply", tx.Kind)
	}
	if err := validateReadyDelivery(ctx, a, tx.DeliveryCommit); err != nil {
		return tx, err
	}
	if err := validatePreparedSource(ctx, a.RepoRoot, tx); err != nil {
		return tx, err
	}
	patch, _, err := runGitBytes(ctx, a.WorktreeRoot, "diff", "--binary", "--full-index", a.BaseHead, tx.DeliveryCommit)
	if err != nil {
		return tx, fmt.Errorf("render delivery patch: %w", err)
	}
	if len(patch) == 0 {
		return tx, errors.New("delivery has no changes to apply")
	}
	if _, stderr, err := runGitInput(ctx, a.RepoRoot, patch, "apply", "--check", "--index", "--3way", "-"); err != nil {
		return tx, fmt.Errorf("delivery does not apply cleanly: %w%s", err, stderrSuffix(stderr))
	}
	if _, stderr, err := runGitInput(ctx, a.RepoRoot, patch, "apply", "--index", "--3way", "-"); err != nil {
		_ = restoreCleanSource(context.Background(), a.RepoRoot, tx.SourceHeadBefore)
		return tx, fmt.Errorf("apply delivery: %w%s", err, stderrSuffix(stderr))
	}
	afterHead, _, err := runGit(ctx, a.RepoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		_ = restoreCleanSource(context.Background(), a.RepoRoot, tx.SourceHeadBefore)
		return tx, err
	}
	afterStatus, err := statusDigest(ctx, a.RepoRoot)
	if err != nil {
		_ = restoreCleanSource(context.Background(), a.RepoRoot, tx.SourceHeadBefore)
		return tx, err
	}
	tx.SourceHeadAfter = strings.TrimSpace(afterHead)
	tx.StatusAfter = afterStatus
	return tx, nil
}

// Merge creates a no-fast-forward merge commit in a clean source worktree.
func Merge(ctx context.Context, a Assignment, deliveryCommit string) (Transaction, error) {
	tx, err := PrepareTransaction(ctx, a, deliveryCommit, "merge")
	if err != nil {
		return Transaction{}, err
	}
	return MergePrepared(ctx, a, tx)
}

// MergePrepared merges a transaction whose clean pre-state has already been
// persisted. Source drift between preparation and mutation is rejected.
func MergePrepared(ctx context.Context, a Assignment, tx Transaction) (Transaction, error) {
	if tx.Kind != "merge" {
		return tx, fmt.Errorf("prepared transaction kind is %q, want merge", tx.Kind)
	}
	if err := validateReadyDelivery(ctx, a, tx.DeliveryCommit); err != nil {
		return tx, err
	}
	if err := validatePreparedSource(ctx, a.RepoRoot, tx); err != nil {
		return tx, err
	}
	_, stderr, err := runGit(ctx, a.RepoRoot, "-c", "user.name=Reames Agent", "-c", "user.email=reames-agent@localhost", "merge", "--no-ff", "--no-edit", tx.DeliveryCommit)
	if err != nil {
		_, _, _ = runGit(context.Background(), a.RepoRoot, "merge", "--abort")
		_ = restoreCleanSource(context.Background(), a.RepoRoot, tx.SourceHeadBefore)
		return tx, fmt.Errorf("merge delivery: %w%s", err, stderrSuffix(stderr))
	}
	afterHead, _, err := runGit(ctx, a.RepoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return tx, err
	}
	afterStatus, err := statusDigest(ctx, a.RepoRoot)
	if err != nil {
		return tx, err
	}
	tx.SourceHeadAfter = strings.TrimSpace(afterHead)
	tx.StatusAfter = afterStatus
	return tx, nil
}

// Rollback reverses an Apply or Merge only when the source still matches the
// exact post-action state. Drift is rejected rather than overwritten.
func Rollback(ctx context.Context, a Assignment, tx Transaction) (Transaction, error) {
	currentHead, _, err := runGit(ctx, a.RepoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return tx, err
	}
	currentStatus, err := statusDigest(ctx, a.RepoRoot)
	if err != nil {
		return tx, err
	}
	if strings.TrimSpace(currentHead) != tx.SourceHeadAfter || currentStatus != tx.StatusAfter {
		return tx, errors.New("source workspace changed after delivery; rollback refused")
	}
	switch tx.Kind {
	case "apply":
		if err := restoreCleanSource(ctx, a.RepoRoot, tx.SourceHeadBefore); err != nil {
			return tx, fmt.Errorf("rollback applied delivery: %w", err)
		}
	case "merge":
		if _, stderr, err := runGit(ctx, a.RepoRoot, "-c", "user.name=Reames Agent", "-c", "user.email=reames-agent@localhost", "revert", "-m", "1", "--no-edit", tx.SourceHeadAfter); err != nil {
			_, _, _ = runGit(context.Background(), a.RepoRoot, "revert", "--abort")
			return tx, fmt.Errorf("revert merged delivery: %w%s", err, stderrSuffix(stderr))
		}
		head, _, err := runGit(ctx, a.RepoRoot, "rev-parse", "--verify", "HEAD")
		if err != nil {
			return tx, err
		}
		tx.RollbackCommit = strings.TrimSpace(head)
	default:
		return tx, fmt.Errorf("unsupported delivery transaction %q", tx.Kind)
	}
	return tx, nil
}

// Remove rejects a managed worktree and deletes its branch. Git's registry is
// consulted before any residual directory cleanup.
func Remove(ctx context.Context, a Assignment, managedRoot string) error {
	if strings.TrimSpace(a.WorktreeRoot) == "" || strings.TrimSpace(a.RepoRoot) == "" {
		return errors.New("worktree identity is incomplete")
	}
	if !IsManagedPath(a.WorktreeRoot, managedRoot) {
		return fmt.Errorf("refusing to remove unmanaged worktree %q", a.WorktreeRoot)
	}
	_, stderr, removeErr := runGit(ctx, a.RepoRoot, "worktree", "remove", "--force", a.WorktreeRoot)
	if removeErr != nil {
		_, _, _ = runGit(context.Background(), a.RepoRoot, "worktree", "prune")
		registered, checkErr := Registered(context.Background(), a.RepoRoot, a.WorktreeRoot)
		if checkErr != nil {
			return errors.Join(fmt.Errorf("remove Git worktree: %w%s", removeErr, stderrSuffix(stderr)), checkErr)
		}
		if registered {
			return fmt.Errorf("remove Git worktree: %w%s", removeErr, stderrSuffix(stderr))
		}
	}
	registered, err := Registered(context.Background(), a.RepoRoot, a.WorktreeRoot)
	if err != nil {
		return err
	}
	if registered {
		return fmt.Errorf("Git still registers worktree %q after removal", a.WorktreeRoot)
	}
	if err := removeManagedResidual(a.WorktreeRoot, managedRoot); err != nil {
		return err
	}
	if strings.TrimSpace(a.Branch) != "" {
		if _, stderr, err := runGit(ctx, a.RepoRoot, "branch", "-D", a.Branch); err != nil && !strings.Contains(strings.ToLower(stderr), "not found") {
			return fmt.Errorf("delete delivery branch %q: %w%s", a.Branch, err, stderrSuffix(stderr))
		}
	}
	return nil
}

// Registered reports whether Git still lists worktreeRoot for repoRoot.
func Registered(ctx context.Context, repoRoot, worktreeRoot string) (bool, error) {
	out, stderr, err := runGit(ctx, repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("list Git worktrees: %w%s", err, stderrSuffix(stderr))
	}
	want, err := canonicalPath(worktreeRoot)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		got, pathErr := canonicalPath(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))
		if pathErr == nil && got == want {
			return true, nil
		}
	}
	return false, nil
}

// Reconcile repairs stale Git registry entries and reports whether a managed
// worktree still exists and is registered.
func Reconcile(ctx context.Context, a Assignment, managedRoot string) (exists, registered bool, err error) {
	if !IsManagedPath(a.WorktreeRoot, managedRoot) {
		return false, false, fmt.Errorf("worktree %q is outside managed storage", a.WorktreeRoot)
	}
	_, statErr := os.Stat(a.WorktreeRoot)
	exists = statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return false, false, statErr
	}
	registered, err = Registered(ctx, a.RepoRoot, a.WorktreeRoot)
	if err != nil {
		return exists, false, err
	}
	if !exists && registered {
		if _, stderr, pruneErr := runGit(ctx, a.RepoRoot, "worktree", "prune"); pruneErr != nil {
			return false, true, fmt.Errorf("prune stale Git worktree: %w%s", pruneErr, stderrSuffix(stderr))
		}
		registered, err = Registered(ctx, a.RepoRoot, a.WorktreeRoot)
	}
	return exists, registered, err
}

// IsManagedPath reports whether path is lexically below managedRoot.
func IsManagedPath(path, managedRoot string) bool {
	path = strings.TrimSpace(path)
	managedRoot = strings.TrimSpace(managedRoot)
	if path == "" || managedRoot == "" {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absManaged, err := filepath.Abs(managedRoot)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(absManaged), filepath.Clean(absPath))
	if err != nil || rel == "." || rel == "" {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func inspect(ctx context.Context, workspaceRoot string) (inspection, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return inspection{}, errors.New("project folder is required")
	}
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return inspection{}, fmt.Errorf("resolve project folder: %w", err)
	}
	absWorkspace = filepath.Clean(absWorkspace)
	st, err := os.Stat(absWorkspace)
	if err != nil {
		return inspection{}, fmt.Errorf("project folder is unavailable: %w", err)
	}
	if !st.IsDir() {
		return inspection{}, errors.New("project path is not a folder")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return inspection{}, errors.New("Git is not installed; writer subagents are unavailable for this workspace")
	}
	repoRoot, stderr, err := runGit(ctx, absWorkspace, "rev-parse", "--show-toplevel")
	if err != nil {
		return inspection{}, fmt.Errorf("project folder is not inside a Git repository%s", stderrSuffix(stderr))
	}
	repoRoot = filepath.Clean(strings.TrimSpace(repoRoot))
	bare, _, err := runGit(ctx, absWorkspace, "rev-parse", "--is-bare-repository")
	if err != nil || strings.EqualFold(strings.TrimSpace(bare), "true") {
		return inspection{}, errors.New("bare Git repositories cannot host writer subagents")
	}
	head, _, err := runGit(ctx, repoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(head) == "" {
		return inspection{}, errors.New("the Git repository needs an initial commit before a writer subagent can run")
	}
	head = strings.TrimSpace(head)
	prefix, _, err := runGit(ctx, absWorkspace, "rev-parse", "--show-prefix")
	if err != nil {
		return inspection{}, fmt.Errorf("resolve selected project path inside repository: %w", err)
	}
	prefix = strings.TrimSpace(prefix)
	if prefix != "" {
		objectType, _, objectErr := runGit(ctx, repoRoot, "cat-file", "-t", head+":"+strings.TrimSuffix(prefix, "/"))
		if objectErr != nil || strings.TrimSpace(objectType) != "tree" {
			return inspection{}, errors.New("the selected project folder is not present in committed HEAD")
		}
	}
	commonDir, _, err := runGit(ctx, repoRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return inspection{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	commonDir = strings.TrimSpace(commonDir)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repoRoot, commonDir)
	}
	branch, _, _ := runGit(ctx, repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	status, _, statusErr := runGit(ctx, repoRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if statusErr != nil {
		return inspection{}, fmt.Errorf("inspect Git working tree: %w", statusErr)
	}
	return inspection{
		Availability: Availability{Available: true, RepoRoot: repoRoot, SourceWorkspaceRoot: absWorkspace, Branch: strings.TrimSpace(branch), SourceDirty: strings.TrimSpace(status) != ""},
		head:         head,
		prefix:       prefix,
		commonDir:    filepath.Clean(commonDir),
	}, nil
}

func validateAssignment(ctx context.Context, a Assignment) error {
	if strings.TrimSpace(a.WorktreeRoot) == "" || strings.TrimSpace(a.RepoRoot) == "" || strings.TrimSpace(a.BaseHead) == "" || strings.TrimSpace(a.Branch) == "" {
		return errors.New("delivery worktree identity is incomplete")
	}
	st, err := os.Stat(a.WorktreeRoot)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("delivery worktree is unavailable: %w", err)
	}
	branch, _, err := runGit(ctx, a.WorktreeRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != strings.TrimSpace(a.Branch) {
		return fmt.Errorf("delivery worktree branch changed: got %q want %q", strings.TrimSpace(branch), a.Branch)
	}
	return nil
}

func validateReadyDelivery(ctx context.Context, a Assignment, deliveryCommit string) error {
	if err := validateAssignment(ctx, a); err != nil {
		return err
	}
	deliveryCommit = strings.TrimSpace(deliveryCommit)
	if deliveryCommit == "" {
		return errors.New("delivery is not sealed")
	}
	head, _, err := runGit(ctx, a.WorktreeRoot, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(head) != deliveryCommit {
		return errors.New("delivery HEAD no longer matches its sealed commit")
	}
	status, _, err := runGit(ctx, a.WorktreeRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("delivery worktree changed after sealing")
	}
	return nil
}

func cleanSourceState(ctx context.Context, a Assignment) (head, status string, err error) {
	head, _, err = runGit(ctx, a.RepoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", "", err
	}
	head = strings.TrimSpace(head)
	status, err = statusDigest(ctx, a.RepoRoot)
	if err != nil {
		return "", "", err
	}
	raw, _, err := runGitBytes(ctx, a.RepoRoot, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return "", "", err
	}
	if len(raw) != 0 {
		return "", "", errors.New("source workspace is dirty; commit, stash, or discard its changes before accepting a delivery")
	}
	return head, status, nil
}

func validatePreparedSource(ctx context.Context, repoRoot string, tx Transaction) error {
	if strings.TrimSpace(tx.SourceHeadBefore) == "" || strings.TrimSpace(tx.StatusBefore) == "" || strings.TrimSpace(tx.DeliveryCommit) == "" || tx.CreatedAt.IsZero() {
		return errors.New("prepared delivery transaction is incomplete")
	}
	head, _, err := runGit(ctx, repoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	status, err := statusDigest(ctx, repoRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) != tx.SourceHeadBefore || status != tx.StatusBefore {
		return errors.New("source workspace changed after delivery acceptance was prepared")
	}
	return nil
}

// RecoverPrepared classifies an interrupted persisted acceptance intent. It
// only declares completion when Git proves a clean merge with the expected two
// parents; an apply with any post-intent change remains ambiguous.
func RecoverPrepared(ctx context.Context, a Assignment, tx Transaction) (string, Transaction, error) {
	if tx.SourceHeadAfter != "" || tx.StatusAfter != "" {
		return RecoveryCompleted, tx, nil
	}
	if tx.Kind != "apply" && tx.Kind != "merge" {
		return "", tx, fmt.Errorf("unsupported prepared transaction %q", tx.Kind)
	}
	head, _, err := runGit(ctx, a.RepoRoot, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", tx, err
	}
	head = strings.TrimSpace(head)
	status, err := statusDigest(ctx, a.RepoRoot)
	if err != nil {
		return "", tx, err
	}
	if head == tx.SourceHeadBefore && status == tx.StatusBefore {
		return RecoveryBefore, tx, nil
	}
	if tx.Kind == "merge" && status == tx.StatusBefore {
		parents, _, parentErr := runGit(ctx, a.RepoRoot, "rev-list", "--parents", "-n", "1", head)
		if parentErr != nil {
			return "", tx, parentErr
		}
		fields := strings.Fields(parents)
		if len(fields) == 3 && fields[0] == head && fields[1] == tx.SourceHeadBefore && fields[2] == tx.DeliveryCommit {
			tx.SourceHeadAfter = head
			tx.StatusAfter = status
			return RecoveryCompleted, tx, nil
		}
	}
	return RecoveryAmbiguous, tx, nil
}

func restoreCleanSource(ctx context.Context, repoRoot, head string) error {
	if _, stderr, err := runGit(ctx, repoRoot, "reset", "--hard", head); err != nil {
		return fmt.Errorf("reset source: %w%s", err, stderrSuffix(stderr))
	}
	if _, stderr, err := runGit(ctx, repoRoot, "clean", "-fd"); err != nil {
		return fmt.Errorf("clean source: %w%s", err, stderrSuffix(stderr))
	}
	return nil
}

func statusDigest(ctx context.Context, dir string) (string, error) {
	raw, _, err := runGitBytes(ctx, dir, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func changedRepoPaths(ctx context.Context, a Assignment) ([]string, error) {
	tracked, _, err := runGitBytes(ctx, a.WorktreeRoot, "diff", "--no-renames", "--name-only", "-z", a.BaseHead)
	if err != nil {
		return nil, fmt.Errorf("list delivery changes: %w", err)
	}
	untracked, _, err := runGitBytes(ctx, a.WorktreeRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list delivery untracked files: %w", err)
	}
	seen := map[string]bool{}
	var paths []string
	for _, raw := range append(bytes.Split(tracked, []byte{0}), bytes.Split(untracked, []byte{0})...) {
		path := filepath.ToSlash(strings.TrimSpace(string(raw)))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func listCommits(ctx context.Context, a Assignment) ([]Commit, error) {
	out, _, err := runGitBytes(ctx, a.WorktreeRoot, "log", "--format=%H%x00%s%x00", a.BaseHead+"..HEAD")
	if err != nil {
		return nil, fmt.Errorf("list delivery commits: %w", err)
	}
	parts := bytes.Split(out, []byte{0})
	var commits []Commit
	for i := 0; i+1 < len(parts); i += 2 {
		hash := strings.TrimSpace(string(parts[i]))
		if hash == "" {
			continue
		}
		commits = append(commits, Commit{Hash: hash, Subject: strings.TrimSpace(string(parts[i+1]))})
	}
	return commits, nil
}

func pathWithinPrefix(path, prefix string) bool {
	path = strings.Trim(filepath.ToSlash(path), "/")
	prefix = strings.Trim(filepath.ToSlash(prefix), "/")
	return prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

func trimPrefixPath(path, prefix string) string {
	path = strings.Trim(filepath.ToSlash(path), "/")
	prefix = strings.Trim(filepath.ToSlash(prefix), "/")
	if prefix == "" {
		return path
	}
	return strings.TrimPrefix(strings.TrimPrefix(path, prefix), "/")
}

func removeManagedResidual(path, managedRoot string) error {
	if !IsManagedPath(path, managedRoot) {
		return fmt.Errorf("refusing residual cleanup outside managed storage: %q", path)
	}
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = os.RemoveAll(path)
		if err == nil {
			if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
				return nil
			}
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return fmt.Errorf("remove managed worktree residual %q: %w", path, err)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	// Git for Windows records the physical target of directory junctions while
	// callers commonly retain the logical path. Resolve the nearest existing
	// ancestor so stale/missing worktrees still compare against Git's registry.
	cursor := abs
	var suffix []string
	for {
		if resolved, resolveErr := filepath.EvalSymlinks(cursor); resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			abs = filepath.Clean(resolved)
			break
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			break
		}
		suffix = append(suffix, filepath.Base(cursor))
		cursor = parent
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(filepath.ToSlash(abs))
	}
	return abs, nil
}

func runGit(parent context.Context, dir string, args ...string) (stdout, stderr string, err error) {
	out, stderr, err := runGitBytes(parent, dir, args...)
	return string(out), stderr, err
}

func runGitBytes(parent context.Context, dir string, args ...string) (stdout []byte, stderr string, err error) {
	return runGitInput(parent, dir, nil, args...)
}

func runGitInput(parent context.Context, dir string, input []byte, args ...string) (stdout []byte, stderr string, err error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, gitTimeout(args))
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", gitCommandArgs(runtime.GOOS, dir, args...)...)
	proc.HideWindow(cmd)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return outBuf.Bytes(), strings.TrimSpace(errBuf.String()), err
}

func gitTimeout(args []string) time.Duration {
	for i := range args {
		if i+1 < len(args) && args[i] == "worktree" && args[i+1] == "add" {
			return gitWorktreeAddTimeout
		}
	}
	for _, arg := range args {
		switch arg {
		case "apply", "merge", "commit", "revert", "reset", "clean":
			return gitMutationTimeout
		}
	}
	return gitProbeTimeout
}

func gitCommandArgs(goos, dir string, args ...string) []string {
	commandArgs := []string{"-c", "core.fsmonitor=false", "-c", "maintenance.auto=false"}
	if goos == "windows" {
		commandArgs = append(commandArgs, "-c", "core.longpaths=true")
	}
	commandArgs = append(commandArgs, "-C", dir)
	return append(commandArgs, args...)
}

func randomID() (string, error) {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate worktree id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func safePathComponent(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32:
			return '-'
		case strings.ContainsRune(`/\\:<>"|?*`, r):
			return '-'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, ". ")
	reserved := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if reserved == "CON" || reserved == "PRN" || reserved == "AUX" || reserved == "NUL" ||
		(len(reserved) == 4 && (strings.HasPrefix(reserved, "COM") || strings.HasPrefix(reserved, "LPT")) && reserved[3] >= '1' && reserved[3] <= '9') {
		name = "_" + name
	}
	return name
}

func stderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	if len(stderr) > 500 {
		stderr = stderr[:500] + "…"
	}
	return ": " + stderr
}
