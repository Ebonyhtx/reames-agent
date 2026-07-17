package main

import (
	"os"
	"strings"
	"testing"
)

func TestInstallerCommandLineIsSilentAndKeepsDFlagLast(t *testing.T) {
	got := installerCommandLine(`C:\Temp\Reames Agent Installer.exe`, `D:\Tools\Reames Agent App`)
	want := `"C:\Temp\Reames Agent Installer.exe" /S /D=D:\Tools\Reames Agent App`
	if got != want {
		t.Fatalf("installerCommandLine = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, `/D=D:\Tools\Reames Agent App`) {
		t.Fatalf("/D= must be the final unquoted NSIS token, got %q", got)
	}
}

func TestWindowsUpdateHandoffArgsCarryParentInstallAndRelaunch(t *testing.T) {
	got := windowsUpdateHandoffArgs(
		4242,
		`C:\Users\Jane Doe\AppData\Local\Reames Agent\updates\Reames Agent-windows-amd64-installer.exe`,
		`D:\Tools\Reames Agent App`,
		`D:\Tools\Reames Agent App\reames-agent-launcher.exe`,
		`v2.0.0`,
		`C:\Users\Jane Doe\AppData\Roaming\reames-agent`,
	)
	want := []string{
		"--parent-pid", "4242",
		"--installer", `C:\Users\Jane Doe\AppData\Local\Reames Agent\updates\Reames Agent-windows-amd64-installer.exe`,
		"--install-dir", `D:\Tools\Reames Agent App`,
		"--relaunch", `D:\Tools\Reames Agent App\reames-agent-launcher.exe`,
		"--to-version", `v2.0.0`,
		"--state-home", `C:\Users\Jane Doe\AppData\Roaming\reames-agent`,
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestWindowsInstallerScriptWaitsBeforeCopyingExecutable(t *testing.T) {
	data, err := os.ReadFile("build/windows/installer/project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		`!define REAMES_AGENT_UPDATE_HELPER "reames-agent-update-helper.exe"`,
		"Function reamesAgent.waitForExecutableUnlock",
		`FileOpen $1 "$INSTDIR\${PRODUCT_EXECUTABLE}" a`,
		"SetErrorLevel 1618",
		"Call reamesAgent.waitForExecutableUnlock",
		`File "/oname=${REAMES_AGENT_UPDATE_HELPER}" "${REAMES_AGENT_UPDATE_HELPER}"`,
		`Delete "$INSTDIR\${REAMES_AGENT_UPDATE_HELPER}"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("project.nsi missing %q", want)
		}
	}
	wait := strings.Index(script, "Call reamesAgent.waitForExecutableUnlock")
	copyFiles := strings.Index(script, "!insertmacro wails.files")
	if wait < 0 || copyFiles < 0 || wait > copyFiles {
		t.Fatalf("installer must wait for the running exe to unlock before wails.files (wait=%d copy=%d)", wait, copyFiles)
	}
}

func TestDesktopBuildScriptCompilesAndPackagesWindowsUpdateHelper(t *testing.T) {
	data, err := os.ReadFile("../scripts/desktop-build.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		`UPDATE_HELPER="reames-agent-update-helper.exe"`,
		`GOOS=windows GOARCH="$arch" go build`,
		`./cmd/update-helper`,
		`build/windows/installer/$UPDATE_HELPER`,
		`cp "$helper" "$staging/$UPDATE_HELPER"`,
		`Compress-Archive -Force -Path '$staging_win\\*'`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("desktop-build.sh missing %q", want)
		}
	}
}
