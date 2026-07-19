package main

import (
	"errors"
	"testing"
	"time"

	"reames-agent/internal/control"
)

func TestMCPMutationEntryPointsRejectSiblingRuntimeWork(t *testing.T) {
	isolateDesktopUserDirs(t)
	active := control.New(control.Options{})
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	sibling := control.New(control.Options{Runner: runner})
	defer active.Close()
	defer sibling.Close()
	defer close(runner.release)

	app := NewApp()
	app.tabs = map[string]*WorkspaceTab{
		"active":  {ID: "active", Ctrl: active, Ready: true, disabledMCP: map[string]ServerView{}},
		"sibling": {ID: "sibling", Ctrl: sibling, Ready: true, disabledMCP: map[string]ServerView{}},
	}
	app.activeTabID = "active"

	sibling.Submit("keep the sibling runtime busy")
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("sibling turn did not start")
	}

	operations := map[string]func() error{
		"add": func() error {
			_, err := app.AddMCPServer(MCPServerInput{Name: "fixture", Command: "fixture"})
			return err
		},
		"update": func() error {
			return app.UpdateMCPServer("fixture", MCPServerInput{Name: "fixture", Command: "fixture"})
		},
		"remove":      func() error { return app.RemoveMCPServer("fixture") },
		"reconnect":   func() error { return app.ReconnectMCPServer("fixture") },
		"reverify":    func() error { return app.ReverifyMCPServer("fixture") },
		"clear-auth":  func() error { return app.ClearMCPServerAuthentication("fixture") },
		"trust":       func() error { return app.TrustMCPServerTool("fixture", "read") },
		"untrust":     func() error { return app.UntrustMCPServerTool("fixture", "read") },
		"disable":     func() error { return app.SetMCPServerEnabled("fixture", false) },
		"legacy-tier": func() error { return app.SetMCPServerTier("fixture", "background") },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()
			var busy *rebuildBusyError
			if !errors.As(err, &busy) {
				t.Fatalf("MCP mutation error = %v, want sibling active-work guard", err)
			}
		})
	}
}

func TestMCPRuntimeReservationBlocksNewSiblingTurnUntilRelease(t *testing.T) {
	active := control.New(control.Options{})
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	sibling := control.New(control.Options{Runner: runner})
	defer active.Close()
	defer sibling.Close()
	defer close(runner.release)

	app := NewApp()
	app.tabs = map[string]*WorkspaceTab{
		"active":  {ID: "active", Ctrl: active, Ready: true},
		"sibling": {ID: "sibling", Ctrl: sibling, Ready: true},
	}
	app.activeTabID = "active"

	releaseMutation, err := app.beginMCPRuntimeMutation()
	if err != nil {
		t.Fatalf("beginMCPRuntimeMutation: %v", err)
	}
	sibling.Submit("must not enter during the MCP mutation")
	select {
	case <-runner.started:
		releaseMutation()
		t.Fatal("sibling turn entered while the MCP runtime reservation was held")
	case <-time.After(100 * time.Millisecond):
	}

	releaseMutation()
	sibling.Submit("may enter after the MCP mutation")
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("sibling turn did not start after the MCP runtime reservation released")
	}
}

func TestTabControllerBuildAdmissionWaitsForRuntimeMutation(t *testing.T) {
	app := NewApp()
	tab := &WorkspaceTab{ID: "late", removed: true}

	app.runtimeBuildGate.Lock()
	attempting := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(attempting)
		app.startTabControllerBuild(tab)
		close(done)
	}()
	<-attempting
	select {
	case <-done:
		app.runtimeBuildGate.Unlock()
		t.Fatal("late controller build bypassed the runtime mutation admission gate")
	case <-time.After(100 * time.Millisecond):
	}

	app.runtimeBuildGate.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("late controller build did not resume after runtime mutation release")
	}
}
