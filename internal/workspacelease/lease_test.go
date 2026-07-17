package workspacelease

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOwnerSerializesIndependentSessions(t *testing.T) {
	root := t.TempDir()
	locks := t.TempDir()
	first, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	first.BeginRun()
	second.BeginRun()
	defer second.EndRun()
	if err := first.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := second.AcquireWrite(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second AcquireWrite = %v, want deadline", err)
	}
	first.EndRun()
	if err := second.AcquireWrite(context.Background()); err != nil {
		t.Fatalf("second AcquireWrite after release: %v", err)
	}
}

func TestOwnerIsReentrantAcrossParticipatingRuns(t *testing.T) {
	o, err := New(t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	o.BeginRun()
	o.BeginRun()
	if err := o.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := o.AcquireWrite(ctx); err != nil {
		t.Fatalf("reentrant AcquireWrite: %v", err)
	}
	o.EndRun()
	o.EndRun()
}

func TestRetainUntilKeepsLeaseAfterRun(t *testing.T) {
	root := t.TempDir()
	locks := t.TempDir()
	first, _ := New(root, locks, nil)
	second, _ := New(root, locks, nil)
	first.BeginRun()
	second.BeginRun()
	defer second.EndRun()
	if err := first.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	first.RetainUntil(done)
	first.EndRun()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := second.AcquireWrite(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second AcquireWrite while retained = %v, want deadline", err)
	}
	close(done)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := second.AcquireWrite(ctx2); err != nil {
		t.Fatalf("second AcquireWrite after retained job: %v", err)
	}
}

func TestCanonicalWorkspaceFoldsRepositorySubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	gotRoot, err := CanonicalWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	gotSub, err := CanonicalWorkspace(sub)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != gotSub {
		t.Fatalf("canonical root %q != subdir %q", gotRoot, gotSub)
	}
}

func TestCanonicalWorkspaceFoldsCaseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path semantics")
	}
	root := t.TempDir()
	lower, err := CanonicalWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	upper, err := CanonicalWorkspace(strings.ToUpper(root[:1]) + root[1:])
	if err != nil {
		t.Fatal(err)
	}
	if lower != upper {
		t.Fatalf("canonical case mismatch: %q != %q", lower, upper)
	}
}

func TestWorkspaceLeaseSerializesAcrossProcesses(t *testing.T) {
	if os.Getenv("REAMES_WORKSPACE_LEASE_HELPER") == "1" {
		return
	}
	root := t.TempDir()
	locks := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	release := filepath.Join(t.TempDir(), "release")
	cmd := exec.Command(os.Args[0], "-test.run=TestWorkspaceLeaseProcessHelper")
	cmd.Env = append(os.Environ(),
		"REAMES_WORKSPACE_LEASE_HELPER=1",
		"REAMES_WORKSPACE_LEASE_ROOT="+root,
		"REAMES_WORKSPACE_LEASE_DIR="+locks,
		"REAMES_WORKSPACE_LEASE_READY="+ready,
		"REAMES_WORKSPACE_LEASE_RELEASE="+release,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.WriteFile(release, []byte("release"), 0o600)
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not acquire workspace lease")
		}
		time.Sleep(20 * time.Millisecond)
	}
	owner, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.BeginRun()
	defer owner.EndRun()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := owner.AcquireWrite(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cross-process AcquireWrite = %v, want deadline", err)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := owner.AcquireWrite(context.Background()); err != nil {
		t.Fatalf("AcquireWrite after helper exit: %v", err)
	}
}

func TestWorkspaceLeaseProcessHelper(t *testing.T) {
	if os.Getenv("REAMES_WORKSPACE_LEASE_HELPER") != "1" {
		return
	}
	owner, err := New(os.Getenv("REAMES_WORKSPACE_LEASE_ROOT"), os.Getenv("REAMES_WORKSPACE_LEASE_DIR"), nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.BeginRun()
	if err := owner.AcquireWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("REAMES_WORKSPACE_LEASE_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(os.Getenv("REAMES_WORKSPACE_LEASE_RELEASE")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for release")
		}
		time.Sleep(20 * time.Millisecond)
	}
	owner.EndRun()
}
