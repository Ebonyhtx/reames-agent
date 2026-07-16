//go:build !windows

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestChildExecHelperRestoresExplicitEnvironment(t *testing.T) {
	child := []string{
		"PATH=" + os.Getenv("PATH"),
		"REAMES_AGENT_TEST_CHILD_EXEC_PROBE=expected",
		"PLUGIN_TOKEN=explicit-plugin-secret",
	}
	host, err := CommandHostEnvironment([]string{"PATH=" + os.Getenv("PATH")}, child)
	if err != nil {
		t.Fatal(err)
	}
	host = append(host, "REAMES_AGENT_TEST_CHILD_EXEC_DISPATCH=1")
	cmd := exec.Command(os.Args[0], "-test.run=^TestChildExecHelperDispatchProcess$")
	cmd.Env = host
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child exec helper failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"probe=expected", "token=explicit-plugin-secret", "reserved=false", "dispatch=false"} {
		if !strings.Contains(text, want) {
			t.Fatalf("helper output missing %q: %s", want, text)
		}
	}
}

func TestChildExecHelperDispatchProcess(t *testing.T) {
	if os.Getenv("REAMES_AGENT_TEST_CHILD_EXEC_DISPATCH") != "1" {
		t.Skip("child helper dispatch process only")
	}
	code := RunChildExecHelper([]string{"--", os.Args[0], "-test.run=^TestChildExecHelperProbeProcess$"}, os.Stdin, os.Stdout, os.Stderr)
	os.Exit(code)
}

func TestChildExecHelperProbeProcess(t *testing.T) {
	if os.Getenv("REAMES_AGENT_TEST_CHILD_EXEC_PROBE") != "expected" {
		t.Skip("child helper probe process only")
	}
	_, reserved := os.LookupEnv(childEnvironmentVariable)
	_, dispatch := os.LookupEnv("REAMES_AGENT_TEST_CHILD_EXEC_DISPATCH")
	fmt.Printf("probe=%s token=%s reserved=%t dispatch=%t\n",
		os.Getenv("REAMES_AGENT_TEST_CHILD_EXEC_PROBE"), os.Getenv("PLUGIN_TOKEN"), reserved, dispatch)
}
