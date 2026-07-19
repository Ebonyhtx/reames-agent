// Package testenv provides process-level guards that keep tests away from the
// caller's real Reames Agent config, state, cache, and temporary directories.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const isolatedUserStateEnv = "REAMES_AGENT_TEST_ISOLATED_USER_STATE"

// TestingM is the subset of testing.M needed by RunWithIsolatedUserState.
type TestingM interface {
	Run() int
}

type savedEnvironment struct {
	value string
	set   bool
}

// IsolateUserState redirects default user-scoped Reames Agent paths to a
// disposable home and clears inherited explicit path overrides. Tests may
// still override any variable for a focused scenario after this guard is
// installed.
func IsolateUserState() (func(), error) {
	// Helper subprocesses spawned by a test binary must keep the exact parent
	// isolation root. Re-isolating here would discard focused overrides such as
	// REAMES_AGENT_HOME and make cross-process lock/transaction tests operate on
	// unrelated files. Only the parent that created the root owns its cleanup.
	if os.Getenv(isolatedUserStateEnv) == "1" {
		return func() {}, nil
	}
	originalHome, _ := os.UserHomeDir()
	home, err := os.MkdirTemp(originalHome, ".reames-agent-test-home-*")
	if err != nil && originalHome != "" {
		home, err = os.MkdirTemp("", "reames-agent-test-home-*")
	}
	if err != nil {
		return nil, fmt.Errorf("create isolated test home: %w", err)
	}
	tempDir := filepath.Join(home, "tmp")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		_ = os.RemoveAll(home)
		return nil, fmt.Errorf("create isolated test temp: %w", err)
	}

	set := map[string]string{
		isolatedUserStateEnv: "1",
		"HOME":               home,
		"USERPROFILE":        home,
		"XDG_CONFIG_HOME":    filepath.Join(home, ".config"),
		"XDG_CACHE_HOME":     filepath.Join(home, ".cache"),
		"XDG_STATE_HOME":     filepath.Join(home, ".local", "state"),
		"AppData":            filepath.Join(home, "AppData", "Roaming"),
		"LocalAppData":       filepath.Join(home, "AppData", "Local"),
		"TEMP":               tempDir,
		"TMP":                tempDir,
		"TMPDIR":             tempDir,
	}
	unset := []string{
		"REAMES_AGENT_HOME",
		"REAMES_AGENT_STATE_HOME",
		"REAMES_AGENT_CACHE_HOME",
	}

	saved := make(map[string]savedEnvironment, len(set)+len(unset))
	remember := func(key string) {
		if _, ok := saved[key]; ok {
			return
		}
		value, ok := os.LookupEnv(key)
		saved[key] = savedEnvironment{value: value, set: ok}
	}
	restore := func() {
		for key, previous := range saved {
			if previous.set {
				_ = os.Setenv(key, previous.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
	fail := func(cause error) (func(), error) {
		restore()
		_ = os.RemoveAll(home)
		return nil, cause
	}

	for key, value := range set {
		remember(key)
		if err := os.Setenv(key, value); err != nil {
			return fail(fmt.Errorf("set %s for isolated test home: %w", key, err))
		}
	}
	for _, key := range unset {
		remember(key)
		if err := os.Unsetenv(key); err != nil {
			return fail(fmt.Errorf("unset %s for isolated test home: %w", key, err))
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			restore()
			_ = os.RemoveAll(home)
		})
	}, nil
}

// RunWithIsolatedUserState runs a package test binary behind the user-state
// isolation guard and exits with the test result.
func RunWithIsolatedUserState(m TestingM) {
	cleanup, err := IsolateUserState()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}
