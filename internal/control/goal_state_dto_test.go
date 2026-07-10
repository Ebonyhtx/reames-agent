package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reames-agent/internal/evidence"
)

func TestGoalStateV1RoundTrip(t *testing.T) {
	original := GoalStateV1{
		Version:            GoalStateVersion,
		Goal:               "fix the build",
		Status:             GoalStatusRunning,
		ResearchMode:       GoalResearchOn,
		AutoResearchTaskID: "task-1",
		Turns:              5,
		Blocks:             0,
		Strict:             true,
		Todos:              []evidence.TodoItem{{Content: "verify persistence", Status: "completed"}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var restored GoalStateV1
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.Version != GoalStateVersion {
		t.Fatalf("Version = %d, want %d", restored.Version, GoalStateVersion)
	}
	if restored.Goal != "fix the build" {
		t.Fatalf("Goal = %q", restored.Goal)
	}
	if restored.Status != GoalStatusRunning {
		t.Fatalf("Status = %q", restored.Status)
	}
	if restored.Strict != true {
		t.Fatal("Strict should be true")
	}
	if len(restored.Todos) != 1 || restored.Todos[0].Content != "verify persistence" {
		t.Fatalf("Todos = %+v, want persisted todo", restored.Todos)
	}
}

func TestGoalStateV1ToGoalStateAndBack(t *testing.T) {
	v1 := GoalStateV1{
		Version:      GoalStateVersion,
		Goal:         "audit security",
		Status:       GoalStatusBlocked,
		ResearchMode: GoalResearchAuto,
		Blocks:       2,
		Block:        "missing API key",
		Strict:       false,
		Todos:        []evidence.TodoItem{{Content: "provide key", Status: "pending"}},
	}

	gs := v1.ToGoalState()
	back := FromGoalState(gs)

	if back.Goal != v1.Goal {
		t.Fatalf("Goal round-trip: %q → %q", v1.Goal, back.Goal)
	}
	if back.Status != v1.Status {
		t.Fatalf("Status round-trip: %q → %q", v1.Status, back.Status)
	}
	if back.Block != v1.Block {
		t.Fatalf("Block round-trip: %q → %q", v1.Block, back.Block)
	}
	if len(back.Todos) != 1 || back.Todos[0].Status != "pending" {
		t.Fatalf("Todos round-trip: %+v", back.Todos)
	}
}

func TestGoalStatePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal-state.json")

	// Write.
	state := GoalStateV1{
		Goal:   "test persistence",
		Status: GoalStatusRunning,
	}
	if err := WriteGoalStateV1(path, state); err != nil {
		t.Fatal(err)
	}

	// Read back.
	restored, err := ReadGoalStateV1(path)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Goal != state.Goal {
		t.Fatalf("Goal = %q", restored.Goal)
	}
	if restored.Status != state.Status {
		t.Fatalf("Status = %q", restored.Status)
	}
	if restored.Version != GoalStateVersion {
		t.Fatalf("Version = %d", restored.Version)
	}

	// Non-existent file should return zero value.
	missing, err := ReadGoalStateV1(filepath.Join(dir, "nonexistent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if missing.Goal != "" || missing.Status != "" {
		t.Fatal("nonexistent file should return zero value")
	}
}

func TestGoalStateV0BackwardCompat(t *testing.T) {
	// Version 0 (no version field) should be accepted.
	v0Data := []byte(`{"goal":"old goal","status":"running","turns":3}`)
	var state GoalStateV1
	if err := json.Unmarshal(v0Data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Goal != "old goal" {
		t.Fatalf("Goal = %q", state.Goal)
	}
	if state.Status != GoalStatusRunning {
		t.Fatalf("Status = %q", state.Status)
	}
	if state.Turns != 3 {
		t.Fatalf("Turns = %d", state.Turns)
	}
	// Version should default to 0.
	if state.Version != 0 {
		t.Fatalf("Version = %d, want 0 for v0 data", state.Version)
	}
}

func TestGoalStateV1RejectsFutureVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.json")

	// Write a state with a version newer than we support.
	if err := os.WriteFile(path, []byte(`{"version":999,"goal":"future","status":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadGoalStateV1(path)
	if err == nil {
		t.Fatal("should reject future version")
	}
}

func TestReadGoalStateForResumeRejectsFutureVersion(t *testing.T) {
	_, err := ReadGoalStateForResume([]byte(`{"version":999,"goal":"future","status":"running"}`))
	if err == nil {
		t.Fatal("resume reader should reject future version")
	}
}

func TestReadGoalStateForResumeRejectsMalformedVersion(t *testing.T) {
	for _, data := range []string{
		`{"version":"1","goal":"future","status":"running"}`,
		`{"version":-1,"goal":"future","status":"running"}`,
	} {
		if _, err := ReadGoalStateForResume([]byte(data)); err == nil {
			t.Fatalf("resume reader accepted malformed version in %s", data)
		}
	}
}

func TestGoalMachineDoesNotRestoreFutureVersion(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(goalStatePath(sessionPath), []byte(`{"version":999,"goal":"future","status":"running","todos":[{"content":"future","status":"completed"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var machine goalMachine
	machine.restoreRunningFromState(sessionPath)
	if machine.active() {
		t.Fatal("future goal-state version must not restore a running goal")
	}
	if todos, ok := machine.terminalTodosFromState(sessionPath); ok || len(todos) != 0 {
		t.Fatalf("future goal-state version restored terminal todos: %+v", todos)
	}
}

func TestReadGoalStateForResumeV1(t *testing.T) {
	v1 := []byte(`{"version":1,"goal":"resume test","status":"running","turns":5,"strict":true,"todos":[{"content":"ship","status":"completed"}]}`)
	gs, err := ReadGoalStateForResume(v1)
	if err != nil {
		t.Fatal(err)
	}
	if gs.Goal != "resume test" {
		t.Fatalf("Goal = %q", gs.Goal)
	}
	if gs.Status != GoalStatusRunning {
		t.Fatalf("Status = %q", gs.Status)
	}
	if !gs.Strict {
		t.Fatal("Strict should be true")
	}
	if len(gs.Todos) != 1 || gs.Todos[0].Status != "completed" {
		t.Fatalf("Todos = %+v, want completed todo", gs.Todos)
	}
}

func TestReadGoalStateForResumeV0Fallback(t *testing.T) {
	v0 := []byte(`{"goal":"legacy goal","status":"blocked","block":"missing key"}`)
	gs, err := ReadGoalStateForResume(v0)
	if err != nil {
		t.Fatal(err)
	}
	if gs.Goal != "legacy goal" {
		t.Fatalf("Goal = %q", gs.Goal)
	}
	if gs.Status != GoalStatusBlocked {
		t.Fatalf("Status = %q", gs.Status)
	}
}
