package main

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDesktopZoomFactorDefaultsAndPersists(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := &App{}

	if got := app.GetDesktopZoomFactor(); got != 1.0 {
		t.Fatalf("default zoom = %v, want 1", got)
	}
	if err := app.SetDesktopZoomFactor(1.35); err != nil {
		t.Fatal(err)
	}
	if got := app.GetDesktopZoomFactor(); got != 1.35 {
		t.Fatalf("persisted zoom = %v, want 1.35", got)
	}
	info, err := os.Stat(zoomFactorPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm&0o077 != 0 {
		t.Fatalf("zoom preference permissions = %o, want no group/world access", perm)
	}
}

func TestDesktopZoomFactorClampsAndRejectsNonFiniteValues(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := &App{}

	for _, tc := range []struct {
		input float64
		want  float64
	}{
		{input: -1, want: 0.5},
		{input: 3, want: 2.0},
	} {
		if err := app.SetDesktopZoomFactor(tc.input); err != nil {
			t.Fatal(err)
		}
		if got := app.GetDesktopZoomFactor(); got != tc.want {
			t.Fatalf("SetDesktopZoomFactor(%v) persisted %v, want %v", tc.input, got, tc.want)
		}
	}

	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := app.SetDesktopZoomFactor(invalid); err == nil {
			t.Fatalf("non-finite zoom %v should be rejected", invalid)
		}
	}
	if got := app.GetDesktopZoomFactor(); got != 2.0 {
		t.Fatalf("rejected value changed persisted zoom to %v", got)
	}
}

func TestDesktopZoomFactorIgnoresCorruptPreference(t *testing.T) {
	isolateDesktopUserDirs(t)
	if err := os.MkdirAll(filepath.Dir(zoomFactorPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zoomFactorPath(), []byte(`{"zoomFactor":9}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (&App{}).GetDesktopZoomFactor(); got != 1.0 {
		t.Fatalf("out-of-range preference returned %v, want default 1", got)
	}
}
