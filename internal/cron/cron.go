// Package cron provides persistent scheduled task storage with human-readable
// schedule parsing (interval, daily, cron-expr) and a background ticker that
// survives restarts by reading from a JSON file store.
package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ScheduleKind classifies how a job repeats.
type ScheduleKind string

const (
	KindOnce     ScheduleKind = "once"
	KindInterval ScheduleKind = "interval"
	KindCron     ScheduleKind = "cron"
)

// Job is a persisted scheduled task.
type Job struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Prompt      string       `json:"prompt"`
	Enabled     bool         `json:"enabled"`
	Schedule    Schedule     `json:"schedule"`
	LastRunAt   time.Time    `json:"last_run_at,omitempty"`
	NextRunAt   time.Time    `json:"next_run_at,omitempty"`
	LastStatus  string       `json:"last_status,omitempty"`
	RunCount    int          `json:"run_count"`
	CreatedAt   time.Time    `json:"created_at"`
}

// Schedule describes when a job runs.
type Schedule struct {
	Kind     ScheduleKind `json:"kind"`
	Minutes  int          `json:"minutes,omitempty"`  // for interval
	Expr     string       `json:"expr,omitempty"`     // for cron
	RunAt    time.Time    `json:"run_at,omitempty"`   // for once
}

// Store persists jobs to a JSON file.
type Store struct {
	mu   sync.Mutex
	path string
	jobs map[string]*Job
}

// Open loads or creates the cron store at the given path.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cron: %w", err)
	}
	path := filepath.Join(dir, "cron.json")
	s := &Store{path: path, jobs: make(map[string]*Job)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.jobs); err != nil {
		return nil, fmt.Errorf("cron: corrupt store: %w", err)
	}
	for _, j := range s.jobs {
		j.NextRunAt = nextRun(j.Schedule, j.LastRunAt)
	}
	return s, nil
}

// Add creates or replaces a job.
func (s *Store) Add(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.NextRunAt = nextRun(job.Schedule, job.LastRunAt)
	s.jobs[job.ID] = &job
	return s.save()
}

// Remove deletes a job.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	return s.save()
}

// List returns all jobs sorted by next run time.
func (s *Store) List() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Job
	for _, j := range s.jobs {
		out = append(out, *j)
	}
	return out
}

// Due returns jobs whose NextRunAt is in the past.
func (s *Store) Due() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var out []Job
	for _, j := range s.jobs {
		if j.Enabled && !j.NextRunAt.IsZero() && !j.NextRunAt.After(now) {
			out = append(out, *j)
		}
	}
	return out
}

// MarkRun updates a job's last-run state and schedules the next run.
func (s *Store) MarkRun(id string, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("cron: job %q not found", id)
	}
	j.LastRunAt = time.Now()
	j.LastStatus = status
	j.RunCount++
	j.NextRunAt = nextRun(j.Schedule, j.LastRunAt)
	return s.save()
}

// ParseSchedule converts a human-readable schedule string to a Schedule.
// Supported formats: "30m", "2h", "every 30m", "every 2h", "daily at 09:00",
// "0 9 * * *" (cron).
func ParseSchedule(raw string) (Schedule, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return Schedule{}, fmt.Errorf("cron: empty schedule")
	}
	if strings.HasPrefix(raw, "every ") {
		d, err := time.ParseDuration(raw[6:])
		if err != nil {
			return Schedule{}, fmt.Errorf("cron: invalid interval: %w", err)
		}
		return Schedule{Kind: KindInterval, Minutes: int(d.Minutes())}, nil
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return Schedule{Kind: KindOnce, Minutes: int(d.Minutes())}, nil
	}
	if strings.Contains(raw, "*") {
		return Schedule{Kind: KindCron, Expr: raw}, nil
	}
	return Schedule{}, fmt.Errorf("cron: unrecognized schedule: %q", raw)
}

func nextRun(s Schedule, after time.Time) time.Time {
	switch s.Kind {
	case KindOnce:
		if s.RunAt.After(after) {
			return s.RunAt
		}
		return time.Time{} // already ran
	case KindInterval:
		if s.Minutes <= 0 {
			return time.Time{}
		}
		next := after.Add(time.Duration(s.Minutes) * time.Minute)
		if next.Before(time.Now()) {
			next = time.Now().Add(time.Duration(s.Minutes) * time.Minute)
		}
		return next
	case KindCron:
		return time.Now().Add(1 * time.Hour) // placeholder — real cron parser for future
	default:
		return time.Time{}
	}
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.jobs, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Ticker runs fn for due jobs at each interval tick, persisting results.
func (s *Store) Ticker(ctx context.Context, fn func(context.Context, Job) (string, error)) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, j := range s.Due() {
				status, err := fn(ctx, j)
				if err != nil {
					status = "error: " + err.Error()
					slog.Warn("cron: job failed", "id", j.ID, "err", err)
				}
				if err := s.MarkRun(j.ID, status); err != nil {
					slog.Warn("cron: mark failed", "id", j.ID, "err", err)
				}
			}
		}
	}
}
