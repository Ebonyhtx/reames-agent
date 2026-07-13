package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"reames-agent/internal/evidence"
	"reames-agent/internal/fileutil"
)

// GoalStateVersion is the newest session runtime sidecar schema.
const GoalStateVersion = 2

// goalStateFileLocks serialize revision inspection with sidecar replacement per
// path across Controller instances in this process. Cross-process ownership is
// provided by the session lease held by each transport runtime.
var goalStateFileLocks sync.Map

func lockGoalStateFile(path string) func() {
	key := filepath.Clean(path)
	if abs, err := filepath.Abs(key); err == nil {
		key = abs
	}
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	v, _ := goalStateFileLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// goalStateV1 is retained only for reading the previous public sidecar shape.
// Running v1 Todo snapshots are not authoritative because that implementation
// did not persist every continuation transition.
type goalStateV1 struct {
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

// GoalStateV2 persists the complete recoverable Goal/Plan/Todo projection.
// Evidence receipts remain transcript-derived/current-turn state and are not
// represented as durable proof by this sidecar.
type GoalStateV2 struct {
	Version            int                 `json:"version"`
	Goal               string              `json:"goal,omitempty"`
	Status             string              `json:"status,omitempty"`
	ResearchMode       GoalResearchMode    `json:"researchMode,omitempty"`
	AutoResearchTaskID string              `json:"autoResearchTaskID,omitempty"`
	Turns              int                 `json:"turns,omitempty"`
	Blocks             int                 `json:"blocks,omitempty"`
	Block              string              `json:"block,omitempty"`
	Strict             bool                `json:"strict"`
	Intercepts         int                 `json:"intercepts,omitempty"`
	InterceptMsg       string              `json:"interceptMsg,omitempty"`
	SelfCheckPending   bool                `json:"selfCheckPending,omitempty"`
	IdleTurns          int                 `json:"idleTurns,omitempty"`
	PlanMode           bool                `json:"planMode,omitempty"`
	TodosKnown         bool                `json:"todosKnown"`
	Todos              []evidence.TodoItem `json:"todos,omitempty"`
	MessageCount       int                 `json:"messageCount,omitempty"`
	TranscriptDigest   string              `json:"transcriptDigest,omitempty"`
	Revision           uint64              `json:"revision,omitempty"`
}

func FromGoalState(gs goalState) GoalStateV2 {
	return GoalStateV2{
		Version:            GoalStateVersion,
		Goal:               gs.Goal,
		Status:             gs.Status,
		ResearchMode:       gs.ResearchMode,
		AutoResearchTaskID: gs.AutoResearchTaskID,
		Turns:              gs.Turns,
		Blocks:             gs.Blocks,
		Block:              gs.Block,
		Strict:             gs.Strict,
		Intercepts:         gs.Intercepts,
		InterceptMsg:       gs.InterceptMsg,
		SelfCheckPending:   gs.SelfCheckPending,
		IdleTurns:          gs.IdleTurns,
		PlanMode:           gs.PlanMode,
		TodosKnown:         gs.TodosKnown,
		Todos:              append([]evidence.TodoItem(nil), gs.Todos...),
		MessageCount:       gs.MessageCount,
		TranscriptDigest:   gs.TranscriptDigest,
		Revision:           gs.Revision,
	}
}

func (v GoalStateV2) ToGoalState() goalState {
	return goalState{
		Goal:               v.Goal,
		Status:             v.Status,
		ResearchMode:       v.ResearchMode,
		AutoResearchTaskID: v.AutoResearchTaskID,
		Turns:              v.Turns,
		Blocks:             v.Blocks,
		Block:              v.Block,
		Strict:             v.Strict,
		Intercepts:         v.Intercepts,
		InterceptMsg:       v.InterceptMsg,
		SelfCheckPending:   v.SelfCheckPending,
		IdleTurns:          v.IdleTurns,
		PlanMode:           v.PlanMode,
		TodosKnown:         v.TodosKnown,
		Todos:              append([]evidence.TodoItem(nil), v.Todos...),
		MessageCount:       v.MessageCount,
		TranscriptDigest:   v.TranscriptDigest,
		Revision:           v.Revision,
	}
}

func (v goalStateV1) toGoalState() goalState {
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

func WriteGoalStateV2(path string, state GoalStateV2) error {
	if path == "" {
		return nil
	}
	unlock := lockGoalStateFile(path)
	defer unlock()
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

// ReadGoalStateForResume accepts v2 plus v1/v0 legacy sidecars. Future or
// malformed versions fail closed instead of partially reviving state.
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
		switch version {
		case 1:
			var v1 goalStateV1
			if err := json.Unmarshal(data, &v1); err != nil {
				return goalState{}, fmt.Errorf("goal state: unmarshal v1: %w", err)
			}
			return v1.toGoalState(), nil
		case GoalStateVersion:
			var v2 GoalStateV2
			if err := json.Unmarshal(data, &v2); err != nil {
				return goalState{}, fmt.Errorf("goal state: unmarshal v2: %w", err)
			}
			return v2.ToGoalState(), nil
		default:
			return goalState{}, fmt.Errorf("goal state: unsupported version %d", version)
		}
	}
	var legacy goalState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return goalState{}, fmt.Errorf("goal state: unmarshal: %w", err)
	}
	return legacy, nil
}

func ReadGoalStateV2(path string) (GoalStateV2, error) {
	if path == "" {
		return GoalStateV2{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GoalStateV2{}, nil
		}
		return GoalStateV2{}, fmt.Errorf("goal state: read: %w", err)
	}
	state, err := ReadGoalStateForResume(data)
	if err != nil {
		return GoalStateV2{}, err
	}
	return FromGoalState(state), nil
}
