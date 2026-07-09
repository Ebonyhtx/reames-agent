package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reames-agent/internal/config"
)

func TestRestoreOrBuildTabsRestoresSavedProjectSessionWorkspaceAndActiveTab(t *testing.T) {
	isolateDesktopUserDirs(t)
	cfg := config.LoadForEdit(config.UserConfigPath())
	if err := cfg.SetDesktopLayoutStyle("classic"); err != nil {
		t.Fatalf("SetDesktopLayoutStyle: %v", err)
	}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save user config: %v", err)
	}

	projectRoot := t.TempDir()
	if err := addProject(projectRoot, "Restart Project"); err != nil {
		t.Fatalf("add project: %v", err)
	}

	globalRoot := globalTabWorkspaceRoot()
	globalDir := desktopSessionDir(globalRoot)
	projectDir := desktopSessionDir(projectRoot)
	for _, dir := range []string{globalDir, projectDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir session dir %s: %v", dir, err)
		}
	}
	topicGlobal := "topic_restart_global"
	topicProject := "topic_restart_project"
	globalSession := writeTopicSessionWithPrompt(t, globalDir, "restart-global.jsonl", topicGlobal, "Restart Global", "", "global restart prompt", time.Now().Add(-time.Hour))
	projectSession := writeTopicSessionWithPrompt(t, projectDir, "restart-project.jsonl", topicProject, "Restart Project", projectRoot, "project restart prompt", time.Now())

	previous := NewApp()
	globalTab := previous.createTabEntryWithID("global", globalRoot, topicGlobal, "tab-global")
	globalTab.TopicTitle = "Restart Global"
	globalTab.SessionPath = globalSession
	projectTab := previous.createTabEntryWithID("project", projectRoot, topicProject, "tab-project")
	projectTab.TopicTitle = "Restart Project"
	projectTab.SessionPath = projectSession
	previous.tabs[globalTab.ID] = globalTab
	previous.tabs[projectTab.ID] = projectTab
	previous.tabOrder = []string{globalTab.ID, projectTab.ID}
	previous.activeTabID = projectTab.ID
	previous.mu.Lock()
	previous.saveTabsLocked()
	previous.mu.Unlock()

	restarted := NewApp()
	restarted.ctx = context.Background()
	restarted.readyHook = func() {}
	installNoopRuntimeEvents(restarted)
	restarted.tabsRestored = make(chan struct{})
	restarted.restoreOrBuildTabs()
	select {
	case <-restarted.tabsRestoredSignal():
	case <-time.After(2 * time.Second):
		t.Fatal("restoreOrBuildTabs did not mark tabs restored")
	}

	tabs := restarted.ListTabs()
	assertTabIDs(t, tabs, globalTab.ID, projectTab.ID)
	if !tabs[1].Active || tabs[0].Active {
		t.Fatalf("restored active tab flags = %+v, want project tab active", tabs)
	}
	if got := normalizeProjectRoot(tabs[1].WorkspaceRoot); got != normalizeProjectRoot(projectRoot) {
		t.Fatalf("project tab workspace = %q, want %q", got, normalizeProjectRoot(projectRoot))
	}

	restoredProject := waitForTabReady(t, restarted, projectTab.ID)
	restoredGlobal := waitForTabReady(t, restarted, globalTab.ID)
	if got := filepath.Clean(restoredProject.Ctrl.SessionPath()); got != filepath.Clean(projectSession) {
		t.Fatalf("project session path = %q, want %q", got, projectSession)
	}
	if got := normalizeProjectRoot(restoredProject.Ctrl.WorkspaceRoot()); got != normalizeProjectRoot(projectRoot) {
		t.Fatalf("project controller workspace = %q, want %q", got, normalizeProjectRoot(projectRoot))
	}
	if got := filepath.Clean(restoredGlobal.Ctrl.SessionPath()); got != filepath.Clean(globalSession) {
		t.Fatalf("global session path = %q, want %q", got, globalSession)
	}

	projectHistory := restarted.HistoryForTab(projectTab.ID)
	if len(projectHistory) == 0 || projectHistory[0].Content != "project restart prompt" {
		t.Fatalf("project history = %+v, want restored project prompt", projectHistory)
	}
	globalHistory := restarted.HistoryForTab(globalTab.ID)
	if len(globalHistory) == 0 || globalHistory[0].Content != "global restart prompt" {
		t.Fatalf("global history = %+v, want restored global prompt", globalHistory)
	}
}
