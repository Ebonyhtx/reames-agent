package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/control"
	"reames-agent/internal/hook"
	"reames-agent/internal/pluginpkg"
	"reames-agent/internal/skill"
	"reames-agent/internal/tool"
)

func TestPluginMutationGateBlocksNewDesktopWork(t *testing.T) {
	app := NewApp()
	ctrl := control.New(control.Options{})
	tab := &WorkspaceTab{ID: "active", Ctrl: ctrl, Ready: true}
	app.mu.Lock()
	app.tabs[tab.ID] = tab
	app.activeTabID = tab.ID
	app.mu.Unlock()

	entered := make(chan struct{})
	unblock := make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() {
		_, err := app.applyPluginOperation(func() (PluginOperationView, error) {
			close(entered)
			<-unblock
			return PluginOperationView{}, nil
		})
		mutationDone <- err
	}()
	<-entered

	if _, err := ctrl.ExecuteCommand(control.NewSubmitCommand("blocked", "blocked", ""), control.CommandScopeTrusted); err == nil {
		t.Fatal("controller accepted direct work during plugin runtime mutation")
	}
	submitDone := make(chan error, 1)
	go func() { submitDone <- app.SubmitToTab(tab.ID, "after mutation") }()
	select {
	case err := <-submitDone:
		t.Fatalf("Desktop submit crossed plugin mutation gate: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(unblock)
	if err := <-mutationDone; err != nil {
		t.Fatalf("plugin mutation: %v", err)
	}
	select {
	case err := <-submitDone:
		if err != nil {
			t.Fatalf("Desktop submit after plugin mutation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Desktop submit did not resume after plugin mutation")
	}
	ctrl.Cancel()
	ctrl.Close()
}

type desktopPluginRuntimeTool string

func (t desktopPluginRuntimeTool) Name() string        { return string(t) }
func (t desktopPluginRuntimeTool) Description() string { return string(t) }
func (desktopPluginRuntimeTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (desktopPluginRuntimeTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}
func (desktopPluginRuntimeTool) ReadOnly() bool { return true }

func TestDesktopPluginLifecycleUsesApprovedPlans(t *testing.T) {
	isolateDesktopUserDirs(t)
	source := writeDesktopPluginFixture(t, "lifecycle", "1.0.0", []string{pluginpkg.PermissionSkillsLoad})
	app := NewApp()

	plan, err := app.PlanPluginInstall(source, PluginInstallOptions{})
	if err != nil {
		t.Fatalf("plan install: %v", err)
	}
	if !plan.OK || plan.Status != "planned" || plan.PlanID == "" || plan.Applied {
		t.Fatalf("install plan = %+v", plan)
	}
	if _, err := app.InstallPlugin(source, PluginInstallOptions{}); err == nil || !strings.Contains(err.Error(), "planId") {
		t.Fatalf("install without planId error = %v", err)
	}
	installed, err := app.InstallPlugin(source, PluginInstallOptions{PlanID: plan.PlanID})
	if err != nil {
		t.Fatalf("apply install: %v", err)
	}
	if !installed.OK || installed.Status != "done" || !installed.Applied || installed.PlanID != plan.PlanID {
		t.Fatalf("install result = %+v", installed)
	}

	plugins := app.Plugins()
	if len(plugins) != 1 {
		t.Fatalf("plugins = %+v", plugins)
	}
	plugin := plugins[0]
	if plugin.ManifestSchema != pluginpkg.NativeSchemaVersion || plugin.InstallMode != pluginpkg.InstallModeCopy || plugin.Digest == "" {
		t.Fatalf("plugin lifecycle metadata = %+v", plugin)
	}
	if !slices.Equal(plugin.Permissions, []string{pluginpkg.PermissionSkillsLoad}) || len(plugin.GrantedPermissions) != 0 || plugin.Enabled {
		t.Fatalf("plugin grants before enable = %+v", plugin)
	}
	if err := app.SetPluginEnabled(plugin.Name, true, plugin.Digest+"-stale", plugin.Permissions); err == nil {
		t.Fatal("enable accepted stale digest")
	}
	if err := app.SetPluginEnabled(plugin.Name, true, plugin.Digest, nil); err == nil {
		t.Fatal("enable accepted incomplete grants")
	}
	if err := app.SetPluginEnabled(plugin.Name, true, plugin.Digest, plugin.Permissions); err != nil {
		t.Fatalf("enable exact approved package: %v", err)
	}

	writeDesktopPluginManifest(t, source, "lifecycle", "2.0.0", []string{
		pluginpkg.PermissionSkillsLoad,
		pluginpkg.PermissionHooksExecute,
	}, true)
	updatePlan, err := app.PlanPluginUpdate("lifecycle")
	if err != nil {
		t.Fatalf("plan update: %v", err)
	}
	if updatePlan.PlanID == "" || len(updatePlan.Actions) != 1 || !slices.Contains(updatePlan.Actions[0].AddedPermissions, pluginpkg.PermissionHooksExecute) {
		t.Fatalf("update plan = %+v", updatePlan)
	}
	if _, err := app.UpdatePlugin("lifecycle", plan.PlanID); err == nil || !strings.Contains(err.Error(), "planId mismatch") {
		t.Fatalf("update accepted install planId: %v", err)
	}
	updated, err := app.UpdatePlugin("lifecycle", updatePlan.PlanID)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if !updated.OK || updated.Status != "done" || !updated.Actions[0].RollbackAvailable {
		t.Fatalf("update result = %+v", updated)
	}
	plugin = app.Plugins()[0]
	if plugin.Version != "2.0.0" || plugin.Enabled || plugin.Rollback == nil || plugin.Rollback.Version != "1.0.0" {
		t.Fatalf("updated plugin = %+v", plugin)
	}

	rollbackPlan, err := app.PlanPluginRollback("lifecycle")
	if err != nil {
		t.Fatalf("plan rollback: %v", err)
	}
	if rollbackPlan.Status != "planned" || rollbackPlan.PlanID == "" || len(rollbackPlan.Actions) != 1 {
		t.Fatalf("rollback plan = %+v", rollbackPlan)
	}
	rolledBack, err := app.RollbackPlugin("lifecycle", rollbackPlan.PlanID)
	if err != nil {
		t.Fatalf("apply rollback: %v", err)
	}
	if !rolledBack.OK || !rolledBack.Applied || rolledBack.Actions[0].Action != "rollback_plugin_package" {
		t.Fatalf("rollback result = %+v", rolledBack)
	}
	plugin = app.Plugins()[0]
	if plugin.Version != "1.0.0" || !plugin.Enabled || plugin.Rollback == nil || plugin.Rollback.Version != "2.0.0" {
		t.Fatalf("rolled back plugin = %+v", plugin)
	}

	removePlan, err := app.PlanPluginRemove("lifecycle")
	if err != nil {
		t.Fatalf("plan remove: %v", err)
	}
	if removePlan.Status != "planned" || removePlan.PlanID == "" {
		t.Fatalf("remove plan = %+v", removePlan)
	}
	removed, err := app.RemovePlugin("lifecycle", removePlan.PlanID)
	if err != nil {
		t.Fatalf("apply remove: %v", err)
	}
	if !removed.OK || !removed.Applied || len(app.Plugins()) != 0 {
		t.Fatalf("remove result = %+v plugins=%+v", removed, app.Plugins())
	}
}

func TestDesktopPluginDoctorRejectsContentTampering(t *testing.T) {
	isolateDesktopUserDirs(t)
	source := writeDesktopPluginFixture(t, "tampered", "1.0.0", []string{pluginpkg.PermissionSkillsLoad})
	app := NewApp()
	plan, err := app.PlanPluginInstall(source, PluginInstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.InstallPlugin(source, PluginInstallOptions{PlanID: plan.PlanID}); err != nil {
		t.Fatal(err)
	}
	plugin := app.Plugins()[0]
	if err := os.WriteFile(filepath.Join(plugin.Root, "skills", "tampered", "SKILL.md"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnostic := app.PluginDoctor("tampered")
	if !strings.Contains(diagnostic.Error, "digest mismatch") {
		t.Fatalf("doctor after tampering = %+v", diagnostic)
	}
	listed := app.Plugins()
	if len(listed) != 1 || !strings.Contains(listed[0].Error, "digest mismatch") {
		t.Fatalf("plugins after tampering = %+v", listed)
	}
}

func TestDesktopPluginDisableRevokesEveryLiveController(t *testing.T) {
	isolateDesktopUserDirs(t)
	source := writeDesktopPluginFixture(t, "disable-runtime", "1.0.0", []string{pluginpkg.PermissionSkillsLoad})
	app := NewApp()
	plan, err := app.PlanPluginInstall(source, PluginInstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.InstallPlugin(source, PluginInstallOptions{PlanID: plan.PlanID}); err != nil {
		t.Fatal(err)
	}
	installed := app.Plugins()[0]
	if err := app.SetPluginEnabled(installed.Name, true, installed.Digest, installed.Permissions); err != nil {
		t.Fatal(err)
	}

	newRuntime := func() (*control.Controller, *hook.Runner, *tool.Registry) {
		runner := hook.NewRunner([]hook.ResolvedHook{{
			HookConfig: hook.HookConfig{Command: "plugin", Env: map[string]string{"REAMES_AGENT_PLUGIN_NAME": installed.Name}},
			Event:      hook.PostToolUse,
			Scope:      hook.ScopePlugin,
		}}, t.TempDir(), nil, nil)
		registry := tool.NewRegistry()
		registry.Add(desktopPluginRuntimeTool("mcp__owned__connect"))
		registry.Add(desktopPluginRuntimeTool("mcp__user__connect"))
		ctrl := control.New(control.Options{
			WorkspaceRoot:   t.TempDir(),
			Hooks:           runner,
			Skills:          []skill.Skill{{Name: "disable-runtime-skill", Description: "fixture"}},
			Registry:        registry,
			PluginMCPOwners: map[string]string{"owned": installed.Name},
		})
		return ctrl, runner, registry
	}
	visibleCtrl, visibleHooks, visibleRegistry := newRuntime()
	detachedCtrl, detachedHooks, detachedRegistry := newRuntime()
	app.mu.Lock()
	app.tabs["visible"] = &WorkspaceTab{ID: "visible", WorkspaceRoot: t.TempDir(), Ctrl: visibleCtrl}
	app.detachedSessions["detached"] = &WorkspaceTab{ID: "detached", WorkspaceRoot: t.TempDir(), Ctrl: detachedCtrl}
	app.activeTabID = "visible"
	app.mu.Unlock()

	if err := app.SetPluginEnabled(installed.Name, false, installed.Digest, installed.Permissions); err != nil {
		t.Fatal(err)
	}
	for name, ctrl := range map[string]*control.Controller{"visible": visibleCtrl, "detached": detachedCtrl} {
		if got := ctrl.Skills(); len(got) != 0 {
			t.Fatalf("%s controller retained skills after plugin disable: %+v", name, got)
		}
	}
	for name, runner := range map[string]*hook.Runner{"visible": visibleHooks, "detached": detachedHooks} {
		if got := runner.Hooks(); len(got) != 0 {
			t.Fatalf("%s controller retained plugin hooks after disable: %+v", name, got)
		}
	}
	for name, registry := range map[string]*tool.Registry{"visible": visibleRegistry, "detached": detachedRegistry} {
		if _, ok := registry.Get("mcp__owned__connect"); ok {
			t.Fatalf("%s controller retained plugin-owned MCP tools", name)
		}
		if _, ok := registry.Get("mcp__user__connect"); !ok {
			t.Fatalf("%s controller removed user MCP tools", name)
		}
	}
	current, ok, err := pluginpkg.FindInstalled(config.ReamesAgentHomeDir(), installed.Name)
	if err != nil || !ok || current.Enabled {
		t.Fatalf("disabled plugin state = %+v ok=%t err=%v", current, ok, err)
	}
}

func TestSupersedePluginStartupBuildsCancelsOldGenerationBuilds(t *testing.T) {
	app := NewApp()
	var visibleCanceled atomic.Bool
	var siblingCanceled atomic.Bool
	visible := &WorkspaceTab{ID: "visible", Ctrl: control.New(control.Options{})}
	visible.buildGeneration = 7
	visible.buildCancel = func() { visibleCanceled.Store(true) }
	sibling := &WorkspaceTab{ID: "sibling", Ctrl: control.New(control.Options{})}
	sibling.buildGeneration = 11
	sibling.buildCancel = func() { siblingCanceled.Store(true) }
	app.mu.Lock()
	app.tabs[visible.ID] = visible
	app.tabs[sibling.ID] = sibling
	app.activeTabID = visible.ID
	app.mu.Unlock()

	app.supersedePluginStartupBuilds()

	if !visibleCanceled.Load() || !siblingCanceled.Load() {
		t.Fatalf("startup build cancellation = visible:%t sibling:%t", visibleCanceled.Load(), siblingCanceled.Load())
	}
	if visible.buildGeneration != 8 || sibling.buildGeneration != 12 || visible.buildCancel != nil || sibling.buildCancel != nil {
		t.Fatalf("startup build generations not superseded: visible=%d sibling=%d", visible.buildGeneration, sibling.buildGeneration)
	}
}

func TestApplyPluginOperationSupersedesStartupBuildBeforeMutation(t *testing.T) {
	app := NewApp()
	activeCtrl := control.New(control.Options{})
	defer activeCtrl.Close()
	var canceled atomic.Bool
	active := &WorkspaceTab{ID: "active", Ctrl: activeCtrl, Ready: true}
	building := &WorkspaceTab{
		ID:              "building",
		buildGeneration: 4,
		buildCancel:     func() { canceled.Store(true) },
	}
	app.mu.Lock()
	app.tabs[active.ID] = active
	app.tabs[building.ID] = building
	app.activeTabID = active.ID
	app.mu.Unlock()

	_, err := app.applyPluginOperation(func() (PluginOperationView, error) {
		if !canceled.Load() {
			return PluginOperationView{}, fmt.Errorf("old startup build was still admitted when plugin mutation began")
		}
		app.mu.Lock()
		building.removed = true // prevent the post-mutation restart from doing real boot work
		generation := building.buildGeneration
		app.mu.Unlock()
		if generation != 5 {
			return PluginOperationView{}, fmt.Errorf("build generation = %d, want 5", generation)
		}
		return PluginOperationView{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDecodePluginOperationClassifiesStatuses(t *testing.T) {
	tests := []struct {
		status  string
		ok      bool
		wantErr bool
	}{
		{status: "planned", ok: true},
		{status: "done", ok: true},
		{status: "partial", ok: false},
		{status: "failed", ok: false, wantErr: true},
		{status: "blocked", ok: false, wantErr: true},
		{status: "denied", ok: false, wantErr: true},
		{status: "done", ok: false, wantErr: true},
		{status: "unknown", ok: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s-%t", tt.status, tt.ok), func(t *testing.T) {
			applied := tt.status == "done" || tt.status == "partial"
			raw := fmt.Sprintf(`{"ok":%t,"status":%q,"applied":%t,"planId":"test-plan","actions":[{"error":"failed action"}]}`, tt.ok, tt.status, applied)
			got, err := decodePluginOperation(raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decode status %q ok=%t: result=%+v err=%v", tt.status, tt.ok, got, err)
			}
			if got.Status != tt.status {
				t.Fatalf("status = %q, want %q", got.Status, tt.status)
			}
		})
	}
}

func writeDesktopPluginFixture(t *testing.T, name, version string, permissions []string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, "skills", name), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDesktopPluginManifest(t, root, name, version, permissions, false)
	skill := fmt.Sprintf("---\nname: %s\ndescription: Desktop lifecycle fixture\n---\nRun the fixture.\n", name)
	if err := os.WriteFile(filepath.Join(root, "skills", name, "SKILL.md"), []byte(skill), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeDesktopPluginManifest(t *testing.T, root, name, version string, permissions []string, withHook bool) {
	t.Helper()
	quoted := make([]string, len(permissions))
	for i, permission := range permissions {
		quoted[i] = fmt.Sprintf("%q", permission)
	}
	hooks := ""
	if withHook {
		hooks = `,"hooks":{"before_tool":[{"command":"check.cmd"}]}`
		if err := os.WriteFile(filepath.Join(root, "check.cmd"), []byte("@echo off\r\nexit /b 0\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := fmt.Sprintf(`{"schemaVersion":1,"name":%q,"version":%q,"description":"Desktop lifecycle fixture","skills":["skills"],"permissions":[%s]%s}`,
		name, version, strings.Join(quoted, ","), hooks)
	if err := os.WriteFile(filepath.Join(root, pluginpkg.NativeManifest), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}
