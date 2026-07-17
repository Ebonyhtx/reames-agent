package repair

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactReportForDisplayRemovesHostPathsAndSecrets(t *testing.T) {
	home := filepath.Join(t.TempDir(), "Alice", "private-home")
	workspace := filepath.Join(home, "projects", "secret-project")
	install := filepath.Join(home, "Applications", "Reames Agent")
	t.Setenv("REAMES_AGENT_HOME", home)
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	report := Report{
		Config:          ConfigReport{Checks: []ConfigCheck{{Path: filepath.Join(home, "config.toml"), Error: "read " + workspace + ": token " + secret}}},
		ConfigSnapshots: []ConfigSnapshot{{Path: filepath.Join(home, "repair", "snapshot.toml"), SourcePath: filepath.Join(home, "config.toml")}},
		LastRepair:      &RepairTransaction{Changes: []RepairChange{{TargetPath: filepath.Join(home, "config.toml"), PreviousPath: filepath.Join(home, "config.old")}}},
		PendingUpdate:   &UpdateTransaction{TargetPath: filepath.Join(install, "desktop.exe"), BackupPath: filepath.Join(home, "repair", "desktop.previous")},
		Findings:        []Finding{{Message: "workspace " + workspace, Action: "open " + filepath.Join(install, "desktop.exe")}},
	}

	redacted := RedactReportForDisplay(report, DisplayOptions{Root: workspace, ExecutablePath: filepath.Join(install, "desktop.exe")})
	body, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{home, workspace, install, secret} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("display report leaked %q: %s", forbidden, text)
		}
	}
	for _, want := range []string{"$REAMES_AGENT_STATE", "$WORKSPACE", "$INSTALL", "[REDACTED:OpenAI]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("display report missing %q: %s", want, text)
		}
	}
}
