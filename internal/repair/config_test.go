package repair

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reames-agent/internal/config"
)

func TestConfigRepairRestoresVerifiedSnapshotAndUndo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	path := config.UserConfigPath()
	good := []byte("default_model = \"deepseek-flash\"\n")
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RecordHealthyConfig("v1"); err != nil {
		t.Fatal(err)
	}
	bad := []byte("[broken\n")
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := InspectAndRepairConfig(ConfigOptions{Root: t.TempDir(), Apply: true, Now: func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Applied) != 2 || report.Transaction == nil {
		t.Fatalf("report = %+v", report)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(good) {
		t.Fatalf("restored config = %q", got)
	}
	if _, err := UndoLastRepair(); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(bad) {
		t.Fatalf("undo config = %q", got)
	}
}

func TestRestoreSnapshotToMissingConfigRecordsUndo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	path := config.UserConfigPath()
	good := []byte("default_model = \"deepseek-flash\"\n")
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RecordHealthyConfig("v1"); err != nil {
		t.Fatal(err)
	}
	snapshots, err := ListConfigSnapshots()
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("snapshots = %+v, %v", snapshots, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	tx, err := RestoreConfigSnapshot(snapshots[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Changes) != 1 || !tx.Changes[0].MissingBefore {
		t.Fatalf("restore transaction = %+v", tx)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("restored config: %v", err)
	}
	if _, err := UndoLastRepair(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("undo should restore the missing-config state: %v", err)
	}
}

func TestConfigSnapshotRejectsTamperAndRetainsFive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	path := config.UserConfigPath()
	for i := 0; i < 7; i++ {
		body := []byte("language = \"en\"\n# " + string(rune('a'+i)) + "\n")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RecordHealthyConfig("v1"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	snapshots, err := ListConfigSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != configSnapshotRetention {
		t.Fatalf("snapshots = %d", len(snapshots))
	}
	if err := os.WriteFile(snapshots[0].Path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreConfigSnapshot(snapshots[0].ID); err == nil {
		t.Fatal("tampered snapshot restored")
	}
	if rel, err := filepath.Rel(filepath.Join(home, "repair", "snapshots"), snapshots[0].Path); err != nil || rel == ".." {
		t.Fatalf("snapshot path escaped: %q (%v)", rel, err)
	}
}
