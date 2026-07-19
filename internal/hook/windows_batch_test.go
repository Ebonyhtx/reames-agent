//go:build windows

package hook

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reames-agent/internal/processpolicy"
	"reames-agent/internal/sandbox"
	"reames-agent/internal/testenv"
)

func TestMain(m *testing.M) {
	sandbox.RegisterHelperDispatch()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case sandbox.ChildExecHelperCommand:
			os.Exit(sandbox.RunChildExecHelper(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		case sandbox.WindowsHelperCommand:
			os.Exit(sandbox.RunWindowsSandboxHelper(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	cleanupUserState, err := testenv.IsolateUserState()
	if err != nil {
		panic(err)
	}
	code := m.Run()
	cleanupUserState()
	os.Exit(code)
}

func TestDefaultSpawnerRunsQuotedBatchHook(t *testing.T) {
	pluginRoot := filepath.Join(t.TempDir(), "plugin root")
	hooksDir := filepath.Join(pluginRoot, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(hooksDir, "run-hook.cmd")
	contents := "@echo off\r\nset /p hook_input=\r\necho %1:%hook_input%\r\n"
	if err := os.WriteFile(script, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	result := DefaultSpawner(context.Background(), SpawnInput{
		Command: `"` + filepath.ToSlash(script) + `" session-start`,
		Stdin:   `{"event":"SessionStart"}`,
		Timeout: 5 * time.Second,
	})
	if result.ExitCode != 0 || result.SpawnErr != nil {
		t.Fatalf("batch hook failed: %+v", result)
	}
	if got, want := result.Stdout, `session-start:{"event":"SessionStart"}`; got != want {
		t.Fatalf("batch hook stdout = %q, want %q", got, want)
	}
}

func TestDefaultSpawnerRunsPackageBatchHookInsideSandbox(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("Windows sandbox unavailable")
	}
	home := t.TempDir()
	pluginRoot := filepath.Join(home, "packages", "plugin root")
	hooksDir := filepath.Join(pluginRoot, "hooks")
	stateRoot := filepath.Join(home, "state", "example")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(hooksDir, "package-hook.cmd")
	contents := "@echo off\r\nset /p hook_input=\r\necho package:%hook_input%\r\n"
	if err := os.WriteFile(script, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	result := DefaultSpawner(context.Background(), SpawnInput{
		Command: script,
		Cwd:     pluginRoot,
		Stdin:   `{"event":"SessionStart"}`,
		Timeout: 10 * time.Second,
		PackagePolicy: processpolicy.PackagePolicy{
			Owner:       "example",
			PackageRoot: pluginRoot,
			StateRoot:   stateRoot,
			HostHome:    home,
			Network:     true,
		},
	})
	if result.ExitCode != 0 || result.SpawnErr != nil {
		t.Fatalf("sandboxed package batch hook failed: %+v", result)
	}
	if got, want := result.Stdout, `package:{"event":"SessionStart"}`; got != want {
		t.Fatalf("sandboxed package batch stdout = %q, want %q", got, want)
	}
}
