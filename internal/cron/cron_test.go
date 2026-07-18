package cron

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestOpenToleratesUTF8BOMAndNextSaveHealsStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cron.json")
	payload := []byte(`{"bom-job":{"id":"bom-job","name":"from Windows editor","prompt":"hello","enabled":true,"schedule":{"kind":"interval","minutes":60}}}`)
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, payload...), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open BOM-prefixed cron store: %v", err)
	}
	jobs := store.List()
	if len(jobs) != 1 || jobs[0].ID != "bom-job" {
		t.Fatalf("BOM-prefixed cron jobs = %+v", jobs)
	}
	if err := store.Add(Job{ID: "plain-job", Name: "plain", Enabled: true, Schedule: Schedule{Kind: KindInterval, Minutes: 30}}); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(written, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("successful cron save should heal the UTF-8 BOM")
	}
}

func TestConcurrentAddRemove(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			id := fmt.Sprintf("job-%d", n)
			s.Add(Job{ID: id, Name: id, Prompt: "test", Enabled: true,
				Schedule: Schedule{Kind: KindInterval, Minutes: 60}})
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	jobs := s.List()
	if len(jobs) != 10 {
		t.Fatalf("concurrent add: got %d jobs", len(jobs))
	}

	for i := 0; i < 5; i++ {
		go func(n int) {
			s.Remove(fmt.Sprintf("job-%d", n))
			done <- true
		}(i)
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	if len(s.List()) != 5 {
		t.Fatalf("after remove: got %d jobs", len(s.List()))
	}
}

func TestMarkRun(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.Add(Job{ID: "mark-test", Name: "test", Prompt: "test", Enabled: true,
		Schedule: Schedule{Kind: KindInterval, Minutes: 60}})

	if err := s.MarkRun("mark-test", "ok"); err != nil {
		t.Fatal(err)
	}
	jobs := s.List()
	if jobs[0].RunCount != 1 || jobs[0].LastStatus != "ok" {
		t.Fatalf("mark not persisted: count=%d status=%s", jobs[0].RunCount, jobs[0].LastStatus)
	}
}

func TestParseScheduleErrors(t *testing.T) {
	_, err := ParseSchedule("")
	if err == nil {
		t.Fatal("expected error for empty")
	}
	_, err = ParseSchedule("invalid")
	if err == nil {
		t.Fatal("expected error for invalid")
	}
}

func TestDueJobs(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	s.Add(Job{ID: "past", Name: "past", Prompt: "test", Enabled: true,
		Schedule: Schedule{Kind: KindInterval, Minutes: 1}})
	// Force next run to be in the past
	s.mu.Lock()
	s.jobs["past"].NextRunAt = time.Now().Add(-1 * time.Minute)
	s.mu.Unlock()

	due := s.Due()
	if len(due) != 1 {
		t.Fatalf("expected 1 due job, got %d", len(due))
	}

	s.Add(Job{ID: "disabled", Name: "off", Prompt: "test", Enabled: false,
		Schedule: Schedule{Kind: KindInterval, Minutes: 1}})
	s.mu.Lock()
	s.jobs["disabled"].NextRunAt = time.Now().Add(-1 * time.Minute)
	s.mu.Unlock()

	if len(s.Due()) != 1 {
		t.Fatal("disabled job should not be due")
	}
}
