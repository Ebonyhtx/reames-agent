package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/config"
	"reames-agent/internal/repair"
)

func TestDesktopRecoveryControlWorksWithoutControllerAndRedactsPaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	path := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	report, err := a.GetRecoveryStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !report.SafeModeRequested || len(report.Config.Checks) != 1 || report.Config.Checks[0].Scope != "global" {
		t.Fatalf("recovery report = %+v", report)
	}
	body, _ := json.Marshal(report)
	if strings.Contains(string(body), home) {
		t.Fatalf("desktop report leaked home: %s", body)
	}

	result, err := a.RunRecoveryAction(repair.ActionRequest{Action: repair.ActionRepairConfig, Target: "global"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Report.LastRepair == nil {
		t.Fatalf("recovery action result = %+v", result)
	}
	body, _ = json.Marshal(result)
	if strings.Contains(string(body), home) {
		t.Fatalf("desktop action leaked home: %s", body)
	}
}
