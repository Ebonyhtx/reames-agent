package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/pluginpkg"
)

func TestPluginInstallReturnsFailureExitForFailedJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)

	source := filepath.Join(t.TempDir(), "superpowers")
	writePluginTestFile(t, filepath.Join(source, pluginpkg.CodexManifest), `{
	  "name": "superpowers",
	  "version": "6.1.1",
	  "description": "Planning workflows",
	  "skills": "skills"
	}`)
	writePluginTestFile(t, filepath.Join(source, "skills", "using-superpowers", "SKILL.md"), "---\ndescription: Use Superpowers\n---\nUse Superpowers.")

	firstPlan := capturePluginPlan(t, []string{"install", source, "--dry-run"})
	firstOut := captureStdout(t, func() {
		if rc := pluginCommand([]string{"install", source, "--yes", "--plan-id", firstPlan.PlanID}); rc != 0 {
			t.Fatalf("first plugin install rc = %d, want 0", rc)
		}
	})
	var first struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(firstOut), &first); err != nil {
		t.Fatalf("first output is not JSON: %v\n%s", err, firstOut)
	}
	if !first.OK {
		t.Fatalf("first output ok = false:\n%s", firstOut)
	}

	secondPlan := capturePluginPlan(t, []string{"install", source, "--dry-run"})
	secondOut := captureStdout(t, func() {
		if rc := pluginCommand([]string{"install", source, "--yes", "--plan-id", secondPlan.PlanID}); rc != 1 {
			t.Fatalf("duplicate plugin install rc = %d, want 1", rc)
		}
	})
	var second struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(secondOut), &second); err != nil {
		t.Fatalf("second output is not JSON: %v\n%s", err, secondOut)
	}
	if second.OK || second.Status != "failed" {
		t.Fatalf("duplicate output ok/status = %v/%q, want false/failed\n%s", second.OK, second.Status, secondOut)
	}
}

func TestPluginCLIUpdateRollbackAndRemoveUseApprovedPlans(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	source := filepath.Join(t.TempDir(), "lifecycle-cli")
	manifest := filepath.Join(source, pluginpkg.CodexManifest)
	writePluginTestFile(t, manifest, `{
  "name": "lifecycle-cli",
  "version": "1.0.0",
  "description": "CLI lifecycle",
  "skills": "skills"
}`)
	writePluginTestFile(t, filepath.Join(source, "skills", "fixture", "SKILL.md"), "---\nname: fixture\ndescription: fixture\n---\nRun fixture.\n")

	installPlan := capturePluginPlan(t, []string{"install", source, "--dry-run"})
	captureStdout(t, func() {
		if rc := pluginCommand([]string{"install", source, "--yes", "--plan-id", installPlan.PlanID}); rc != 0 {
			t.Fatalf("install rc = %d", rc)
		}
	})
	review := captureStderr(t, func() {
		if rc := pluginCommand([]string{"enable", "lifecycle-cli"}); rc != 2 {
			t.Fatalf("unconfirmed enable rc = %d, want 2", rc)
		}
	})
	if !strings.Contains(review, pluginpkg.PermissionSkillsLoad) || !strings.Contains(review, "digest:") {
		t.Fatalf("enable review omitted approval evidence:\n%s", review)
	}
	captureStdout(t, func() {
		if rc := pluginCommand([]string{"enable", "lifecycle-cli", "--yes"}); rc != 0 {
			t.Fatalf("enable rc = %d", rc)
		}
	})

	writePluginTestFile(t, manifest, `{
  "name": "lifecycle-cli",
  "version": "2.0.0",
  "description": "CLI lifecycle v2",
  "skills": "skills"
}`)
	updatePlan := capturePluginPlan(t, []string{"update", "lifecycle-cli", "--dry-run"})
	updateOut := captureStdout(t, func() {
		if rc := pluginCommand([]string{"update", "lifecycle-cli", "--yes", "--plan-id", updatePlan.PlanID}); rc != 0 {
			t.Fatalf("update rc = %d", rc)
		}
	})
	if !strings.Contains(updateOut, `"status":"done"`) {
		t.Fatalf("update output = %s", updateOut)
	}
	active, ok, err := pluginpkg.FindInstalled(home, "lifecycle-cli")
	if err != nil || !ok || active.Version != "2.0.0" || !active.Enabled || active.Previous == nil || active.Previous.Version != "1.0.0" {
		t.Fatalf("state after update = %+v ok=%t err=%v", active, ok, err)
	}

	rollbackPlan := capturePluginPlan(t, []string{"rollback", "lifecycle-cli", "--dry-run"})
	captureStdout(t, func() {
		if rc := pluginCommand([]string{"rollback", "lifecycle-cli", "--yes", "--plan-id", rollbackPlan.PlanID}); rc != 0 {
			t.Fatalf("rollback rc = %d", rc)
		}
	})
	active, ok, err = pluginpkg.FindInstalled(home, "lifecycle-cli")
	if err != nil || !ok || active.Version != "1.0.0" || !active.Enabled {
		t.Fatalf("state after rollback = %+v ok=%t err=%v", active, ok, err)
	}

	removePlan := capturePluginPlan(t, []string{"remove", "lifecycle-cli", "--dry-run"})
	captureStdout(t, func() {
		if rc := pluginCommand([]string{"remove", "lifecycle-cli", "--yes", "--plan-id", removePlan.PlanID}); rc != 0 {
			t.Fatalf("remove rc = %d", rc)
		}
	})
	if _, ok, err := pluginpkg.FindInstalled(home, "lifecycle-cli"); err != nil || ok {
		t.Fatalf("plugin remains after remove: ok=%t err=%v", ok, err)
	}
}

type pluginCLIPlan struct {
	OK     bool   `json:"ok"`
	Status string `json:"status"`
	PlanID string `json:"planId"`
}

func capturePluginPlan(t *testing.T, args []string) pluginCLIPlan {
	t.Helper()
	out := captureStdout(t, func() {
		if rc := pluginCommand(args); rc != 0 {
			t.Fatalf("plugin %v rc = %d", args, rc)
		}
	})
	var plan pluginCLIPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("plugin plan is not JSON: %v\n%s", err, out)
	}
	if !plan.OK || plan.Status != "planned" || plan.PlanID == "" {
		t.Fatalf("plugin plan = %+v\n%s", plan, out)
	}
	return plan
}
