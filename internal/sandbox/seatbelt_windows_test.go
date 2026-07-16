//go:build windows

package sandbox

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/winsandbox"
)

func TestWindowsCommandWrapsWithHelper(t *testing.T) {
	// Command only wraps when Available(), which requires the entry point to
	// have registered its helper dispatch route (cli.Run / desktop main do).
	RegisterHelperDispatch()
	if !winsandbox.Available() {
		t.Skip("windows sandbox APIs unavailable")
	}
	cmd, wrapped := Command(Spec{Mode: "enforce", WriteRoots: []string{`C:\work`}, Network: true}, Shell{Kind: ShellPowerShell, Path: "powershell"}, "Write-Output ok")
	if !wrapped {
		t.Fatal("windows enforce should wrap through helper")
	}
	if len(cmd) < 6 {
		t.Fatalf("wrapped argv too short: %v", cmd)
	}
	if got := cmd[1]; got != WindowsHelperCommand {
		t.Fatalf("helper command = %q, want %q (argv=%v)", got, WindowsHelperCommand, cmd)
	}
	if cmd[3] != "--" {
		t.Fatalf("helper argv separator = %q, want -- (argv=%v)", cmd[3], cmd)
	}
	payload, err := decodeWindowsSandboxPayload(cmd[2])
	if err != nil {
		t.Fatalf("decode helper payload: %v", err)
	}
	if payload.Spec.Mode != "enforce" || !payload.Spec.Network || len(payload.Spec.WriteRoots) != 1 || !payload.Writable {
		t.Fatalf("payload not preserved: %+v writable=%v", payload.Spec, payload.Writable)
	}
	if !strings.Contains(strings.Join(cmd[4:], " "), "Write-Output ok") {
		t.Fatalf("child argv not appended: %v", cmd)
	}
}

func TestWindowsCommandArgsWrapsReadOnly(t *testing.T) {
	RegisterHelperDispatch()
	if !winsandbox.Available() {
		t.Skip("windows sandbox APIs unavailable")
	}
	cmd, wrapped := CommandArgs(Spec{Mode: "enforce", WriteRoots: []string{`C:\work`}, Network: false}, []string{`C:\tools\rg.exe`, "needle"})
	if !wrapped {
		t.Fatal("windows enforce should wrap direct argv through helper")
	}
	payload, err := decodeWindowsSandboxPayload(cmd[2])
	if err != nil {
		t.Fatalf("decode helper payload: %v", err)
	}
	if payload.Writable {
		t.Fatalf("direct argv should be marked read-only: %+v", payload)
	}
	if payload.Spec.Network {
		t.Fatalf("network=false should be preserved for AppContainer launch: %+v", payload.Spec)
	}
}

func TestWindowsCommandArgsWithOptionsKeepsChildEnvOutOfArgv(t *testing.T) {
	RegisterHelperDispatch()
	if !winsandbox.Available() {
		t.Skip("windows sandbox APIs unavailable")
	}
	cmd, wrapped := CommandArgsWithOptions(Spec{Mode: "enforce", WriteRoots: []string{`C:\work`}, Network: true, Strict: true}, []string{`C:\tools\plugin.exe`, "serve"}, CommandOptions{
		Writable: true, Env: []string{`PATH=C:\Windows\System32`, "PLUGIN_TOKEN=explicit"}, Dir: `C:\work`,
	})
	if !wrapped {
		t.Fatal("windows package command should be wrapped")
	}
	payload, err := decodeWindowsSandboxPayload(cmd[2])
	if err != nil {
		t.Fatal(err)
	}
	if !payload.Writable || !payload.ChildEnvironment || payload.Dir != `C:\work` {
		t.Fatalf("child options not preserved: %+v", payload)
	}
	joined := strings.Join(cmd, "\n")
	for _, leaked := range []string{"PLUGIN_TOKEN", "explicit"} {
		if strings.Contains(joined, leaked) {
			t.Fatalf("windows wrapper argv exposed child environment %q: %v", leaked, cmd)
		}
	}
}

func TestConvertWindowsSandboxSpec(t *testing.T) {
	spec := Spec{
		Mode:            "enforce",
		WriteRoots:      []string{`C:\work`},
		ForbidReadRoots: []string{`C:\work\secret`},
		ForbidReadPaths: []string{`C:\work\.env`},
		Network:         true,
	}
	got := convertWindowsSandboxSpec(spec, true)
	if !got.Writable || !got.Network || got.TempPrefix != "reamesAgent-sandbox-" {
		t.Fatalf("converted flags = %+v", got)
	}
	if len(got.WritableRoots) != 1 || got.WritableRoots[0] != spec.WriteRoots[0] {
		t.Fatalf("converted writable roots = %v", got.WritableRoots)
	}
	if len(got.ForbidReadRoots) != 2 || got.ForbidReadRoots[0] != spec.ForbidReadRoots[0] || got.ForbidReadRoots[1] != spec.ForbidReadPaths[0] {
		t.Fatalf("converted forbid roots = %v", got.ForbidReadRoots)
	}

	got.WritableRoots[0] = `C:\mutated`
	got.ForbidReadRoots[0] = `C:\mutated`
	if spec.WriteRoots[0] == got.WritableRoots[0] || spec.ForbidReadRoots[0] == got.ForbidReadRoots[0] {
		t.Fatal("converted spec should not alias Reames Agent slices")
	}
}

func TestWindowsSandboxAvailableOnCI(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("only require Windows sandbox availability on CI")
	}
	if RegisterHelperDispatch(); !Available() {
		t.Fatal("windows sandbox APIs unavailable on CI")
	}
	if !winsandbox.Available() {
		t.Fatal("bundled windows sandbox APIs unavailable on CI")
	}
}

func TestWindowsUnavailableWithoutHelperDispatch(t *testing.T) {
	// The dispatch flag is process-global and other tests set it, so this can
	// only assert the wrap-side contract indirectly: with the flag forced off,
	// Command must refuse to wrap (unwrapped argv triggers the bash tool's
	// fail-closed / escape-approval path) rather than emit a helper argv that
	// a dispatch-less binary would swallow into empty output.
	prev := helperDispatchRegistered.Load()
	helperDispatchRegistered.Store(false)
	defer helperDispatchRegistered.Store(prev)
	if Available() {
		t.Fatal("Available() must be false while the helper dispatch is unregistered")
	}
	argv, wrapped := Command(Spec{Mode: "enforce", WriteRoots: []string{`C:\work`}, Network: true}, Shell{Kind: ShellPowerShell, Path: "powershell"}, "Write-Output ok")
	if wrapped {
		t.Fatalf("enforce without helper dispatch must not wrap, got argv %v", argv)
	}
}

func TestRunWindowsSandboxHelperRunsExternalSandbox(t *testing.T) {
	RegisterHelperDispatch()
	if !Available() {
		t.Skip("windows sandbox APIs unavailable")
	}
	sh := powershellArgvForWindowsSandboxTest(t, "")
	if sh == nil {
		t.Skip("PowerShell unavailable")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	insideFile := filepath.Join(workspace, "inside.txt")
	outsideFile := filepath.Join(outside, "outside.txt")
	payload, err := encodeWindowsSandboxPayload(windowsSandboxPayload{
		Spec:     Spec{Mode: "enforce", WriteRoots: []string{workspace}, Network: true},
		Writable: true,
	})
	if err != nil {
		t.Fatalf("encode helper payload: %v", err)
	}
	script := "$ErrorActionPreference='Stop'; " +
		"Set-Content -LiteralPath " + psQuoteWindowsSandboxTest(insideFile) + " -Value ok; " +
		"try { Set-Content -LiteralPath " + psQuoteWindowsSandboxTest(outsideFile) + " -Value nope; exit 9 } catch { exit 0 }"
	helperArgs := append([]string{payload, "--"}, append(sh, script)...)
	if code := RunWindowsSandboxHelper(helperArgs, os.Stdin, os.Stdout, os.Stderr); code != 0 {
		t.Fatalf("helper exit code = %d, want 0", code)
	}
	if got, err := os.ReadFile(insideFile); err != nil || !strings.Contains(string(got), "ok") {
		t.Fatalf("inside write missing: %q err=%v", got, err)
	}
	if _, err := os.Stat(outsideFile); err == nil {
		t.Fatalf("outside write unexpectedly succeeded: %s", outsideFile)
	}
}

func TestRunWindowsSandboxHelperAppliesChildEnvironmentAndDir(t *testing.T) {
	RegisterHelperDispatch()
	if !Available() {
		t.Skip("windows sandbox APIs unavailable")
	}
	sh := powershellArgvForWindowsSandboxTest(t, "")
	if sh == nil {
		t.Skip("PowerShell unavailable")
	}
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "env-dir.txt")
	env := []string{"PLUGIN_TOKEN=explicit"}
	for _, key := range []string{"PATH", "PATHEXT", "SYSTEMROOT", "COMSPEC", "TEMP", "TMP", "USERPROFILE"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	payload, err := encodeWindowsSandboxPayload(windowsSandboxPayload{
		Spec:             Spec{Mode: "enforce", WriteRoots: []string{workspace}, Network: true, Strict: true},
		Writable:         true,
		ChildEnvironment: true,
		Dir:              workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	encodedEnv, err := encodeChildEnvironment(env)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(childEnvironmentVariable, encodedEnv)
	script := "$ErrorActionPreference='Stop'; " +
		"if ($env:PLUGIN_TOKEN -ne 'explicit') { exit 7 }; " +
		"Set-Content -LiteralPath 'env-dir.txt' -Value ok"
	helperArgs := append([]string{payload, "--"}, append(sh, script)...)
	if code := RunWindowsSandboxHelper(helperArgs, os.Stdin, os.Stdout, os.Stderr); code != 0 {
		t.Fatalf("helper exit code = %d, want 0", code)
	}
	if body, err := os.ReadFile(marker); err != nil || !strings.Contains(string(body), "ok") {
		t.Fatalf("child environment/cwd marker = %q err=%v", body, err)
	}
}

func TestRunWindowsSandboxHelperFailsClosedWithoutChildEnvironment(t *testing.T) {
	previous, existed := os.LookupEnv(childEnvironmentVariable)
	_ = os.Unsetenv(childEnvironmentVariable)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(childEnvironmentVariable, previous)
		} else {
			_ = os.Unsetenv(childEnvironmentVariable)
		}
	})
	payload, err := encodeWindowsSandboxPayload(windowsSandboxPayload{
		Spec:             Spec{Mode: "enforce", Network: true},
		Writable:         true,
		ChildEnvironment: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "sandbox-stderr-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	if code := RunWindowsSandboxHelper([]string{payload, "--", "cmd"}, os.Stdin, os.Stdout, stderr); code != 126 {
		t.Fatalf("missing child environment exit code = %d, want 126", code)
	}
	if _, err := stderr.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(stderr)
	if err != nil {
		t.Fatal(err)
	}
	if text := string(body); !strings.Contains(text, WindowsSandboxFailureMarker(payload)) || !strings.Contains(text, "environment is missing") {
		t.Fatalf("missing child environment diagnostic = %q", text)
	}
}

func powershellArgvForWindowsSandboxTest(t *testing.T, command string) []string {
	t.Helper()
	for _, name := range []string{"pwsh", "powershell"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		args := []string{path, "-NoProfile", "-NonInteractive", "-Command"}
		if command != "" {
			args = append(args, command)
		}
		return args
	}
	return nil
}

func psQuoteWindowsSandboxTest(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
