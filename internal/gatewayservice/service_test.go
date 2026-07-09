package gatewayservice

import (
	"strings"
	"testing"
)

func TestLinuxInstallPlanRendersSystemdUserService(t *testing.T) {
	plan, err := BuildPlan("linux", Options{
		Action:     "install",
		Executable: "/opt/reames/reames-agent",
		Channels:   "feishu,qq",
		Dir:        "/srv/work repo",
		Model:      "deepseek-pro",
		StartNow:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(plan.Files))
	}
	unit := plan.Files[0].Content
	for _, want := range []string{
		"ExecStart=",
		`"gateway" "run"`,
		`"--channels" "feishu,qq"`,
		`"--dir" "/srv/work repo"`,
		`"--model" "deepseek-pro"`,
		"Restart=always",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("systemd unit missing %q:\n%s", want, unit)
		}
	}
	if len(plan.Commands) != 2 || plan.Commands[0].Name != "systemctl" {
		t.Fatalf("commands = %#v, want daemon-reload + enable", plan.Commands)
	}
	if got := strings.Join(plan.Commands[1].Args, " "); !strings.Contains(got, "enable --now reames-agent-gateway.service") {
		t.Fatalf("enable command args = %q", got)
	}
}

func TestDarwinInstallPlanRendersLaunchdPlist(t *testing.T) {
	plan, err := BuildPlan("darwin", Options{
		Action:     "install",
		Executable: "/Applications/Reames Agent.app/Contents/MacOS/reames-agent",
		Channels:   "feishu",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(plan.Files))
	}
	plist := plan.Files[0].Content
	for _, want := range []string{
		"<key>ProgramArguments</key>",
		"<string>gateway</string>",
		"<string>run</string>",
		"<string>--channels</string>",
		"<string>feishu</string>",
		"<key>KeepAlive</key>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("launchd plist missing %q:\n%s", want, plist)
		}
	}
}

func TestWindowsInstallPlanRendersScheduledTask(t *testing.T) {
	plan, err := BuildPlan("windows", Options{
		Action:     "install",
		Executable: `C:\Program Files\Reames Agent\reames-agent.exe`,
		Channels:   "feishu",
		StartNow:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 2 {
		t.Fatalf("commands = %d, want create + run", len(plan.Commands))
	}
	create := plan.Commands[0]
	if create.Name != "schtasks.exe" {
		t.Fatalf("command name = %q, want schtasks.exe", create.Name)
	}
	got := strings.Join(create.Args, " ")
	for _, want := range []string{
		"/Create",
		"/SC ONLOGON",
		`\ReamesAgent\reames-agent-gateway`,
		"gateway run",
		"--channels feishu",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("scheduled task command missing %q:\n%#v", want, create.Args)
		}
	}
}

func TestInvalidScopeIsRejected(t *testing.T) {
	if _, err := BuildPlan("linux", Options{Action: "status", Scope: "planet", Executable: "reames-agent"}); err == nil {
		t.Fatal("BuildPlan accepted invalid scope")
	}
}

func TestUninstallPlanDeletesServiceDefinition(t *testing.T) {
	linux, err := BuildPlan("linux", Options{Action: "uninstall", Executable: "reames-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(linux.Deletes) != 1 || !strings.HasSuffix(linux.Deletes[0], "reames-agent-gateway.service") {
		t.Fatalf("linux uninstall deletes = %#v, want service unit", linux.Deletes)
	}

	darwin, err := BuildPlan("darwin", Options{Action: "uninstall", Executable: "reames-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(darwin.Deletes) != 1 || !strings.HasSuffix(darwin.Deletes[0], ".plist") {
		t.Fatalf("darwin uninstall deletes = %#v, want plist", darwin.Deletes)
	}
}
