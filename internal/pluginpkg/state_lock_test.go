package pluginpkg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

const (
	stateLockHelperEnv      = "REAMES_PLUGINPKG_STATE_LOCK_HELPER"
	stateLockHelperOpEnv    = "REAMES_PLUGINPKG_STATE_LOCK_OP"
	stateLockHelperHomeEnv  = "REAMES_PLUGINPKG_STATE_LOCK_HOME"
	stateLockHelperNameEnv  = "REAMES_PLUGINPKG_STATE_LOCK_NAME"
	stateLockHelperReadyEnv = "REAMES_PLUGINPKG_STATE_LOCK_READY"
	stateLockHelperStartEnv = "REAMES_PLUGINPKG_STATE_LOCK_START"
	stateLockHelperTimeout  = 20 * time.Second
	stateLockProcessTimeout = 30 * time.Second
)

// TestStateConcurrentUpsertAndSetEnabled pins that concurrent load-modify-save
// cycles on the state file don't clobber each other: every plugin upserted by a
// racing goroutine must survive, with the enabled flag it was last given.
func TestStateConcurrentUpsertAndSetEnabled(t *testing.T) {
	home := t.TempDir()
	const n = 16

	var wg sync.WaitGroup
	for i := range n {
		name := fmt.Sprintf("plugin-%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Upsert(home, InstalledPlugin{Name: name, Root: "plugins/" + name, Enabled: true}); err != nil {
				t.Errorf("Upsert(%s): %v", name, err)
				return
			}
			if err := SetEnabled(home, name, false); err != nil {
				t.Errorf("SetEnabled(%s): %v", name, err)
			}
		}()
	}
	wg.Wait()

	st, err := LoadState(home)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Plugins) != n {
		t.Fatalf("got %d plugins, want %d (lost updates)", len(st.Plugins), n)
	}
	for _, p := range st.Plugins {
		if p.Enabled {
			t.Errorf("plugin %s lost its SetEnabled update", p.Name)
		}
	}
}

// TestStateConcurrentRemove pins that racing removals each observe their own
// plugin exactly once and leave nothing behind.
func TestStateConcurrentRemove(t *testing.T) {
	home := t.TempDir()
	const n = 8
	for i := range n {
		name := fmt.Sprintf("plugin-%02d", i)
		if err := Upsert(home, InstalledPlugin{Name: name, Root: "plugins/" + name}); err != nil {
			t.Fatalf("Upsert(%s): %v", name, err)
		}
	}

	var wg sync.WaitGroup
	for i := range n {
		name := fmt.Sprintf("plugin-%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			removed, ok, err := Remove(home, name)
			if err != nil {
				t.Errorf("Remove(%s): %v", name, err)
				return
			}
			if !ok || removed.Name != name {
				t.Errorf("Remove(%s) = %+v, ok=%v", name, removed, ok)
			}
		}()
	}
	wg.Wait()

	st, err := LoadState(home)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(st.Plugins) != 0 {
		t.Fatalf("got %d plugins after removing all, want 0", len(st.Plugins))
	}
}

// TestStateLoadDuringSaveNeverSeesTornFile pins the atomic write: a reader
// racing a writer sees either the old state or the new one, never a truncated
// or half-written file (which would surface as a JSON parse error). On Windows
// the rename that publishes a new state file can make a concurrent open fail
// with a transient sharing violation — that is the platform's locking
// behavior, not a torn file, so such reads are retried instead of failed.
func TestStateLoadDuringSaveNeverSeesTornFile(t *testing.T) {
	home := t.TempDir()
	if err := Upsert(home, InstalledPlugin{Name: "seed", Root: "plugins/seed", Enabled: true}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			if err := Upsert(home, InstalledPlugin{Name: "seed", Root: "plugins/seed", Enabled: i%2 == 0}); err != nil {
				t.Errorf("Upsert: %v", err)
				return
			}
		}
	}()
	// Keep the writer's lifetime inside the test body: a t.Fatalf below must
	// not let TempDir cleanup race the still-running writer goroutine.
	defer func() { <-done }()

	for {
		st, err := LoadState(home)
		if err != nil {
			var jsonErr *json.SyntaxError
			if errors.As(err, &jsonErr) {
				t.Fatalf("LoadState saw a torn state file: %v", err)
			}
			if runtime.GOOS == "windows" {
				// Transient sharing violation while the writer renames the
				// new state into place — retry, it is not a torn file.
				continue
			}
			t.Fatalf("LoadState: %v", err)
		}
		if len(st.Plugins) != 1 || st.Plugins[0].Name != "seed" {
			t.Fatalf("state = %+v, want the single seed plugin", st.Plugins)
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func TestStateCrossProcessUpsertAndRemoveDoNotLoseUpdates(t *testing.T) {
	home := t.TempDir()
	const removeCount = 8
	const upsertCount = 12
	for i := range removeCount {
		name := fmt.Sprintf("remove-%02d", i)
		if err := Upsert(home, InstalledPlugin{Name: name, Root: "plugins/" + name}); err != nil {
			t.Fatalf("seed Upsert(%s): %v", name, err)
		}
	}
	if err := Upsert(home, InstalledPlugin{Name: "survivor", Root: "plugins/survivor"}); err != nil {
		t.Fatalf("seed survivor: %v", err)
	}

	operations := make([]stateLockHelperOperation, 0, removeCount+upsertCount)
	for i := range removeCount {
		operations = append(operations, stateLockHelperOperation{op: "remove", name: fmt.Sprintf("remove-%02d", i)})
	}
	for i := range upsertCount {
		operations = append(operations, stateLockHelperOperation{op: "upsert", name: fmt.Sprintf("added-%02d", i)})
	}
	if err := runStateLockHelpers(home, t.TempDir(), operations); err != nil {
		t.Fatal(err)
	}

	st, err := LoadState(home)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	want := map[string]bool{"survivor": true}
	for i := range upsertCount {
		want[fmt.Sprintf("added-%02d", i)] = true
	}
	if len(st.Plugins) != len(want) {
		t.Fatalf("got %d plugins, want %d after cross-process mutations: %+v", len(st.Plugins), len(want), st.Plugins)
	}
	for _, plugin := range st.Plugins {
		if !want[plugin.Name] {
			t.Fatalf("unexpected or unremoved plugin after cross-process mutations: %+v", plugin)
		}
		delete(want, plugin.Name)
	}
	if len(want) != 0 {
		t.Fatalf("cross-process mutations lost plugins: %v", want)
	}
}

func TestStateLockCanBeAcquiredAfterHolderProcessExits(t *testing.T) {
	home := t.TempDir()
	signals := t.TempDir()
	ready := filepath.Join(signals, "holder-ready")
	exit := filepath.Join(signals, "holder-exit")

	ctx, cancel := context.WithTimeout(context.Background(), stateLockProcessTimeout)
	defer cancel()
	holder, output := newStateLockHelperCommand(ctx, home, "hold-exit", "", ready, exit)
	if err := holder.Start(); err != nil {
		t.Fatalf("start lock holder: %v", err)
	}
	holderDone := make(chan error, 1)
	go func() { holderDone <- holder.Wait() }()
	if err := waitForStateLockSignal(ctx, ready); err != nil {
		cancel()
		<-holderDone
		t.Fatalf("wait for holder lock: %v\n%s", err, output.String())
	}
	if err := os.WriteFile(exit, []byte("exit\n"), 0o600); err != nil {
		cancel()
		<-holderDone
		t.Fatalf("release holder process: %v", err)
	}
	select {
	case err := <-holderDone:
		if err != nil {
			t.Fatalf("holder process: %v\n%s", err, output.String())
		}
	case <-ctx.Done():
		t.Fatalf("holder process did not exit: %v", ctx.Err())
	}

	reacquire, reacquireOutput := newStateLockHelperCommand(ctx, home, "upsert", "after-exit", "", "")
	if err := reacquire.Run(); err != nil {
		t.Fatalf("reacquire lock after holder exit: %v\n%s", err, reacquireOutput.String())
	}
	st, err := LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Plugins) != 1 || st.Plugins[0].Name != "after-exit" {
		t.Fatalf("state after lock reacquire = %+v", st.Plugins)
	}
}

// TestStateLockHelperProcess is executed in a separate copy of the test binary.
// It is a no-op during the normal package test run.
func TestStateLockHelperProcess(t *testing.T) {
	if os.Getenv(stateLockHelperEnv) != "1" {
		return
	}
	home := os.Getenv(stateLockHelperHomeEnv)
	op := os.Getenv(stateLockHelperOpEnv)
	name := os.Getenv(stateLockHelperNameEnv)
	ready := os.Getenv(stateLockHelperReadyEnv)
	start := os.Getenv(stateLockHelperStartEnv)
	if home == "" {
		t.Fatal("helper home is empty")
	}

	if op == "hold-exit" {
		unlock, err := acquireStateFileLock(StateLockPath(home))
		if err != nil {
			t.Fatal(err)
		}
		_ = unlock // Intentionally rely on process exit to close the lock handle.
		writeStateLockSignal(t, ready)
		if err := waitForStateLockSignalWithTimeout(start, stateLockHelperTimeout); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}

	writeStateLockSignal(t, ready)
	if start != "" {
		if err := waitForStateLockSignalWithTimeout(start, stateLockHelperTimeout); err != nil {
			t.Fatal(err)
		}
	}
	switch op {
	case "upsert":
		if err := Upsert(home, InstalledPlugin{Name: name, Root: "plugins/" + name}); err != nil {
			t.Fatal(err)
		}
	case "remove":
		removed, ok, err := Remove(home, name)
		if err != nil {
			t.Fatal(err)
		}
		if !ok || removed.Name != name {
			t.Fatalf("Remove(%s) = %+v, ok=%v", name, removed, ok)
		}
	default:
		t.Fatalf("unknown helper operation %q", op)
	}
}

type stateLockHelperOperation struct {
	op   string
	name string
}

type stateLockHelperProcess struct {
	operation stateLockHelperOperation
	ready     string
	command   *exec.Cmd
	output    *bytes.Buffer
}

type stateLockHelperResult struct {
	process *stateLockHelperProcess
	err     error
}

func runStateLockHelpers(home, signalDir string, operations []stateLockHelperOperation) error {
	ctx, cancel := context.WithTimeout(context.Background(), stateLockProcessTimeout)
	defer cancel()
	start := filepath.Join(signalDir, "start")
	results := make(chan stateLockHelperResult, len(operations))
	processes := make([]*stateLockHelperProcess, 0, len(operations))
	for i, operation := range operations {
		ready := filepath.Join(signalDir, fmt.Sprintf("ready-%02d", i))
		command, output := newStateLockHelperCommand(ctx, home, operation.op, operation.name, ready, start)
		process := &stateLockHelperProcess{operation: operation, ready: ready, command: command, output: output}
		if err := command.Start(); err != nil {
			return fmt.Errorf("start %s %s helper: %w", operation.op, operation.name, err)
		}
		processes = append(processes, process)
		go func() { results <- stateLockHelperResult{process: process, err: command.Wait()} }()
	}

	ready := make(map[string]bool, len(processes))
	for len(ready) < len(processes) {
		for _, process := range processes {
			if ready[process.ready] {
				continue
			}
			if _, err := os.Stat(process.ready); err == nil {
				ready[process.ready] = true
			}
		}
		if len(ready) == len(processes) {
			break
		}
		select {
		case result := <-results:
			return fmt.Errorf("%s %s helper exited before release: %v\n%s", result.process.operation.op, result.process.operation.name, result.err, result.process.output.String())
		case <-ctx.Done():
			return fmt.Errorf("wait for %d helper processes: %w", len(processes)-len(ready), ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := os.WriteFile(start, []byte("start\n"), 0o600); err != nil {
		return err
	}
	for range processes {
		select {
		case result := <-results:
			if result.err != nil {
				return fmt.Errorf("%s %s helper: %w\n%s", result.process.operation.op, result.process.operation.name, result.err, result.process.output.String())
			}
		case <-ctx.Done():
			return fmt.Errorf("wait for helper completion: %w", ctx.Err())
		}
	}
	return nil
}

func newStateLockHelperCommand(ctx context.Context, home, op, name, ready, start string) (*exec.Cmd, *bytes.Buffer) {
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStateLockHelperProcess$")
	command.Env = append(os.Environ(),
		stateLockHelperEnv+"=1",
		stateLockHelperOpEnv+"="+op,
		stateLockHelperHomeEnv+"="+home,
		stateLockHelperNameEnv+"="+name,
		stateLockHelperReadyEnv+"="+ready,
		stateLockHelperStartEnv+"="+start,
	)
	output := &bytes.Buffer{}
	command.Stdout = output
	command.Stderr = output
	return command, output
}

func writeStateLockSignal(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForStateLockSignal(ctx context.Context, path string) error {
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForStateLockSignalWithTimeout(path string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return waitForStateLockSignal(ctx, path)
}
