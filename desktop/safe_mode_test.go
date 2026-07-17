package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/config"
	"reames-agent/internal/repair"
)

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
