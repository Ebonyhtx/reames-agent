package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reames-agent/internal/config"
)

type savedRestartTabsFixture struct {
	projectRoot    string
	globalTab      *WorkspaceTab
	projectTab     *WorkspaceTab
	globalSession  string
	projectSession string
}

func TestRestoreOrBuildTabsRestoresSavedProjectSessionWorkspaceAndActiveTab(t *testing.T) {
	isolateDesktopUserDirs(t)
	cfg := config.LoadForEdit(config.UserConfigPath())
	if err := cfg.SetDesktopLayoutStyle("classic"); err != nil {
		t.Fatalf("SetDesktopLayoutStyle: %v", err)
	}
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatalf("save user config: %v", err)
	}

	fixture := saveRestartTabsFixture(t)
	restarted := restoreSavedTabsForTest(t)

	tabs := restarted.ListTabs()
	assertTabIDs(t, tabs, fixture.globalTab.ID, fixture.projectTab.ID)
	if !tabs[1].Active || tabs[0].Active {
		t.Fatalf("restored active tab flags = %+v, want project tab active", tabs)
	}
	if got := normalizeProjectRoot(tabs[1].WorkspaceRoot); got != normalizeProjectRoot(fixture.projectRoot) {
		t.Fatalf("project tab workspace = %q, want %q", got, normalizeProjectRoot(fixture.projectRoot))
	}

	restoredProject := waitForTabReady(t, restarted, fixture.projectTab.ID)
	restoredGlobal := waitForTabReady(t, restarted, fixture.globalTab.ID)
	assertRestoredRestartProjectTab(t, restarted, restoredProject, fixture)
	if got := filepath.Clean(restoredGlobal.Ctrl.SessionPath()); got != filepath.Clean(fixture.globalSession) {
		t.Fatalf("global session path = %q, want %q", got, fixture.globalSession)
	}
	globalHistory := restarted.HistoryForTab(fixture.globalTab.ID)
	if len(globalHistory) == 0 || globalHistory[0].Content != "global restart prompt" {
		t.Fatalf("global history = %+v, want restored global prompt", globalHistory)
	}
}

func TestRestoreOrBuildTabsWorkbenchRestoresOnlyActiveProjectSession(t *testing.T) {
	isolateDesktopUserDirs(t)

	fixture := saveRestartTabsFixture(t)
	restarted := restoreSavedTabsForTest(t)

	tabs := restarted.ListTabs()
	assertTabIDs(t, tabs, fixture.projectTab.ID)
	if !tabs[0].Active {
		t.Fatalf("workbench restored tab is not active: %+v", tabs[0])
	}
	if got := normalizeProjectRoot(tabs[0].WorkspaceRoot); got != normalizeProjectRoot(fixture.projectRoot) {
		t.Fatalf("workbench project tab workspace = %q, want %q", got, normalizeProjectRoot(fixture.projectRoot))
	}
	restoredProject := waitForTabReady(t, restarted, fixture.projectTab.ID)
	assertRestoredRestartProjectTab(t, restarted, restoredProject, fixture)
}

func saveRestartTabsFixture(t *testing.T) savedRestartTabsFixture {
	t.Helper()
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
	return savedRestartTabsFixture{
		projectRoot:    projectRoot,
		globalTab:      globalTab,
		projectTab:     projectTab,
		globalSession:  globalSession,
		projectSession: projectSession,
	}
}

func restoreSavedTabsForTest(t *testing.T) *App {
	t.Helper()
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
	return restarted
}

func assertRestoredRestartProjectTab(t *testing.T, app *App, tab *WorkspaceTab, fixture savedRestartTabsFixture) {
	t.Helper()
	if got := filepath.Clean(tab.Ctrl.SessionPath()); got != filepath.Clean(fixture.projectSession) {
		t.Fatalf("project session path = %q, want %q", got, fixture.projectSession)
	}
	if got := normalizeProjectRoot(tab.Ctrl.WorkspaceRoot()); got != normalizeProjectRoot(fixture.projectRoot) {
		t.Fatalf("project controller workspace = %q, want %q", got, normalizeProjectRoot(fixture.projectRoot))
	}

	projectHistory := app.HistoryForTab(fixture.projectTab.ID)
	if len(projectHistory) == 0 || projectHistory[0].Content != "project restart prompt" {
		t.Fatalf("project history = %+v, want restored project prompt", projectHistory)
	}
}
