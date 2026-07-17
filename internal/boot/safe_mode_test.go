package boot

import (
	"context"
	"errors"
	"os"
	"testing"

	"reames-agent/internal/config"
)

func TestBuildRejectsSafeModeAgentAssembly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	t.Setenv("DEEPSEEK_API_KEY", "must-not-enable-agent")
	if err := os.WriteFile(config.UserConfigPath(), []byte("[broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctrl, err := Build(context.Background(), Options{WorkspaceRoot: t.TempDir()})
	if ctrl != nil || !errors.Is(err, ErrSafeModeRecoveryOnly) {
		t.Fatalf("Safe Mode Build = %+v, %v", ctrl, err)
	}
}
