package control

import (
	"testing"
)

func TestRecoveryStatusProjectsOfflineReport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	c := New(Options{WorkspaceRoot: t.TempDir()})
	report, err := c.RecoveryStatus()
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.GeneratedAt == "" {
		t.Fatalf("report = %+v", report)
	}
}
