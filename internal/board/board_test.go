package board

import (
	"encoding/json"
	"testing"

	"reames-agent/internal/control"
	"reames-agent/internal/evidence"
)

type boardTestController struct {
	control.SessionAPI
	goal       string
	goalStatus string
	todos      []evidence.TodoItem
	evidence   evidence.Snapshot
}

func (c boardTestController) RuntimeStatus() control.RuntimeStatus { return control.RuntimeStatus{} }
func (c boardTestController) SessionCache() (int, int)             { return 0, 0 }
func (c boardTestController) Goal() string                         { return c.goal }
func (c boardTestController) GoalStatus() string                   { return c.goalStatus }
func (c boardTestController) PlanMode() bool                       { return false }
func (c boardTestController) Label() string                        { return "test" }
func (c boardTestController) Todos() []evidence.TodoItem           { return c.todos }
func (c boardTestController) EvidenceSnapshot() evidence.Snapshot  { return c.evidence }

func TestStatusJSON(t *testing.T) {
	s := Status{
		Goal: GoalStatus{Active: false},
		Plan: PlanStatus{Active: true},
		Session: SessionInfo{
			Running:  false,
			PlanMode: true,
			CacheHit: 100,
		},
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var out Status
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !out.Plan.Active {
		t.Fatal("plan not active after round-trip")
	}
	if out.Session.CacheHit != 100 {
		t.Fatalf("cache hit mismatch: %d", out.Session.CacheHit)
	}
}

func TestStatusEmpty(t *testing.T) {
	s := Status{}
	data, _ := json.Marshal(s)
	if string(data) == "" {
		t.Fatal("empty json")
	}
}

func TestSafePaths(t *testing.T) {
	paths := []string{"a", "b", "c"}
	r := safe(paths, 2)
	if len(r) != 2 {
		t.Fatalf("expected 2, got %d", len(r))
	}
	if r[0] != "a" || r[1] != "b" {
		t.Fatal("wrong truncation")
	}
}

func TestBuildUsesRunningStatusAndControllerEvidence(t *testing.T) {
	ctrl := boardTestController{
		goal:       "waiting for approval",
		goalStatus: control.GoalStatusBlocked,
		todos:      []evidence.TodoItem{{Content: "approve", Status: "in_progress"}},
		evidence: evidence.Snapshot{
			Receipts: 3, WriteOrCommand: true, Touched: []string{"a.go"},
		},
	}
	status := Build(ctrl, nil)
	if status.Goal.Active {
		t.Fatal("blocked goal with retained text must not be projected as active")
	}
	if status.Evidence.Receipts != 3 || !status.Evidence.WriteRecent || len(status.Evidence.Touched) != 1 {
		t.Fatalf("evidence projection = %+v", status.Evidence)
	}
	if status.Evidence.TodoReady {
		t.Fatal("incomplete canonical todo must keep TodoReady false")
	}

	ctrl.goalStatus = control.GoalStatusRunning
	ctrl.todos[0].Status = "completed"
	status = Build(ctrl, nil)
	if !status.Goal.Active || !status.Evidence.TodoReady {
		t.Fatalf("running/completed projection = %+v", status)
	}
}
