package cli

import (
	"os"
	"path/filepath"
	"testing"

	"reames-agent/internal/homebackup"
)

func TestBackupCLIExplicitHomeDoesNotMixAmbientStateAndRoundTrips(t *testing.T) {
	base := t.TempDir()
	ambientHome := filepath.Join(base, "ambient-home")
	ambientState := filepath.Join(base, "ambient-state")
	selectedHome := filepath.Join(base, "selected-home")
	writeBackupCLIFixture(t, filepath.Join(ambientState, "sessions", "ambient.jsonl"), "must-not-be-included")
	writeBackupCLIFixture(t, filepath.Join(selectedHome, "config.toml"), "model = \"fixture\"\n")
	t.Setenv("REAMES_AGENT_HOME", ambientHome)
	t.Setenv("REAMES_AGENT_STATE_HOME", ambientState)

	archive := filepath.Join(base, "backup.zip")
	out := captureStdout(t, func() {
		if rc := Run([]string{"backup", "create", "--offline", "--out", archive, "--home", selectedHome}, "v1.2.3-test"); rc != 0 {
			t.Fatalf("backup create exit = %d", rc)
		}
	})
	if out == "" {
		t.Fatal("backup create produced no evidence output")
	}
	manifest, err := homebackup.ReadManifest(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Roots) != 1 || manifest.Roots[0].ID != "home" {
		t.Fatalf("explicit --home mixed ambient state: roots=%+v", manifest.Roots)
	}

	if rc := Run([]string{"backup", "verify", archive}, "v1.2.3-test"); rc != 0 {
		t.Fatalf("backup verify exit = %d", rc)
	}
	if _, err := os.Stat(filepath.Join(ambientHome, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("backup verify triggered ambient config migration/write: %v", err)
	}

	restored := filepath.Join(base, "restored-home")
	if rc := Run([]string{"backup", "restore", "--dry-run", "--home", restored, archive}, "v1.2.3-test"); rc != 0 {
		t.Fatalf("backup restore dry-run exit = %d", rc)
	}
	if _, err := os.Stat(restored); !os.IsNotExist(err) {
		t.Fatalf("restore dry-run wrote target: %v", err)
	}
	if rc := Run([]string{"backup", "restore", "--offline", "--home", restored, archive}, "v1.2.3-test"); rc != 0 {
		t.Fatalf("backup restore exit = %d", rc)
	}
	data, err := os.ReadFile(filepath.Join(restored, "config.toml"))
	if err != nil || string(data) != "model = \"fixture\"\n" {
		t.Fatalf("restored config = %q err=%v", data, err)
	}
}

func TestBackupCLIRequiresOfflineAndExplicitRestoreTargets(t *testing.T) {
	if rc := backupCommand([]string{"create", "--out", filepath.Join(t.TempDir(), "backup.zip")}, "test"); rc != 2 {
		t.Fatalf("backup create without --offline exit = %d, want 2", rc)
	}

	base := t.TempDir()
	home := filepath.Join(base, "home")
	state := filepath.Join(base, "state")
	writeBackupCLIFixture(t, filepath.Join(home, "config.toml"), "home")
	writeBackupCLIFixture(t, filepath.Join(state, "sessions", "one.jsonl"), "state")
	archive := filepath.Join(base, "split.zip")
	if _, err := homebackup.Create(homebackup.CreateOptions{
		Roots:       []homebackup.Root{{ID: "home", Path: home}, {ID: "state", Path: state}},
		Destination: archive,
	}); err != nil {
		t.Fatal(err)
	}
	if rc := backupCommand([]string{"restore", "--dry-run", archive}, "test"); rc != 2 {
		t.Fatalf("backup restore without --home exit = %d, want 2", rc)
	}
	if rc := backupCommand([]string{"restore", "--dry-run", "--home", filepath.Join(base, "new-home"), archive}, "test"); rc != 2 {
		t.Fatalf("split backup restore without --state-home exit = %d, want 2", rc)
	}
}

func TestBackupCLIRejectsExplicitEmptyCreateRoots(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	writeBackupCLIFixture(t, filepath.Join(home, "config.toml"), "home")
	t.Setenv("REAMES_AGENT_STATE_HOME", filepath.Join(base, "ambient-state"))

	for _, args := range [][]string{
		{"backup", "create", "--offline", "--out", filepath.Join(base, "empty-home.zip"), "--home="},
		{"backup", "create", "--offline", "--out", filepath.Join(base, "empty-state.zip"), "--home", home, "--state-home="},
	} {
		if rc := Run(args, "v1.2.3-test"); rc != 2 {
			t.Fatalf("Run(%v) exit = %d, want 2", args, rc)
		}
		if _, err := os.Stat(args[4]); !os.IsNotExist(err) {
			t.Fatalf("invalid create wrote archive %s: %v", args[4], err)
		}
	}
}

func writeBackupCLIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
