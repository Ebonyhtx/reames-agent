package repair

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStartupTrackerCrashLoopAndHealthyReset(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	tracker := NewStartupTracker(filepath.Join(t.TempDir(), "startup.json"))
	tracker.now = func() time.Time { return now }
	tracker.processAlive = func(int) bool { return false }
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := tracker.Begin("v1", false); err != nil {
			t.Fatal(err)
		}
		if err := tracker.MarkFailed(errors.New("boot failed")); err != nil {
			t.Fatal(err)
		}
		if attempt < 2 && tracker.SafeModeRecommended() {
			t.Fatalf("Safe Mode recommended after only %d incomplete startups", attempt+1)
		}
		now = now.Add(time.Minute)
	}
	if !tracker.SafeModeRecommended() {
		t.Fatal("three incomplete startups in five-minute crash window did not recommend Safe Mode")
	}
	if _, err := tracker.Begin("v1", true); err != nil {
		t.Fatal(err)
	}
	if err := tracker.MarkHealthy(); err != nil {
		t.Fatal(err)
	}
	state, err := tracker.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != "healthy" || state.ConsecutiveFailures != 0 || state.WindowStartedAt != "" {
		t.Fatalf("healthy state = %+v", state)
	}
}

func TestStartupTrackerPreservesLiveOwner(t *testing.T) {
	tracker := NewStartupTracker(filepath.Join(t.TempDir(), "startup.json"))
	foreign := os.Getpid() + 100
	tracker.processAlive = func(pid int) bool { return pid == foreign }
	state := StartupState{SchemaVersion: 1, Phase: "ready", PID: foreign, Version: "v1"}
	if err := tracker.write(state); err != nil {
		t.Fatal(err)
	}
	got, err := tracker.Begin("v2", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != foreign || got.Version != "v1" {
		t.Fatalf("live owner overwritten: %+v", got)
	}
	if err := tracker.MarkClean(); err != nil {
		t.Fatal(err)
	}
	got, err = tracker.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != "ready" {
		t.Fatalf("foreign state transitioned: %+v", got)
	}
}

func TestStartupTrackerSerializesColdStarts(t *testing.T) {
	tracker := NewStartupTracker(filepath.Join(t.TempDir(), "startup.json"))
	tracker.processAlive = func(int) bool { return false }
	tracker.now = func() time.Time { return time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC) }
	const launches = 8
	var wg sync.WaitGroup
	for range launches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tracker.Begin("v1", false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	state, err := tracker.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsecutiveFailures != launches {
		t.Fatalf("failures = %d, want %d", state.ConsecutiveFailures, launches)
	}
}
