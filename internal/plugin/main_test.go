package plugin

import (
	"os"
	"testing"

	"reames-agent/internal/testenv"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// MCP helper subprocesses inherit the already-isolated parent environment;
	// do not allocate a second user home outside their sandbox roots.
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" || os.Getenv("GO_WANT_HELPER_STDERR_EXIT") == "1" {
		os.Exit(m.Run())
	}
	cleanupUserState, err := testenv.IsolateUserState()
	if err != nil {
		panic(err)
	}
	goleak.VerifyTestMain(m, goleak.Cleanup(func(exitCode int) {
		cleanupUserState()
		os.Exit(exitCode)
	}))
}
