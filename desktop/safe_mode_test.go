package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/config"
	"reames-agent/internal/control"
	"reames-agent/internal/repair"
)

type shutdownSnapshotController struct {
	control.SessionAPI
	calls           []string
	normalSnapshots int
	sessionPath     string
	shutdown        func() error
}

func (c *shutdownSnapshotController) Snapshot() error {
	c.normalSnapshots++
	return nil
}

func (c *shutdownSnapshotController) SnapshotForShutdown() error {
	c.calls = append(c.calls, "shutdown-snapshot")
	if c.shutdown != nil {
		return c.shutdown()
	}
	return nil
}

func (c *shutdownSnapshotController) SessionPath() string {
	if c.sessionPath != "" {
		return c.sessionPath
	}
	if c.SessionAPI != nil {
		return c.SessionAPI.SessionPath()
	}
	return ""
}

func (c *shutdownSnapshotController) Close() {
	c.calls = append(c.calls, "close")
	if c.SessionAPI != nil {
		c.SessionAPI.Close()
	}
}

func TestParseDesktopLaunchArgs(t *testing.T) {
	if parseDesktopLaunchArgs([]string{"-devserver", "x"}).SafeMode {
		t.Fatal("unrelated Wails flags enabled Safe Mode")
	}
	if !parseDesktopLaunchArgs([]string{"--home", "x", "--safe-mode"}).SafeMode {
		t.Fatal("--safe-mode was not detected")
	}
}

func TestShutdownDoesNotBlessStartupBeforeReady(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	tracker := repair.NewStartupTracker(filepath.Join(t.TempDir(), "startup.json"))
	if _, err := tracker.Begin("v1", false); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.startupTracker = tracker
	a.shutdown(context.Background())
	state, err := tracker.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != "starting" {
		t.Fatalf("pre-ready shutdown phase = %q", state.Phase)
	}
	a.startupReady.Store(true)
	a.shutdown(context.Background())
	state, err = tracker.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != "clean-exit" {
		t.Fatalf("ready shutdown phase = %q", state.Phase)
	}
}

func TestShutdownUsesDurableSnapshotBeforeClosingController(t *testing.T) {
	isolateDesktopUserDirs(t)
	ctrl := &shutdownSnapshotController{SessionAPI: control.New(control.Options{Label: "shutdown"})}
	a := NewApp()
	a.tabs["tab"] = &WorkspaceTab{ID: "tab", Ctrl: ctrl}
	a.tabOrder = []string{"tab"}

	a.shutdown(context.Background())

	if ctrl.normalSnapshots != 0 {
		t.Fatalf("ordinary Snapshot calls = %d, want shutdown-specific persistence", ctrl.normalSnapshots)
	}
	if len(ctrl.calls) != 2 || ctrl.calls[0] != "shutdown-snapshot" || ctrl.calls[1] != "close" {
		t.Fatalf("shutdown call order = %v, want [shutdown-snapshot close]", ctrl.calls)
	}
}

func TestShutdownPersistsRecoveryPathCommittedAfterCallback(t *testing.T) {
	isolateDesktopUserDirs(t)
	dir := t.TempDir()
	originalPath := filepath.Join(dir, "original.jsonl")
	recoveryPath := filepath.Join(dir, "original-recovery.jsonl")
	a := NewApp()
	ctrl := &shutdownSnapshotController{
		SessionAPI:  control.New(control.Options{Label: "shutdown", SessionPath: originalPath}),
		sessionPath: originalPath,
	}
	tab := &WorkspaceTab{ID: "tab", Ctrl: ctrl, SessionPath: originalPath}
	a.tabs[tab.ID] = tab
	a.tabOrder = []string{tab.ID}
	a.activeTabID = tab.ID
	ctrl.shutdown = func() error {
		err := a.handleTabSessionRecovered(tab)(control.SessionRecoveryInfo{
			OriginalPath: originalPath,
			RecoveryPath: recoveryPath,
		})
		if err == nil {
			// Force a newer ordinary layout write while Controller still exposes
			// the old path. The recovery lease must keep this write anchored to
			// recovery instead of undoing the callback's first save.
			a.mu.Lock()
			a.saveTabsLocked()
			a.mu.Unlock()
			// Controller.commitRecoveredSession updates its path only after the
			// callback succeeds. Mirror that ordering exactly.
			ctrl.sessionPath = recoveryPath
		}
		return err
	}

	a.shutdown(context.Background())

	saved := loadTabsFile()
	if len(saved.Tabs) != 1 || saved.Tabs[0].ID != tab.ID {
		t.Fatalf("saved tabs = %+v, want recovered tab %q", saved.Tabs, tab.ID)
	}
	if got := saved.Tabs[0].SessionPath; got != recoveryPath {
		t.Fatalf("saved shutdown session path = %q, want recovery path %q", got, recoveryPath)
	}
	if len(ctrl.calls) != 2 || ctrl.calls[0] != "shutdown-snapshot" || ctrl.calls[1] != "close" {
		t.Fatalf("shutdown call order = %v, want [shutdown-snapshot close]", ctrl.calls)
	}
}

func TestSafeModeDoesNotRestoreSavedTabs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	bad := []byte("[broken\n")
	if err := os.WriteFile(config.UserConfigPath(), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := config.Load(); err != nil || !cfg.SafeMode() {
		t.Fatalf("Safe Mode config = %+v, %v", cfg, err)
	}
	model, approval := desktopNewSessionDefaults()
	want := config.LoadRecoveryDefaultsForRoot("")
	if model != want.DefaultModel || approval != normalizeToolApprovalMode(want.DesktopDefaultToolApprovalMode()) {
		t.Fatalf("Safe Mode session defaults = %q/%q", model, approval)
	}
	got, err := os.ReadFile(config.UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(bad) {
		t.Fatalf("Safe Mode rewrote config: %q", got)
	}
}

func TestSafeModeTabIsRecoveryOnly(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	a := NewApp()
	tab := a.createTabEntry("global", globalTabWorkspaceRoot(), "")
	a.mu.Lock()
	a.tabs[tab.ID] = tab
	a.mu.Unlock()
	a.startTabControllerBuild(tab)
	if !tab.Ready || tab.Ctrl != nil || !strings.Contains(tab.StartupErr, "recovery-only") {
		t.Fatalf("Safe Mode tab = ready:%v ctrl:%v err:%q", tab.Ready, tab.Ctrl, tab.StartupErr)
	}
}
