package control

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/event"
)

type PendingPromptSnapshot struct {
	ID        string              `json:"id"`
	Kind      string              `json:"kind"`
	Tool      string              `json:"tool,omitempty"`
	Subject   string              `json:"subject,omitempty"`
	FileDiff  event.FileDiff      `json:"file_diff,omitempty"`
	Plan      *event.ApprovalPlan `json:"plan,omitempty"`
	Questions []string            `json:"questions,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
	SessionID string              `json:"session_id"`
}

func pendingSnapshotPath() string {
	return filepath.Join(config.ReamesAgentHomeDir(), "pending_prompts.json")
}

func (am *approvalManager) writePendingSnapshot(sessionID string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.writePendingSnapshotLocked(sessionID)
}

// writePendingSnapshotLocked persists the current prompt snapshot. The caller
// must hold am.mu.
func (am *approvalManager) writePendingSnapshotLocked(sessionID string) {
	var snaps []PendingPromptSnapshot
	now := time.Now()
	for id, a := range am.approvals {
		snaps = append(snaps, PendingPromptSnapshot{ID: id, Kind: "approval", Tool: a.tool, Subject: a.subject, FileDiff: a.fileDiff, Plan: a.plan, CreatedAt: now, SessionID: sessionID})
	}
	for id, a := range am.asks {
		qs := make([]string, len(a.questions))
		for i, q := range a.questions {
			qs[i] = q.Prompt
		}
		snaps = append(snaps, PendingPromptSnapshot{ID: id, Kind: "ask", Questions: qs, CreatedAt: now, SessionID: sessionID})
	}
	path := pendingSnapshotPath()
	if len(snaps) == 0 {
		os.Remove(path)
		return
	}
	data, _ := json.MarshalIndent(snaps, "", "  ")
	tmp := path + ".tmp"
	os.WriteFile(tmp, data, 0600)
	os.Rename(tmp, path)
}

func (am *approvalManager) clearPendingSnapshot() {
	os.Remove(pendingSnapshotPath())
}

func LoadPendingSnapshots() ([]PendingPromptSnapshot, error) {
	path := pendingSnapshotPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snaps []PendingPromptSnapshot
	if err := json.Unmarshal(data, &snaps); err != nil {
		return nil, fmt.Errorf("corrupt pending snapshot at %s: %w", path, err)
	}
	return snaps, nil
}
