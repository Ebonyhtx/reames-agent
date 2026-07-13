package control

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/agent"
	"reames-agent/internal/checkpoint"
	"reames-agent/internal/diff"
	"reames-agent/internal/event"
	"reames-agent/internal/evidence"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func rewriteCheckpointRecord(t *testing.T, c *Controller, turn int, mutate func(*checkpoint.Checkpoint)) {
	t.Helper()
	path := filepath.Join(ckptDir(c.SessionPath()), fmt.Sprintf("turn-%d.json", turn))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record checkpoint.Checkpoint
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	mutate(&record)
	data, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	c.rebindCheckpoints(c.SessionPath())
}

func runTwoTurns(t *testing.T) (*Controller, *agent.Agent, *[]event.Event) {
	t.Helper()
	dir := t.TempDir()
	prov := &scriptedTurns{turns: [][]provider.Chunk{
		textTurn("first answer"),
		textTurn("second answer"),
		textTurn("edited answer"),
	}}
	ag := agent.New(prov, tool.NewRegistry(), agent.NewSession("sys"), agent.Options{}, event.Discard)
	var events []event.Event
	c := New(Options{
		Runner:        ag,
		Executor:      ag,
		SessionDir:    dir,
		WorkspaceRoot: dir,
		Label:         "test",
		Sink:          event.FuncSink(func(e event.Event) { events = append(events, e) }),
	})
	c.SetSessionPath(agent.NewSessionPath(dir, "test"))
	if err := c.runTurnWithRaw(context.Background(), "first prompt", "first prompt"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if err := c.runTurnWithRaw(context.Background(), "second prompt", "second prompt"); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	return c, ag, &events
}

func TestRewindBothPreflightsConversationBeforeChangingCode(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.mu.Unlock()
	path := filepath.Join(c.workspaceRoot, "preflight.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.checkpoints.snapshot(diff.Change{Path: path, Kind: diff.Modify, OldText: "before"})
	if err := os.WriteFile(path, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make the conversation boundary stale. RewindBoth must fail before the
	// checkpoint restore plan is applied.
	ag.Session().Replace([]provider.Message{{Role: provider.RoleUser, Content: "compacted"}})
	if err := c.Rewind(lastTurn, RewindBoth); err == nil || !strings.Contains(err.Error(), "compacted") {
		t.Fatalf("RewindBoth error = %v, want compacted-boundary failure", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "edited" {
		t.Fatalf("RewindBoth changed code before preflight: %q", data)
	}
}

func TestForkRejectsCompactedConversationBoundary(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.mu.Unlock()
	originalPath := c.SessionPath()

	ag.Session().Replace([]provider.Message{{Role: provider.RoleUser, Content: "compacted"}})
	if _, err := c.Fork(lastTurn); err == nil || !strings.Contains(err.Error(), "compacted") {
		t.Fatalf("Fork error = %v, want compacted-boundary failure", err)
	}
	if c.SessionPath() != originalPath {
		t.Fatalf("failed fork switched session from %q to %q", originalPath, c.SessionPath())
	}
}

func TestRewindForkAndSummarizeRejectSameLengthTranscriptDivergence(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.mu.Unlock()
	originalPath := c.SessionPath()
	msgs := ag.Session().Snapshot()
	msgs[1].Content = "rewritten first prompt"
	ag.Session().Replace(msgs)

	if err := c.Rewind(lastTurn, RewindConversation); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("Rewind divergence error = %v", err)
	}
	if ag.Session().Len() != len(msgs) || ag.Session().Snapshot()[1].Content != "rewritten first prompt" {
		t.Fatalf("failed rewind mutated divergent transcript: %+v", ag.Session().Snapshot())
	}
	if _, err := c.Fork(lastTurn); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("Fork divergence error = %v", err)
	}
	if err := c.SummarizeFrom(context.Background(), lastTurn); err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("Summarize divergence error = %v", err)
	}
	if c.SessionPath() != originalPath {
		t.Fatalf("failed fork switched session from %q to %q", originalPath, c.SessionPath())
	}
}

func TestRewindForkAndSummarizeRejectLegacyCheckpointWithoutDigest(t *testing.T) {
	c, _, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.mu.Unlock()
	rewriteCheckpointRecord(t, c, lastTurn, func(record *checkpoint.Checkpoint) {
		record.TranscriptDigest = ""
	})

	if err := c.Rewind(lastTurn, RewindConversation); err == nil || !strings.Contains(err.Error(), "legacy checkpoint") {
		t.Fatalf("Rewind legacy checkpoint error = %v", err)
	}
	if _, err := c.Fork(lastTurn); err == nil || !strings.Contains(err.Error(), "legacy checkpoint") {
		t.Fatalf("Fork legacy checkpoint error = %v", err)
	}
	if err := c.SummarizeFrom(context.Background(), lastTurn); err == nil || !strings.Contains(err.Error(), "legacy checkpoint") {
		t.Fatalf("Summarize legacy checkpoint error = %v", err)
	}
}

func TestRewindRejectsInvalidRuntimeBeforeMutatingSession(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.mu.Unlock()
	rewriteCheckpointRecord(t, c, lastTurn, func(record *checkpoint.Checkpoint) {
		record.Runtime = json.RawMessage(`{"version":999,"status":"running","goal":"future format"}`)
	})
	c.SetGoal("future goal")
	c.SetPlanMode(true)
	ag.SeedTodoState([]evidence.TodoItem{{Content: "future todo", Status: "in_progress"}})
	before := ag.Session().Snapshot()

	if err := c.Rewind(lastTurn, RewindConversation); err == nil || !strings.Contains(err.Error(), "invalid checkpoint runtime") {
		t.Fatalf("Rewind invalid runtime error = %v", err)
	}
	if got := ag.Session().Snapshot(); len(got) != len(before) || got[len(got)-1].Content != before[len(before)-1].Content {
		t.Fatalf("invalid runtime rewind mutated transcript: before=%+v after=%+v", before, got)
	}
	if c.Goal() != "future goal" || !c.PlanMode() {
		t.Fatalf("invalid runtime rewind mutated runtime: goal=%q plan=%v", c.Goal(), c.PlanMode())
	}
	if todos := c.Todos(); len(todos) != 1 || todos[0].Content != "future todo" {
		t.Fatalf("invalid runtime rewind mutated todos: %+v", todos)
	}
}

func TestRewindRejectsNegativeBoundaryWithoutPanic(t *testing.T) {
	c, _, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.bound[lastTurn] = -1
	c.checkpoints.mu.Unlock()
	if err := c.Rewind(lastTurn, RewindConversation); err == nil || !strings.Contains(err.Error(), "negative boundary") {
		t.Fatalf("Rewind negative boundary error = %v", err)
	}
	if err := c.SummarizeUpTo(context.Background(), lastTurn); err == nil || !strings.Contains(err.Error(), "negative boundary") {
		t.Fatalf("Summarize negative boundary error = %v", err)
	}
}

func TestRewindConversationRestoresCheckpointRuntimeProjection(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	c.checkpoints.mu.Unlock()

	// These fields describe state after the target checkpoint and must not
	// survive truncating the transcript back to its turn-start anchor.
	ag.SeedTodoState([]evidence.TodoItem{{Content: "future task", Status: "in_progress"}})
	c.SetGoal("future goal")
	c.SetPlanMode(true)
	if err := c.Rewind(lastTurn, RewindConversation); err != nil {
		t.Fatal(err)
	}
	if c.Goal() != "" || c.GoalStatus() != GoalStatusStopped || c.PlanMode() {
		t.Fatalf("runtime after rewind = goal %q status %q plan %v", c.Goal(), c.GoalStatus(), c.PlanMode())
	}
	if todos := c.Todos(); len(todos) != 0 {
		t.Fatalf("future todos survived rewind: %+v", todos)
	}
	loaded, err := agent.LoadSession(c.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	fresh := agent.New(nil, nil, agent.NewSession("sys"), agent.Options{}, event.Discard)
	resumed := New(Options{Executor: fresh, SessionDir: filepath.Dir(c.SessionPath()), Label: "test"})
	resumed.Resume(loaded, c.SessionPath())
	if resumed.Goal() != "" || resumed.PlanMode() || len(resumed.Todos()) != 0 {
		t.Fatalf("cold resume diverged after rewind: goal=%q plan=%v todos=%+v", resumed.Goal(), resumed.PlanMode(), resumed.Todos())
	}
}

// TestRewindConversationFailsLoudlyAfterCompaction reproduces #3598: once
// compaction shrinks the message log below a turn's recorded boundary, a
// conversation/both rewind to that turn skipped the truncation but still emitted
// a success notice — code rolled back, conversation silently did not.
func TestRewindConversationFailsLoudlyAfterCompaction(t *testing.T) {
	c, ag, events := runTwoTurns(t)

	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	boundary := c.checkpoints.bound[lastTurn]
	c.checkpoints.mu.Unlock()
	if boundary <= 1 {
		t.Fatalf("expected the latest turn's boundary above 1, got bound=%v", c.checkpoints.bound)
	}

	// Auto-compaction replaces the prefix with a summary, shrinking the log below
	// the recorded boundary; compaction does not rewrite checkpoint boundaries.
	sess := ag.Session()
	sess.Messages = []provider.Message{{Role: provider.RoleUser, Content: "summary"}}

	*events = nil
	err := c.Rewind(lastTurn, RewindBoth)
	if err == nil || !strings.Contains(err.Error(), "compacted") {
		t.Fatalf("Rewind after compaction error = %v, want a 'compacted past' failure", err)
	}
	for _, e := range *events {
		if e.Kind == event.Notice && strings.Contains(e.Text, "rewound conversation") {
			t.Fatalf("emitted a false conversation-rewind success after skipping truncation: %q", e.Text)
		}
	}
	if got := len(ag.Session().Messages); got != 1 {
		t.Fatalf("session messages = %d, want the compacted log left intact at 1", got)
	}
}

// TestRewindConversationSucceedsWithLiveBoundary is the companion happy path: a
// boundary still within the log truncates the conversation and reports success.
func TestRewindConversationSucceedsWithLiveBoundary(t *testing.T) {
	c, ag, events := runTwoTurns(t)

	c.checkpoints.mu.Lock()
	lastTurn := c.checkpoints.turn - 1
	boundary := c.checkpoints.bound[lastTurn]
	c.checkpoints.mu.Unlock()

	*events = nil
	if err := c.Rewind(lastTurn, RewindConversation); err != nil {
		t.Fatalf("Rewind with a live boundary: %v", err)
	}
	if got := len(ag.Session().Messages); got != boundary {
		t.Fatalf("session truncated to %d messages, want boundary %d", got, boundary)
	}
	ok := false
	for _, e := range *events {
		if e.Kind == event.Notice && strings.Contains(e.Text, "rewound conversation") {
			ok = true
		}
	}
	if !ok {
		t.Fatal("expected a conversation-rewind success notice")
	}
}

func TestEditPromptPersistsOriginalPrompt(t *testing.T) {
	c, ag, _ := runTwoTurns(t)

	if err := c.Rewind(1, RewindConversation); err != nil {
		t.Fatal(err)
	}
	c.SubmitEditedDisplay("edited prompt", "edited prompt", "second prompt")
	defer c.autosaveWG.Wait()

	var loaded *agent.Session
	deadline := time.Now().Add(time.Second)
	for {
		var err error
		loaded, err = agent.LoadSession(c.SessionPath())
		if err == nil {
			msgs := loaded.Snapshot()
			if len(msgs) >= 2 {
				last := msgs[len(msgs)-2]
				if last.Role == provider.RoleUser && last.Content == "edited prompt" {
					break
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("edited prompt was not persisted before deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	msgs := loaded.Snapshot()
	last := msgs[len(msgs)-2]
	if last.Role != provider.RoleUser || last.Content != "edited prompt" {
		t.Fatalf("last user message = %+v, want edited prompt", last)
	}
	if !last.Edited || last.Original != "second prompt" {
		t.Fatalf("edit metadata = edited:%v original:%q, want edited:true original:%q", last.Edited, last.Original, "second prompt")
	}
	for _, m := range ag.Session().Snapshot() {
		if m.Role == provider.RoleUser && m.Content == "second prompt" {
			t.Fatalf("original prompt stayed as an active model turn: %+v", ag.Session().Snapshot())
		}
	}
}
