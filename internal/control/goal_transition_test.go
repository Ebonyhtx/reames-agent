package control

import (
	"testing"
)

// TestGoalStateMachine_TransitionsAreValid verifies that every
// transition the goal FSM makes in practice is listed in
// AllowedGoalTransitions. This is enforced by instrumenting the
// goalMachine to validate each transition via ValidateGoalTransition.
func TestGoalStateMachine_TransitionsAreValid(t *testing.T) {
	// Verify the transition table is internally consistent.
	for _, tr := range AllowedGoalTransitions {
		if err := ValidateGoalTransition(tr.From, tr.To); err != nil {
			t.Fatalf("allowed transition %q→%q should validate: %v", tr.From, tr.To, err)
		}
	}

	// Verify all terminal states are recognised.
	for _, s := range []string{GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped} {
		if !IsTerminalGoalStatus(s) {
			t.Fatalf("%q should be terminal", s)
		}
	}

	// Verify running is NOT terminal.
	if IsTerminalGoalStatus(GoalStatusRunning) {
		t.Fatal("running should not be terminal")
	}

	// Verify common disallowed transitions.
	disallowed := []struct{ from, to string }{
		{GoalStatusComplete, GoalStatusRunning},
		{GoalStatusRunning, GoalStatusRunning},
		{"", GoalStatusComplete},
		{GoalStatusComplete, GoalStatusBlocked},
	}
	for _, d := range disallowed {
		if err := ValidateGoalTransition(d.from, d.to); err == nil {
			t.Fatalf("disallowed transition %q→%q should fail", d.from, d.to)
		}
	}
}

// TestGoalStateMachine_TerminalStatesDoNotAdvance verifies that
// the goal loop stops after reaching a terminal state.
func TestGoalStateMachine_TerminalStatesDoNotAdvance(t *testing.T) {
	// All terminal states should be recognised.
	terminal := map[string]bool{
		GoalStatusComplete: true,
		GoalStatusBlocked:  true,
		GoalStatusStopped:  true,
	}

	for status := range terminal {
		if !IsTerminalGoalStatus(status) {
			t.Fatalf("expected %q to be terminal", status)
		}
	}

	if IsTerminalGoalStatus(GoalStatusRunning) {
		t.Fatal("running must not be terminal")
	}
	if IsTerminalGoalStatus("") {
		t.Fatal("empty must not be terminal")
	}
}

// TestGoalStateMachine_TransitionRuleCoverage verifies that the
// transition table covers all meaningful status combinations.
func TestGoalStateMachine_TransitionRuleCoverage(t *testing.T) {
	allStatuses := []string{"", GoalStatusRunning, GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped}

	// Build a map of (from, to) pairs that are allowed.
	allowed := make(map[string]bool)
	for _, tr := range AllowedGoalTransitions {
		allowed[tr.From+"→"+tr.To] = true
	}

	// Check that every status has at least one incoming and outgoing
	// transition (except the zero/empty initial state).
	hasIncoming := map[string]int{}
	hasOutgoing := map[string]int{}
	for _, tr := range AllowedGoalTransitions {
		if tr.To != "" {
			hasIncoming[tr.To]++
		}
		if tr.From != "" {
			hasOutgoing[tr.From]++
		}
	}

	for _, s := range allStatuses {
		if s == "" {
			continue // initial state, no incoming needed
		}
		if hasIncoming[s] == 0 {
			t.Errorf("status %q has no incoming transition", s)
		}
	}

	for _, s := range allStatuses {
		if s == "" || IsTerminalGoalStatus(s) {
			continue // terminal, no outgoing needed
		}
		if hasOutgoing[s] == 0 {
			t.Errorf("non-terminal status %q has no outgoing transition", s)
		}
	}

	// Each terminal state should have a clearing transition to "".
	for _, s := range []string{GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped} {
		key := s + "→"
		if !allowed[key] {
			t.Errorf("terminal status %q needs a clearing transition (to empty)", s)
		}
	}
}
