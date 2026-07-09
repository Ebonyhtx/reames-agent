package main

import (
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
	oldHome := explicitHome
	t.Cleanup(func() { explicitHome = oldHome })

	// Without explicit home, the ID depends only on executable path.
	explicitHome = ""
	id1 := singleInstanceID()

	// With explicit home, the home path is mixed into the ID.
	explicitHome = "/tmp/home-a"
	id2 := singleInstanceID()

	if id1 == id2 {
		t.Fatalf("singleInstanceID with explicitHome=%q should differ from default: both are %q", explicitHome, id1)
	}

	// Different homes should yield different IDs.
	explicitHome = "/tmp/home-b"
	id3 := singleInstanceID()
	if id2 == id3 {
		t.Fatalf("singleInstanceID for home-a and home-b should differ: both are %q", id2)
	}

	// Same home should be stable.
	id4 := singleInstanceID()
	if id3 != id4 {
		t.Fatalf("singleInstanceID for same home should be stable: %q vs %q", id3, id4)
	}
}
