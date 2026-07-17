package guardcmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGuardBinaryLaunchesSiblingInSafeModeAndPropagatesExitCode(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	guardPath := filepath.Join(installDir, "reames-agent-guard"+ext)
	desktopPath := filepath.Join(installDir, "reames-agent-desktop"+ext)
	buildGoBinary(t, root, guardPath, filepath.Join(root, "cmd", "reames-agent-guard"))

	fakeSource := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(fakeSource, []byte(`package main
import "os"
func main() {
	if len(os.Args) < 2 { os.Exit(22) }
	if err := os.WriteFile(os.Args[1], []byte(os.Getenv("REAMES_AGENT_SAFE_MODE")), 0600); err != nil { os.Exit(21) }
	os.Exit(23)
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	buildGoBinary(t, root, desktopPath, fakeSource)

	resultPath := filepath.Join(t.TempDir(), "safe-mode.txt")
	cmd := exec.Command(guardPath, "launch", "--detach=false", "--safe-mode", "--app", desktopPath, resultPath)
	cmd.Env = append(os.Environ(), "REAMES_AGENT_STATE_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("guard launch error = %v, output=%q", err, output)
	}
	got, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1" {
		t.Fatalf("safe mode env = %q", got)
	}

	outside := filepath.Join(t.TempDir(), "reames-agent-desktop"+ext)
	if err := copyTestFile(desktopPath, outside); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(guardPath, "launch", "--detach=false", "--app", outside, resultPath)
	cmd.Env = append(os.Environ(), "REAMES_AGENT_STATE_HOME="+t.TempDir())
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "outside the current Guard installation") {
		t.Fatalf("outside launch error = %v, output=%q", err, output)
	}
}

func buildGoBinary(t *testing.T, root, output string, input ...string) {
	t.Helper()
	args := append([]string{"build", "-trimpath", "-o", output}, input...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func copyTestFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o700)
}
