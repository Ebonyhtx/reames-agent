package control

import (
	"encoding/json"
	"fmt"
	"os"

	"reames-agent/internal/evidence"
	"reames-agent/internal/fileutil"
)

// GoalStateVersion is incremented when the on-disk goal state format changes
// in a backward-incompatible way. Readers check this before deserializing.
const GoalStateVersion = 1

// GoalStateV1 is the versioned, JSON-serializable goal state persisted as a
// sidecar file alongside the session.
type GoalStateV1 struct {
	Version            int                 `json:"version"`
	Goal               string              `json:"goal,omitempty"`
	Status             string              `json:"status,omitempty"`
	ResearchMode       GoalResearchMode    `json:"researchMode,omitempty"`
	AutoResearchTaskID string              `json:"autoResearchTaskID,omitempty"`
	Turns              int                 `json:"turns,omitempty"`
	Blocks             int                 `json:"blocks,omitempty"`
	Block              string              `json:"block,omitempty"`
	Strict             bool                `json:"strict"`
	Todos              []evidence.TodoItem `json:"todos,omitempty"`
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
		Todos:              append([]evidence.TodoItem(nil), gs.Todos...),
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
		Todos:              append([]evidence.TodoItem(nil), v.Todos...),
	}
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
	if err := fileutil.AtomicWriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("goal state: write: %w", err)
	}
	return nil
}

// ReadGoalStateForResume reads a goal state sidecar, accepting both v1
// (versioned) and v0 (unversioned legacy) formats for backward compatibility.
func ReadGoalStateForResume(data []byte) (goalState, error) {
	var header struct {
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return goalState{}, fmt.Errorf("goal state: unmarshal: %w", err)
	}
	if len(header.Version) > 0 {
		var version int
		if err := json.Unmarshal(header.Version, &version); err != nil {
			return goalState{}, fmt.Errorf("goal state: invalid version: %w", err)
		}
		if version < 0 || version > GoalStateVersion {
			return goalState{}, fmt.Errorf("goal state: unsupported version %d", version)
		}
		if version >= 1 {
			var v1 GoalStateV1
			if err := json.Unmarshal(data, &v1); err != nil {
				return goalState{}, fmt.Errorf("goal state: unmarshal v%d: %w", version, err)
			}
			return v1.ToGoalState(), nil
		}
	}
	// Fall back to v0 (raw goalState without a positive version field).
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
	if state.Version < 0 || state.Version > GoalStateVersion {
		return GoalStateV1{}, fmt.Errorf("goal state: unsupported version %d (max %d)", state.Version, GoalStateVersion)
	}
	return state, nil
}
