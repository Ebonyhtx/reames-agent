package main

import (
	"os"
	"strings"
	"testing"
)

func TestDesktopPackagesUseGuardAsDefaultLauncher(t *testing.T) {
	buildData, err := os.ReadFile("../scripts/desktop-build.sh")
	if err != nil {
		t.Fatal(err)
	}
	build := string(buildData)
	for _, want := range []string{
		`./cmd/reames-agent-guard`,
		`cp "$guard_out" "$app/Contents/MacOS/$GUARDNAME"`,
		`Set :CFBundleExecutable $GUARDNAME`,
		`-H windowsgui`,
		`cp "$launcher_out" "$staging/${APPNAME}.exe"`,
		`cp "$guard_out" "$staging/$GUARDNAME.exe"`,
		`"$BINNAME" "$GUARDNAME" licenses`,
	} {
		if !strings.Contains(build, want) {
			t.Errorf("desktop-build.sh missing Guard contract %q", want)
		}
	}

	linuxData, err := os.ReadFile("build/linux/reames-agent.desktop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(linuxData), "Exec=reames-agent-guard launch --detach") {
		t.Fatal("Linux desktop entry does not launch through Guard")
	}
	nfpmData, err := os.ReadFile("build/linux/nfpm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nfpmData), "/usr/bin/reames-agent-guard") {
		t.Fatal("Linux package omits Guard")
	}
	windowsData, err := os.ReadFile("build/windows/installer/project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	windows := string(windowsData)
	for _, want := range []string{
		`!define REAMES_AGENT_GUARD "reames-agent-guard.exe"`,
		`!define REAMES_AGENT_LAUNCHER "reames-agent-launcher.exe"`,
		`!define REAMES_AGENT_UPDATE_HELPER "reames-agent-update-helper.exe"`,
		`"$INSTDIR\${REAMES_AGENT_LAUNCHER}" "launch --detach"`,
	} {
		if !strings.Contains(windows, want) {
			t.Errorf("Windows installer missing Guard contract %q", want)
		}
	}
}
