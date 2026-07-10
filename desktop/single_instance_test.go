package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestSingleInstanceLockRestoresExistingInstance(t *testing.T) {
	app := NewApp()
	lock := singleInstanceLock(app)

	if lock == nil {
		t.Fatal("singleInstanceLock returned nil")
	}
	id := singleInstanceID()
	if lock.UniqueId != id {
		t.Fatalf("UniqueId = %q, want %q", lock.UniqueId, id)
	}
	if !strings.HasPrefix(lock.UniqueId, singleInstanceIDPrefix+".") {
		t.Fatalf("UniqueId = %q, want prefix %s.", lock.UniqueId, singleInstanceIDPrefix)
	}
	if lock.OnSecondInstanceLaunch == nil {
		t.Fatal("OnSecondInstanceLaunch should restore the existing window")
	}

	lock.OnSecondInstanceLaunch(options.SecondInstanceData{})
}

func TestSingleInstanceLockSkipsInDevMode(t *testing.T) {
	t.Setenv("REAMES_AGENT_DEV", "1")
	if lock := singleInstanceLock(NewApp()); lock != nil {
		t.Fatalf("singleInstanceLock returned %#v, want nil in dev mode", lock)
	}
}

func TestSingleInstanceIDDifferentHomesYieldDifferentIDs(t *testing.T) {
	// Without an isolated home, the ID depends only on executable path.
	t.Setenv("REAMES_AGENT_HOME", "")
	id1 := singleInstanceID()

	// With an isolated home, the normalized home path is mixed into the ID.
	homeA := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", homeA)
	id2 := singleInstanceID()

	if id1 == id2 {
		t.Fatalf("singleInstanceID with REAMES_AGENT_HOME=%q should differ from default: both are %q", homeA, id1)
	}

	// Different homes should yield different IDs.
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	id3 := singleInstanceID()
	if id2 == id3 {
		t.Fatalf("singleInstanceID for home-a and home-b should differ: both are %q", id2)
	}

	// Equivalent spellings of the same home should be stable.
	t.Setenv("REAMES_AGENT_HOME", homeA+string(os.PathSeparator)+".")
	id4 := singleInstanceID()
	if id2 != id4 {
		t.Fatalf("singleInstanceID for equivalent home paths should be stable: %q vs %q (%s)", id2, id4, filepath.Clean(homeA))
	}
}
