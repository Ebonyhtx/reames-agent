package repair

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPendingUpdateVerifiesWholeUnitBeforeRollback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent-desktop")
	guard := filepath.Join(dir, "reames-agent-guard")
	for path, body := range map[string]string{target: "old-desktop", guard: "old-guard"} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	originalExecutable := repairExecutable
	repairExecutable = func() (string, error) { return guard, nil }
	t.Cleanup(func() { repairExecutable = originalExecutable })
	if _, err := PrepareFileUpdate("v1", "v2", target, guard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("new-desktop"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guard, []byte("new-guard"), 0o700); err != nil {
		t.Fatal(err)
	}
	tx, err := ReadPendingUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tx.Files[1].BackupPath, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := RollbackPendingUpdate(); err == nil {
		t.Fatal("tampered release-unit backup rolled back")
	}
	for path, want := range map[string]string{target: "new-desktop", guard: "new-guard"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", path, got, err)
		}
	}
}

func TestPendingUpdateRollbackRestoresUnit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent-desktop.exe")
	guard := filepath.Join(dir, "reames-agent-guard.exe")
	for path, body := range map[string]string{target: "old-desktop", guard: "old-guard"} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	originalExecutable := repairExecutable
	repairExecutable = func() (string, error) { return guard, nil }
	t.Cleanup(func() { repairExecutable = originalExecutable })
	if _, err := PrepareFileUpdate("v1", "v2", target, guard); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(target, []byte("new-desktop"), 0o700)
	_ = os.WriteFile(guard, []byte("new-guard"), 0o700)
	result, err := RollbackPendingUpdate()
	if err != nil || !result.RolledBack || result.MixedInstall {
		t.Fatalf("rollback = %+v, %v", result, err)
	}
	for path, want := range map[string]string{target: "old-desktop", guard: "old-guard"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", path, got, err)
		}
	}
}

func TestPrepareUpdatePreservesExistingPendingEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent-desktop.exe")
	guard := filepath.Join(dir, "reames-agent-guard.exe")
	for path, body := range map[string]string{target: "old-desktop", guard: "old-guard"} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	originalExecutable := repairExecutable
	repairExecutable = func() (string, error) { return guard, nil }
	t.Cleanup(func() { repairExecutable = originalExecutable })
	first, err := PrepareFileUpdate("v1", "v2", target, guard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareFileUpdate("v2", "v3", target, guard); err == nil {
		t.Fatal("second update overwrote pending rollback evidence")
	}
	stillPending, err := ReadPendingUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if stillPending.ToVersion != first.ToVersion || stillPending.CreatedAt != first.CreatedAt || stillPending.BackupSHA256 != first.BackupSHA256 {
		t.Fatalf("pending evidence changed: first=%+v current=%+v", first, stillPending)
	}
}

func TestAutomaticRollbackRechecksTransactionIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent-desktop.exe")
	guard := filepath.Join(dir, "reames-agent-guard.exe")
	for path, body := range map[string]string{target: "old-desktop", guard: "old-guard"} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	originalExecutable := repairExecutable
	repairExecutable = func() (string, error) { return guard, nil }
	t.Cleanup(func() { repairExecutable = originalExecutable })
	tx, err := PrepareFileUpdate("v1", "v2", target, guard)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("new-desktop"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guard, []byte("new-guard"), 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := RollbackPendingUpdateIfCurrent(tx.ToVersion, tx.CreatedAt+"-changed")
	if err != nil || result.RolledBack {
		t.Fatalf("mismatched rollback = %+v, %v", result, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new-desktop" {
		t.Fatalf("mismatched rollback changed target = %q, %v", got, err)
	}
	if _, err := os.Stat(PendingUpdatePath()); err != nil {
		t.Fatalf("mismatched rollback removed pending transaction: %v", err)
	}

	result, err = RollbackPendingUpdateIfCurrent(tx.ToVersion, tx.CreatedAt)
	if err != nil || !result.RolledBack {
		t.Fatalf("matched rollback = %+v, %v", result, err)
	}
}

func TestPendingUpdateCommitAndRollbackAreSerializedAcrossProcesses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	dir := t.TempDir()
	target := filepath.Join(dir, "reames-agent-desktop.exe")
	guard := filepath.Join(dir, "reames-agent-guard.exe")
	for path, body := range map[string]string{target: "old-desktop", guard: "old-guard"} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	originalExecutable := repairExecutable
	repairExecutable = func() (string, error) { return guard, nil }
	t.Cleanup(func() { repairExecutable = originalExecutable })
	if _, err := PrepareFileUpdate("v1", "v2", target, guard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("new-desktop"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guard, []byte("new-guard"), 0o700); err != nil {
		t.Fatal(err)
	}

	commands := []*exec.Cmd{
		exec.Command(os.Args[0], "-test.run=^TestPendingUpdateCrossProcessHelper$"),
		exec.Command(os.Args[0], "-test.run=^TestPendingUpdateCrossProcessHelper$"),
	}
	commands[0].Env = append(os.Environ(), "REAMES_AGENT_PENDING_HELPER=rollback", "REAMES_AGENT_PENDING_TARGET="+guard)
	commands[1].Env = append(os.Environ(), "REAMES_AGENT_PENDING_HELPER=commit", "REAMES_AGENT_PENDING_TARGET="+guard)
	for _, cmd := range commands {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper %v: %v", cmd.Args, err)
		}
	}
	if _, err := os.Stat(PendingUpdatePath()); !os.IsNotExist(err) {
		t.Fatalf("pending update remains after serialized commit/rollback: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-desktop" && string(got) != "new-desktop" {
		t.Fatalf("desktop contains mixed/corrupt content %q", got)
	}
	for _, suffix := range []string{".reames-rollback-stage", ".reames-rollback-aside"} {
		if _, err := os.Stat(target + suffix); !os.IsNotExist(err) {
			t.Fatalf("rollback artifact %s remains: %v", suffix, err)
		}
	}
}

func TestPendingUpdateCrossProcessHelper(t *testing.T) {
	op := os.Getenv("REAMES_AGENT_PENDING_HELPER")
	if op == "" {
		return
	}
	target := os.Getenv("REAMES_AGENT_PENDING_TARGET")
	repairExecutable = func() (string, error) { return target, nil }
	switch op {
	case "rollback":
		if _, err := RollbackPendingUpdate(); err != nil {
			t.Fatal(err)
		}
	case "commit":
		if err := MarkUpdateHealthy("v2"); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown helper operation %q", op)
	}
}
