//go:build windows

package main

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestInstallerFailureRecordsMarkerAndRelaunchesGuard(t *testing.T) {
	var logs bytes.Buffer
	var markedVersion, markedReason string
	var relaunchedPath, relaunchedDir string
	deps := helperDependencies{
		logger:      log.New(&logs, "", 0),
		waitForExit: func(uint32, time.Duration) error { return nil },
		setenv:      func(string, string) error { return nil },
		runInstaller: func(installer, installDir string) error {
			if installer != `C:\updates\reames.exe` || installDir != `C:\Reames` {
				t.Fatalf("installer call = %q, %q", installer, installDir)
			}
			return errors.New("installer exit 7")
		},
		markApplyFailed: func(version, reason string) error {
			markedVersion, markedReason = version, reason
			return nil
		},
		startRelaunch: func(path, dir string) error {
			relaunchedPath, relaunchedDir = path, dir
			return nil
		},
	}
	code := runWithDependencies([]string{
		"--installer", `C:\updates\reames.exe`,
		"--install-dir", `C:\Reames`,
		"--relaunch", `C:\Reames\reames-agent-guard.exe`,
		"--to-version", "v2",
	}, deps)
	if code != 1 {
		t.Fatalf("code = %d, logs=%q", code, logs.String())
	}
	if markedVersion != "v2" || markedReason != "installer exit 7" {
		t.Fatalf("failure marker = %q, %q", markedVersion, markedReason)
	}
	if relaunchedPath != `C:\Reames\reames-agent-guard.exe` || relaunchedDir != `C:\Reames` {
		t.Fatalf("relaunch = %q, %q", relaunchedPath, relaunchedDir)
	}
}

func TestInstallerFailureMarkerFailureDoesNotLaunchUnattributedRelease(t *testing.T) {
	var logs bytes.Buffer
	relaunched := false
	deps := helperDependencies{
		logger:          log.New(&logs, "", 0),
		waitForExit:     func(uint32, time.Duration) error { return nil },
		setenv:          func(string, string) error { return nil },
		runInstaller:    func(string, string) error { return errors.New("failed") },
		markApplyFailed: func(string, string) error { return errors.New("state unavailable") },
		startRelaunch: func(string, string) error {
			relaunched = true
			return nil
		},
	}
	code := runWithDependencies([]string{"--installer", "installer.exe", "--relaunch", "reames-agent-guard.exe"}, deps)
	if code != 1 || relaunched {
		t.Fatalf("code=%d relaunched=%v logs=%q", code, relaunched, logs.String())
	}
	if !strings.Contains(logs.String(), "record installer failure") {
		t.Fatalf("logs = %q", logs.String())
	}
}
