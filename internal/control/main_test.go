package control

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	if os.Getenv("REAMES_AGENT_CREDENTIALS_STORE") == "" {
		_ = os.Setenv("REAMES_AGENT_CREDENTIALS_STORE", "file")
	}
	goleak.VerifyTestMain(m)
}
