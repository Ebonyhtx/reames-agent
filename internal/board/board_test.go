package board

import (
	"encoding/json"
	"testing"
)

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
