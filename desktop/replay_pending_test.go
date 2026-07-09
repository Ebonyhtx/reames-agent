package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/agent"
	agenttest "reames-agent/internal/agent/testutil"
	"reames-agent/internal/control"
	"reames-agent/internal/permission"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
	"reames-agent/internal/tool/builtin"
)

func TestDesktopReplayPendingPromptsReEmitsApprovalWithTabAndDiff(t *testing.T) {
	isolateDesktopUserDirs(t)

	workspace := t.TempDir()
	sessionDir := t.TempDir()
	rel := filepath.Join("notes", "desktop-replay.txt")
	args, err := json.Marshal(struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{Path: rel, Content: "desktop replay\nkeeps approval diff\n"})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: workspace}).Tools("write_file") {
		reg.Add(tl)
	}
	prov := agenttest.NewMock("desktop-replay",
		agenttest.Turn{ToolCalls: []provider.ToolCall{{
			ID:        "write-1",
			Name:      "write_file",
			Arguments: string(args),
		}}},
		agenttest.Turn{Text: "done"},
	)

	app := NewApp()
	events := make(chan wireEventTab, 16)
	sink := &tabEventSink{tabID: "tab-replay", app: app, ctx: context.Background()}
	sink.runtimeEvents.emit = func(_ context.Context, name string, payload ...interface{}) {
		if name != eventChannel {
			return
		}
		if len(payload) != 1 {
			t.Errorf("payload count = %d, want 1", len(payload))
			return
		}
		wire, ok := payload[0].(wireEventTab)
		if !ok {
			t.Errorf("payload type = %T, want wireEventTab", payload[0])
			return
		}
		events <- wire
	}

	ag := agent.New(prov, reg, agent.NewSession("sys"), agent.Options{}, sink)
	ctrl := control.New(control.Options{
		Runner:        ag,
		Executor:      ag,
		Policy:        permission.New("ask", nil, nil, nil),
		Sink:          sink,
		SessionDir:    sessionDir,
		SessionPath:   agent.NewSessionPath(sessionDir, "desktop-replay"),
		WorkspaceRoot: workspace,
		Label:         "desktop-replay",
	})
	ctrl.EnableInteractiveApproval()
	defer ctrl.Close()

	tab := &WorkspaceTab{
		ID:            "tab-replay",
		Scope:         "project",
		WorkspaceRoot: workspace,
		SessionPath:   ctrl.SessionPath(),
		Ctrl:          ctrl,
		sink:          sink,
		Ready:         true,
	}
	app.tabs[tab.ID] = tab
	app.tabOrder = []string{tab.ID}
	app.activeTabID = tab.ID

	ctrl.Submit("write the replay note")
	first := waitForDesktopApproval(t, events)
	if first.TabID != tab.ID {
		t.Fatalf("first approval tabId = %q, want %q", first.TabID, tab.ID)
	}
	if first.Approval == nil || first.Approval.Tool != "write_file" {
		t.Fatalf("first approval = %+v, want write_file approval", first.Approval)
	}
	if first.Approval.Diff == "" || first.Approval.Added == 0 {
		t.Fatalf("first approval diff = %+v, want patch preview", first.Approval)
	}
	if !strings.Contains(first.Approval.Diff, "+desktop replay") ||
		!strings.Contains(first.Approval.Diff, "+keeps approval diff") {
		t.Fatalf("first approval diff %q does not show expected additions", first.Approval.Diff)
	}

	app.ReplayPendingPrompts()
	replayed := waitForDesktopApproval(t, events)
	if replayed.TabID != tab.ID {
		t.Fatalf("replayed approval tabId = %q, want %q", replayed.TabID, tab.ID)
	}
	if replayed.Approval == nil {
		t.Fatal("replayed approval is nil")
	}
	if replayed.Approval.ID != first.Approval.ID || replayed.Approval.Subject != first.Approval.Subject {
		t.Fatalf("replayed approval = %+v, want same prompt as %+v", replayed.Approval, first.Approval)
	}
	if replayed.Approval.Diff != first.Approval.Diff ||
		replayed.Approval.Added != first.Approval.Added ||
		replayed.Approval.Removed != first.Approval.Removed {
		t.Fatalf("replayed approval diff = %+v, want same preview as %+v", replayed.Approval, first.Approval)
	}

	ctrl.Approve(first.Approval.ID, true, false, false)
	waitNotRunning(t, ctrl)
}

func waitForDesktopApproval(t *testing.T, events <-chan wireEventTab) wireEventTab {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case wire := <-events:
			if wire.Kind == "approval_request" {
				return wire
			}
		case <-deadline:
			t.Fatal("timed out waiting for desktop approval_request")
		}
	}
}
