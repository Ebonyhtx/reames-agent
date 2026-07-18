//go:build linux

package gatewayservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSystemdAnalyzeAcceptsWatchdogUnit(t *testing.T) {
	if _, err := exec.LookPath("systemd-analyze"); err != nil {
		t.Skip("systemd-analyze is unavailable")
	}
	plan, err := BuildPlan("linux", Options{
		Action:      "install",
		Executable:  "/bin/true",
		Home:        "/tmp/reames-agent-test-home",
		Dir:         "/tmp",
		WatchdogSec: 60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("watchdog install files = %d, want 1", len(plan.Files))
	}
	path := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(path, []byte(plan.Files[0].Content), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemd-analyze", "verify", path).CombinedOutput()
	if err != nil {
		t.Fatalf("systemd-analyze verify: %v\n%s", err, out)
	}
}
