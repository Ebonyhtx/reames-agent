package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GoalStateVersion is incremented when the on-disk goal state format changes
// in a backward-incompatible way. Readers check this before deserializing.
const GoalStateVersion = 1

// GoalStateV1 is the versioned, JSON-serializable goal state persisted as a
// sidecar file alongside the session.
type GoalStateV1 struct {
	Version            int              `json:"version"`
	Goal               string           `json:"goal,omitempty"`
	Status             string           `json:"status,omitempty"`
	ResearchMode       GoalResearchMode `json:"researchMode,omitempty"`
	AutoResearchTaskID string           `json:"autoResearchTaskID,omitempty"`
	Turns              int              `json:"turns,omitempty"`
	Blocks             int              `json:"blocks,omitempty"`
	Block              string           `json:"block,omitempty"`
	Strict             bool             `json:"strict"`
}

// FromGoalState converts the internal goalState to GoalStateV1.
func FromGoalState(gs goalState) GoalStateV1 {
	return GoalStateV1{
		Version:            GoalStateVersion,
		Goal:               gs.Goal,
		Status:             gs.Status,
		ResearchMode:       gs.ResearchMode,
		AutoResearchTaskID: gs.AutoResearchTaskID,
		Turns:              gs.Turns,
		Blocks:             gs.Blocks,
		Block:              gs.Block,
		Strict:             gs.Strict,
	}
}

// ToGoalState converts back to the internal representation.
func (v GoalStateV1) ToGoalState() goalState {
	return goalState{
		Goal:               v.Goal,
		Status:             v.Status,
		ResearchMode:       v.ResearchMode,
		AutoResearchTaskID: v.AutoResearchTaskID,
		Turns:              v.Turns,
		Blocks:             v.Blocks,
		Block:              v.Block,
		Strict:             v.Strict,
	}
}

// GoalTransition describes an allowed state change.
type GoalTransition struct {
	From   string
	To     string
	Reason string
}

// AllowedGoalTransitions is the exhaustive table of permitted goal state
// transitions. Every status change in the codebase must appear here.
var AllowedGoalTransitions = []GoalTransition{
	{From: "", To: GoalStatusRunning, Reason: "SetGoal initialises a new goal"},
	{From: GoalStatusStopped, To: GoalStatusRunning, Reason: "restarting a stopped goal"},
	{From: GoalStatusBlocked, To: GoalStatusRunning, Reason: "resuming a blocked goal"},
	{From: GoalStatusRunning, To: GoalStatusComplete, Reason: "model emitted [goal:complete]"},
	{From: GoalStatusRunning, To: GoalStatusBlocked, Reason: "model emitted [goal:blocked]"},
	{From: GoalStatusRunning, To: GoalStatusStopped, Reason: "user called Stop/Cancel"},
	{From: GoalStatusBlocked, To: GoalStatusStopped, Reason: "user stopped a blocked goal"},
	{From: GoalStatusComplete, To: "", Reason: "ClearGoal"},
	{From: GoalStatusBlocked, To: "", Reason: "ClearGoal"},
	{From: GoalStatusStopped, To: "", Reason: "ClearGoal"},
}

// IsTerminalGoalStatus reports whether the status is terminal.
func IsTerminalGoalStatus(status string) bool {
	switch status {
	case GoalStatusComplete, GoalStatusBlocked, GoalStatusStopped:
		return true
	default:
		return false
	}
}

// ValidateGoalTransition checks whether a transition is allowed.
func ValidateGoalTransition(from, to string) error {
	for _, t := range AllowedGoalTransitions {
		if t.From == from && t.To == to {
			return nil
		}
	}
	return fmt.Errorf("goal transition %q → %q is not allowed", from, to)
}

// WriteGoalStateV1 persists a GoalStateV1 to disk atomically.
func WriteGoalStateV1(path string, state GoalStateV1) error {
	if path == "" {
		return nil
	}
	state.Version = GoalStateVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("goal state: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("goal state: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("goal state: write tmp: %w", err)
	}
	return os.Rename(tmp, path)
}

// ReadGoalStateForResume reads a goal state sidecar, accepting both v1
// (versioned) and v0 (unversioned legacy) formats for backward compatibility.
func ReadGoalStateForResume(data []byte) (goalState, error) {
	// Try v1 first.
	var v1 GoalStateV1
	if err := json.Unmarshal(data, &v1); err == nil && v1.Version >= 1 {
		if v1.Version > GoalStateVersion {
			return goalState{}, fmt.Errorf("goal state: unsupported version %d", v1.Version)
		}
		return v1.ToGoalState(), nil
	}
	// Fall back to v0 (raw goalState without version field).
	var gs goalState
	if err := json.Unmarshal(data, &gs); err != nil {
		return goalState{}, fmt.Errorf("goal state: unmarshal: %w", err)
	}
	return gs, nil
}
func ReadGoalStateV1(path string) (GoalStateV1, error) {
	if path == "" {
		return GoalStateV1{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GoalStateV1{}, nil
		}
		return GoalStateV1{}, fmt.Errorf("goal state: read: %w", err)
	}
	var state GoalStateV1
	if err := json.Unmarshal(data, &state); err != nil {
		return GoalStateV1{}, fmt.Errorf("goal state: unmarshal: %w", err)
	}
	if state.Version > GoalStateVersion {
		return GoalStateV1{}, fmt.Errorf("goal state: unsupported version %d (max %d)", state.Version, GoalStateVersion)
	}
	return state, nil
}
