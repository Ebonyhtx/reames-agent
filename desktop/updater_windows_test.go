//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerCommandPassesUnquotedDFlagLast(t *testing.T) {
	cmd := installerCommand(`C:\Temp\reamesAgent-update-1.exe`, `D:\Tools\Reames Agent App`)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected a raw command line forcing the install dir")
	}
	got := cmd.SysProcAttr.CmdLine
	want := `"C:\Temp\reamesAgent-update-1.exe" /S /D=D:\Tools\Reames Agent App`
	if got != want {
		t.Fatalf("CmdLine = %q, want %q", got, want)
	}
}

func TestInstallerCommandWithoutDirSkipsDFlag(t *testing.T) {
	cmd := installerCommand(`C:\Temp\reamesAgent-update-1.exe`, "")
	if cmd.SysProcAttr == nil {
		t.Fatal("expected a raw command line for silent updater installs")
	}
	got := cmd.SysProcAttr.CmdLine
	want := `"C:\Temp\reamesAgent-update-1.exe" /S`
	if got != want {
		t.Fatalf("CmdLine = %q, want %q", got, want)
	}
}

func TestWindowsUpdateHandoffFailsClosedWithoutPackagedHelper(t *testing.T) {
	dir := t.TempDir()
	err := startWindowsUpdateHandoff(
		filepath.Join(dir, "verified-installer.exe"),
		dir,
		filepath.Join(dir, "reames-agent-launcher.exe"),
		"v2",
		t.TempDir(),
	)
	if err == nil || !strings.Contains(err.Error(), "start verified update helper") {
		t.Fatalf("missing helper error = %v", err)
	}
}
