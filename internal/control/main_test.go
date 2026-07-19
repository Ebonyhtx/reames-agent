package control

import (
	"os"
	"testing"

	"reames-agent/internal/testenv"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	cleanupUserState, err := testenv.IsolateUserState()
	if err != nil {
		panic(err)
	}
	if os.Getenv("REAMES_AGENT_CREDENTIALS_STORE") == "" {
		_ = os.Setenv("REAMES_AGENT_CREDENTIALS_STORE", "file")
	}
	goleak.VerifyTestMain(m, goleak.Cleanup(func(exitCode int) {
		cleanupUserState()
		os.Exit(exitCode)
	}))
}
