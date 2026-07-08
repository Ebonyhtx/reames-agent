//go:build windows

package main

import "testing"

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
