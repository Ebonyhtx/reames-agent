package main

import (
	"strings"
	"testing"
)

func TestInstallerCommandLineIsSilentAndLeavesDFlagLast(t *testing.T) {
	got := installerCommandLine(`C:\Temp\Reames Agent Installer.exe`, `D:\Tools\Reames Agent App`)
	want := `"C:\Temp\Reames Agent Installer.exe" /S /D=D:\Tools\Reames Agent App`
	if got != want {
		t.Fatalf("installerCommandLine = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, `/D=D:\Tools\Reames Agent App`) {
		t.Fatalf("/D= must be the final unquoted NSIS token, got %q", got)
	}
}
