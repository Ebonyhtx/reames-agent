package bot

import (
	"testing"

	"reames-agent/internal/testenv"
)

func TestMain(m *testing.M) {
	testenv.RunWithIsolatedUserState(m)
}
