package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
}

func TestAllowedGoalTransitions(t *testing.T) {
	// Every allowed transition should validate.
	for _, tr := range AllowedGoalTransitions {
		t.Run(tr.From+"→"+tr.To, func(t *testing.T) {
			if err := ValidateGoalTransition(tr.From, tr.To); err != nil {
				t.Fatalf("allowed transition %q→%q should pass: %v", tr.From, tr.To, err)
			}
		})
	}

	// Disallowed transitions should fail.
	disallowed := []struct{ from, to string }{
		{GoalStatusComplete, GoalStatusRunning}, // terminal → running should not happen
		{GoalStatusRunning, GoalStatusRunning},  // no-op
		{"", GoalStatusComplete},                // can't go from empty to complete directly
		{GoalStatusComplete, GoalStatusBlocked}, // terminal → blocked
	}
	for _, d := range disallowed {
		t.Run("deny-"+d.from+"→"+d.to, func(t *testing.T) {
			if err := ValidateGoalTransition(d.from, d.to); err == nil {
				t.Fatalf("disallowed transition %q→%q should fail", d.from, d.to)
			}
		})
	}
}

func TestIsTerminalGoalStatus(t *testing.T) {
	for _, s := range []string{GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped} {
		if !IsTerminalGoalStatus(s) {
			t.Fatalf("%q should be terminal", s)
		}
	}
	if IsTerminalGoalStatus(GoalStatusRunning) {
		t.Fatal("running should not be terminal")
	}
	if IsTerminalGoalStatus("") {
		t.Fatal("empty should not be terminal")
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
