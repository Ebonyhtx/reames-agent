package board

import (
	"time"

	"reames-agent/internal/control"
	"reames-agent/internal/evidence"
)

type Status struct {
	Goal      GoalStatus      `json:"goal"`
	Plan      PlanStatus      `json:"plan"`
	Todos     []TodoItem      `json:"todos"`
	Evidence  EvidenceSummary `json:"evidence"`
	Session   SessionInfo     `json:"session"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type GoalStatus struct {
	Active bool   `json:"active"`
	Text   string `json:"text,omitempty"`
	Status string `json:"status,omitempty"`
}

type PlanStatus struct {
	Active bool `json:"active"`
}

type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"active_form,omitempty"`
	Level      int    `json:"level,omitempty"`
}

type EvidenceSummary struct {
	Receipts    int      `json:"receipts"`
	WriteRecent bool     `json:"write_recent"`
	Touched     []string `json:"touched,omitempty"`
	TodoReady   bool     `json:"todo_ready"`
}

type SessionInfo struct {
	Label     string `json:"label,omitempty"`
	Running   bool   `json:"running"`
	PlanMode  bool   `json:"plan_mode"`
	Pending   bool   `json:"pending_prompt"`
	CacheHit  int    `json:"cache_hit"`
	CacheMiss int    `json:"cache_miss"`
}

func Build(ctrl control.SessionAPI, l *evidence.Ledger) Status {
	rs := ctrl.RuntimeStatus()
	hit, miss := ctrl.SessionCache()
	goalText := ctrl.Goal()
	goalStatus := ctrl.GoalStatus()

	s := Status{
		Goal: GoalStatus{
			Text:   goalText,
			Status: goalStatus,
			Active: goalText != "" && goalStatus == control.GoalStatusRunning,
		},
		Plan: PlanStatus{Active: ctrl.PlanMode()},
		Session: SessionInfo{
			Label: ctrl.Label(), Running: rs.Running, PlanMode: ctrl.PlanMode(),
			Pending: rs.PendingPrompt, CacheHit: hit, CacheMiss: miss,
		},
		UpdatedAt: time.Now(),
	}

	raw := ctrl.Todos()
	s.Todos = make([]TodoItem, len(raw))
	for i, t := range raw {
		s.Todos[i] = TodoItem{Content: t.Content, Status: t.Status, ActiveForm: t.ActiveForm, Level: t.Level}
	}

	snapshot := ctrl.EvidenceSnapshot()
	if l != nil {
		snapshot = l.Snapshot(50)
	}
	s.Evidence = EvidenceSummary{
		Receipts:    snapshot.Receipts,
		WriteRecent: snapshot.WriteOrCommand,
		Touched:     safe(snapshot.Touched, 50),
		TodoReady:   len(evidence.IncompleteTodos(raw)) == 0,
	}

	return s
}

func safe(paths []string, max int) []string {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) > max {
		paths = paths[:max]
	}
	return paths
}
