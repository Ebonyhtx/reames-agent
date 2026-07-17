package repair

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectReportsBinaryConfigAndStoresOffline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	target := filepath.Join(t.TempDir(), "reames-agent-desktop")
	if err := os.WriteFile(target, []byte("current"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target+".previous", []byte("previous"), 0o700); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect(InspectOptions{Root: t.TempDir(), ExecutablePath: target})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || len(report.Binaries) != 2 || !report.Binaries[0].Exists || report.Binaries[0].SHA256 == "" || !report.Binaries[1].Exists {
		t.Fatalf("report = %+v", report)
	}
	if _, err := MarshalReport(report); err != nil {
		t.Fatal(err)
	}
}
