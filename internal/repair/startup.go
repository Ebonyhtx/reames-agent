package repair

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

const (
	startupStateVersion = 1
	defaultCrashWindow  = 5 * time.Minute
	defaultFailureLimit = 3
	// StartupHealthDelay keeps a new build probationary after the UI first paints.
	StartupHealthDelay = 30 * time.Second
)

// StartupState is the durable, process-owned desktop startup ledger.
type StartupState struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Phase               string `json:"phase"`
	Version             string `json:"version,omitempty"`
	PID                 int    `json:"pid,omitempty"`
	SafeMode            bool   `json:"safeMode,omitempty"`
	ConsecutiveFailures int    `json:"consecutiveFailures,omitempty"`
	WindowStartedAt     string `json:"windowStartedAt,omitempty"`
	StartedAt           string `json:"startedAt,omitempty"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
	Error               string `json:"error,omitempty"`
}

// StartupTracker serializes startup state across Guard and Desktop processes.
type StartupTracker struct {
	path         string
	now          func() time.Time
	processAlive func(int) bool
}

func NewStartupTracker(path string) *StartupTracker {
	if path == "" {
		if root := config.MemoryUserDir(); root != "" {
			path = filepath.Join(root, "repair", "startup-state.json")
		}
	}
	return &StartupTracker{path: path, now: time.Now, processAlive: startupProcessAlive}
}

func (t *StartupTracker) Path() string { return t.path }

func (t *StartupTracker) Read() (StartupState, error) {
	if t.path == "" {
		return StartupState{}, nil
	}
	b, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return StartupState{}, nil
		}
		return StartupState{}, err
	}
	var state StartupState
	if err := json.Unmarshal(b, &state); err != nil {
		return StartupState{}, err
	}
	if state.SchemaVersion != 0 && state.SchemaVersion != startupStateVersion {
		return StartupState{}, errors.New("unsupported startup-state schema")
	}
	return state, nil
}

// SafeModeRecommended reports strong crash-loop evidence without mutating state.
func (t *StartupTracker) SafeModeRecommended() bool {
	state, err := t.Read()
	if err != nil || !incompleteStartupPhase(state.Phase) {
		return false
	}
	if runningStartupPhase(state.Phase) && state.PID > 0 && t.processAlive(state.PID) {
		return false
	}
	started, err := time.Parse(time.RFC3339Nano, state.WindowStartedAt)
	now := t.now()
	if err != nil || now.Before(started) || now.Sub(started) > defaultCrashWindow {
		return false
	}
	return state.ConsecutiveFailures >= defaultFailureLimit
}

func (t *StartupTracker) lock() (func(), error) {
	if t.path == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o700); err != nil {
		return nil, err
	}
	return lockRepairStateFile(t.path)
}

// Begin atomically records this startup attempt. A live owner is never replaced.
func (t *StartupTracker) Begin(version string, safeMode bool) (StartupState, error) {
	unlock, err := t.lock()
	if err != nil {
		return StartupState{}, err
	}
	defer unlock()

	now := t.now().UTC()
	previous, err := t.Read()
	if err != nil {
		return StartupState{}, err
	}
	if runningStartupPhase(previous.Phase) && previous.PID > 0 && t.processAlive(previous.PID) {
		return previous, nil
	}
	failures := 0
	windowStart := now
	if incompleteStartupPhase(previous.Phase) {
		failures = nextFailureCount(previous, now, defaultCrashWindow) - 1
		if parsed, parseErr := time.Parse(time.RFC3339Nano, previous.WindowStartedAt); parseErr == nil && !now.Before(parsed) && now.Sub(parsed) <= defaultCrashWindow {
			windowStart = parsed
		}
	}
	state := StartupState{
		SchemaVersion:       startupStateVersion,
		Phase:               "starting",
		Version:             version,
		PID:                 os.Getpid(),
		SafeMode:            safeMode,
		ConsecutiveFailures: failures + 1,
		WindowStartedAt:     windowStart.Format(time.RFC3339Nano),
		StartedAt:           now.Format(time.RFC3339Nano),
		UpdatedAt:           now.Format(time.RFC3339Nano),
	}
	return state, t.write(state)
}

func incompleteStartupPhase(phase string) bool {
	return phase == "starting" || phase == "ready" || phase == "failed"
}

func runningStartupPhase(phase string) bool {
	return phase == "starting" || phase == "ready" || phase == "healthy"
}

func nextFailureCount(state StartupState, now time.Time, window time.Duration) int {
	started, err := time.Parse(time.RFC3339Nano, state.WindowStartedAt)
	if err != nil || now.Before(started) || now.Sub(started) > window {
		return 1
	}
	return state.ConsecutiveFailures + 1
}

func (t *StartupTracker) MarkReady() error   { return t.transition("ready", "") }
func (t *StartupTracker) MarkHealthy() error { return t.transition("healthy", "") }
func (t *StartupTracker) MarkClean() error   { return t.transition("clean-exit", "") }

func (t *StartupTracker) MarkFailed(err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return t.transition("failed", message)
}

func (t *StartupTracker) transition(phase, message string) error {
	unlock, err := t.lock()
	if err != nil {
		return err
	}
	defer unlock()
	state, err := t.Read()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if state.PID > 0 && state.PID != os.Getpid() && t.processAlive(state.PID) {
		return nil
	}
	state.SchemaVersion = startupStateVersion
	state.Phase = phase
	state.Error = message
	state.UpdatedAt = t.now().UTC().Format(time.RFC3339Nano)
	if phase == "healthy" || phase == "clean-exit" {
		state.ConsecutiveFailures = 0
		state.WindowStartedAt = ""
	}
	return t.write(state)
}

func (t *StartupTracker) write(state StartupState) error {
	if t.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(t.path, append(b, '\n'), 0o600)
}
