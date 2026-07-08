package cron

import (
	"testing"
)

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		raw     string
		want    ScheduleKind
		wantMin int
	}{
		{"30m", KindOnce, 30},
		{"2h", KindOnce, 120},
		{"every 30m", KindInterval, 30},
		{"every 2h", KindInterval, 120},
		{"0 9 * * *", KindCron, 0},
	}
	for _, tt := range tests {
		s, err := ParseSchedule(tt.raw)
		if err != nil {
			t.Errorf("ParseSchedule(%q): %v", tt.raw, err)
			continue
		}
		if s.Kind != tt.want {
			t.Errorf("ParseSchedule(%q).Kind = %q, want %q", tt.raw, s.Kind, tt.want)
		}
		if s.Minutes != tt.wantMin {
			t.Errorf("ParseSchedule(%q).Minutes = %d, want %d", tt.raw, s.Minutes, tt.wantMin)
		}
	}
}

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	job := Job{
		ID:      "test-1",
		Name:    "test job",
		Prompt:  "echo hello",
		Enabled: true,
		Schedule: Schedule{
			Kind:    KindInterval,
			Minutes: 30,
		},
	}
	if err := s.Add(job); err != nil {
		t.Fatal(err)
	}

	jobs := s.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	if err := s.Remove("test-1"); err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Fatal("expected empty after remove")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	job := Job{ID: "persist-1", Name: "persist", Prompt: "test", Enabled: true,
		Schedule: Schedule{Kind: KindInterval, Minutes: 60}}
	s.Add(job)

	// Reopen
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	jobs := s2.List()
	if len(jobs) != 1 || jobs[0].ID != "persist-1" {
		t.Fatalf("persistence failed: %+v", jobs)
	}
}
