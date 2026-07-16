package main

import (
	"testing"

	"reames-agent/internal/sandbox"
)

// Sandbox wrappers relaunch os.Executable() with a hidden helper as argv[1]. If
// the desktop binary loses either route, package or Windows bash commands can
// start a second GUI instance. These tests pin both routes.

func TestSandboxHelperRoutesRecognized(t *testing.T) {
	for _, helper := range []string{sandbox.ChildExecHelperCommand, sandbox.WindowsHelperCommand} {
		// argv[2:] is empty, so each helper rejects it. The point is only that
		// the route matched and would exit instead of booting the GUI.
		code, ok := runSandboxHelperIfRequested([]string{"reamesAgent-desktop", helper})
		if !ok {
			t.Fatalf("helper %q not routed; sandboxed commands would boot a second GUI instance", helper)
		}
		if code == 0 {
			t.Fatalf("helper %q with no payload should fail, got exit code %d", helper, code)
		}
	}
}

func TestWindowsSandboxHelperRouteIgnoresNormalLaunch(t *testing.T) {
	for _, argv := range [][]string{
		{"reamesAgent-desktop"},
		{"reamesAgent-desktop", "--some-flag"},
		{"reamesAgent-desktop", "not-the-helper", sandbox.WindowsHelperCommand},
	} {
		if _, ok := runSandboxHelperIfRequested(argv); ok {
			t.Fatalf("argv %v should not be treated as a helper launch", argv)
		}
	}
}
