package control

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/agent"
	"reames-agent/internal/event"
	"reames-agent/internal/permission"
	"reames-agent/internal/tool"
	"reames-agent/internal/tool/builtin"
)

// TestReplayPendingPromptsReEmitsBlockedApproval proves a tool approval that is
// still blocking the gate is re-emitted on demand, so a frontend that reloaded
// after the original ApprovalRequest can rebuild its modal instead of leaving the
// gate stuck (#3844).
func TestReplayPendingPromptsReEmitsBlockedApproval(t *testing.T) {
	reqs := make(chan event.Approval, 8)
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			reqs <- e.Approval
		}
	})})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = gateApprover{c}.Approve(context.Background(), "bash", "go test ./...", nil)
	}()

	first := <-reqs
	if first.Tool != "bash" || first.Subject != "go test ./..." {
		t.Fatalf("first request = %+v, want bash / go test ./...", first)
	}

	c.ReplayPendingPrompts()

	replayed := <-reqs
	if replayed != first {
		t.Fatalf("replayed = %+v, want identical re-emit of %+v", replayed, first)
	}

	c.Approve(first.ID, true, false, false)
	<-done
}

// TestReplayPendingPromptsPreservesApprovalFileDiff proves a frontend reload
// gets the same patch preview as the original approval prompt. Without this, the
// gate is still answerable after reconnect, but the user loses the critical "what
// will change?" context before approving a writer tool.
func TestReplayPendingPromptsPreservesApprovalFileDiff(t *testing.T) {
	workspace := t.TempDir()
	rel := filepath.Join("notes", "replay.txt")
	args, err := json.Marshal(struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}{Path: rel, Content: "replay\nkeeps diff\n"})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	for _, tl := range (builtin.Workspace{Dir: workspace}).Tools("write_file") {
		reg.Add(tl)
	}
	ag := agent.New(nil, reg, agent.NewSession("sys"), agent.Options{}, event.Discard)
	reqs := make(chan event.Approval, 8)
	c := New(Options{
		Executor:      ag,
		Policy:        permission.New("ask", nil, nil, nil),
		WorkspaceRoot: workspace,
		Sink: event.FuncSink(func(e event.Event) {
			if e.Kind == event.ApprovalRequest {
				reqs <- e.Approval
			}
		}),
	})
	c.EnableInteractiveApproval()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = c.requestApproval(context.Background(), "write_file", rel, args)
	}()

	first := <-reqs
	if first.FileDiff.Diff == "" || first.FileDiff.Added == 0 {
		t.Fatalf("first approval diff = %+v, want patch preview", first.FileDiff)
	}
	if !strings.Contains(first.FileDiff.Diff, "+replay") || !strings.Contains(first.FileDiff.Diff, "+keeps diff") {
		t.Fatalf("first approval diff %q does not show expected additions", first.FileDiff.Diff)
	}

	c.ReplayPendingPrompts()
	replayed := <-reqs
	if replayed.ID != first.ID || replayed.Tool != first.Tool || replayed.Subject != first.Subject {
		t.Fatalf("replayed approval = %+v, want same identity as %+v", replayed, first)
	}
	if replayed.FileDiff != first.FileDiff {
		t.Fatalf("replayed diff = %+v, want identical preview %+v", replayed.FileDiff, first.FileDiff)
	}

	c.Approve(first.ID, false, false, false)
	<-done
}

// TestReplayPendingPromptsReEmitsBlockedAsk proves the same for a blocked `ask`
// question, including its question payload (which the controller now retains).
func TestReplayPendingPromptsReEmitsBlockedAsk(t *testing.T) {
	asks := make(chan event.Ask, 8)
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.AskRequest {
			asks <- e.Ask
		}
	})})

	questions := []event.AskQuestion{{
		ID:      "q1",
		Header:  "Pick",
		Prompt:  "Which option?",
		Options: []event.AskOption{{Label: "A"}, {Label: "B"}},
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Ask(context.Background(), questions)
	}()

	first := <-asks
	c.ReplayPendingPrompts()
	replayed := <-asks

	if replayed.ID != first.ID || len(replayed.Questions) != 1 || replayed.Questions[0].Prompt != "Which option?" {
		t.Fatalf("replayed ask = %+v, want same id and questions as %+v", replayed, first)
	}

	c.AnswerQuestion(first.ID, []event.AskAnswer{{QuestionID: "q1", Selected: []string{"A"}}})
	<-done
}

// TestReplayPendingPromptsNoOpWhenIdle proves replay emits nothing when no prompt
// is outstanding, so a frontend (re)connect on an idle session is silent.
func TestReplayPendingPromptsNoOpWhenIdle(t *testing.T) {
	var count int
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest || e.Kind == event.AskRequest {
			count++
		}
	})})

	c.ReplayPendingPrompts()
	if count != 0 {
		t.Fatalf("emitted %d prompts with nothing pending, want 0", count)
	}
}
