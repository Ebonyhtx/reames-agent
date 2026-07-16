package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain mirrors the two production entry points so integration tests can
// exercise the real re-exec protocol instead of calling helpers only in-process.
func TestMain(m *testing.M) {
	RegisterHelperDispatch()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case ChildExecHelperCommand:
			os.Exit(RunChildExecHelper(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		case WindowsHelperCommand:
			os.Exit(RunWindowsSandboxHelper(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	os.Exit(m.Run())
}

func TestCommandArgsWithOptionsEndToEndChildEnvironment(t *testing.T) {
	if !Available() {
		t.Skip("native OS sandbox unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	childEnv := []string{
		"REAMES_AGENT_TEST_SANDBOX_CHILD=expected",
		"PLUGIN_TOKEN=synthetic-explicit-plugin-secret",
	}
	hostBase := make([]string, 0, 12)
	for _, key := range []string{
		"PATH", "PATHEXT", "SYSTEMROOT", "SYSTEMDRIVE", "COMSPEC",
		"TEMP", "TMP", "TMPDIR", "USERPROFILE", "USERNAME", "HOME",
	} {
		if value := os.Getenv(key); value != "" {
			entry := key + "=" + value
			hostBase = append(hostBase, entry)
			childEnv = append(childEnv, entry)
		}
	}
	argv, wrapped := CommandArgsWithOptions(
		Spec{Mode: "enforce", WriteRoots: []string{workspace}, Network: true, Strict: true},
		[]string{executable, "-test.run=^TestSandboxChildEnvironmentProbe$"},
		CommandOptions{Writable: true, Env: childEnv, Dir: workspace},
	)
	if !wrapped {
		t.Fatal("package command should be wrapped")
	}
	hostEnv, err := CommandHostEnvironment(hostBase, childEnv)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = hostEnv
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("end-to-end sandbox child failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"child=expected", "token=synthetic-explicit-plugin-secret", "reserved=false"} {
		if !strings.Contains(text, want) {
			t.Fatalf("sandbox child output missing %q: %s", want, text)
		}
	}
}

func TestSandboxChildEnvironmentProbe(t *testing.T) {
	if os.Getenv("REAMES_AGENT_TEST_SANDBOX_CHILD") != "expected" {
		t.Skip("sandbox child probe process only")
	}
	_, reserved := os.LookupEnv(childEnvironmentVariable)
	fmt.Printf("child=%s token=%s reserved=%t\n", os.Getenv("REAMES_AGENT_TEST_SANDBOX_CHILD"), os.Getenv("PLUGIN_TOKEN"), reserved)
}
