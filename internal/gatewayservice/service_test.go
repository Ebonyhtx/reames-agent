package gatewayservice

import (
	"strings"
	"testing"
)

func TestLinuxInstallPlanRendersSystemdUserService(t *testing.T) {
	plan, err := BuildPlan("linux", Options{
		Action:     "install",
		Executable: "/opt/reames/reames-agent",
		Home:       "/home/reames/.reames-agent",
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
		`Environment=REAMES_AGENT_HOME="/home/reames/.reames-agent"`,
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
		Home:       "/Users/reames/.reames-agent",
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
		"<key>EnvironmentVariables</key>",
		"<key>REAMES_AGENT_HOME</key>",
		"<string>/Users/reames/.reames-agent</string>",
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
		Home:       `C:\Users\reames\.reames-agent`,
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
		"REAMES_AGENT_HOME=C:\\Users\\reames\\.reames-agent",
		"gateway run",
		"--channels feishu",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("scheduled task command missing %q:\n%#v", want, create.Args)
		}
	}
}

func TestInstallPlansUseGatewayRunAndNeverLegacyEntrypoints(t *testing.T) {
	tests := []struct {
		goos string
		opts Options
	}{
		{
			goos: "linux",
			opts: Options{
				Action:     "install",
				Executable: "/opt/reames/reames-agent",
				Home:       "/home/reames/.reames-agent",
				Channels:   "feishu,telegram",
				Dir:        "/srv/reames work",
				Model:      "deepseek-pro",
				StartNow:   true,
			},
		},
		{
			goos: "darwin",
			opts: Options{
				Action:     "install",
				Executable: "/Applications/Reames Agent.app/Contents/MacOS/reames-agent",
				Home:       "/Users/reames/.reames-agent",
				Channels:   "feishu",
				Dir:        "/Users/reames/projects/demo",
				Model:      "deepseek-pro",
				StartNow:   true,
			},
		},
		{
			goos: "windows",
			opts: Options{
				Action:     "install",
				Executable: `C:\Program Files\Reames Agent\reames-agent.exe`,
				Home:       `C:\Users\reames\.reames-agent`,
				Channels:   "feishu",
				Dir:        `D:\work repo`,
				Model:      "deepseek-pro",
				StartNow:   true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			plan, err := BuildPlan(tc.goos, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			formatted := FormatPlan(plan)
			for _, want := range []string{"gateway", "run", "REAMES_AGENT_HOME", "feishu", "deepseek-pro"} {
				if !strings.Contains(formatted, want) {
					t.Fatalf("formatted service plan missing %q:\n%s", want, formatted)
				}
			}
			for _, forbidden := range []string{" bot start", " serve ", " serve\"", "bot\" \"start"} {
				if strings.Contains(formatted, forbidden) {
					t.Fatalf("service plan regressed to legacy/serve entrypoint %q:\n%s", forbidden, formatted)
				}
			}
		})
	}
}

func TestLifecycleCommandPlansUsePlatformServiceManagers(t *testing.T) {
	tests := []struct {
		name string
		goos string
		opts Options
		want []string
	}{
		{
			name: "linux user status",
			goos: "linux",
			opts: Options{Action: "status", Executable: "/opt/reames/reames-agent"},
			want: []string{`"systemctl" "--user" "status" "reames-agent-gateway.service"`},
		},
		{
			name: "linux system restart",
			goos: "linux",
			opts: Options{Action: "restart", Scope: "system", Executable: "/opt/reames/reames-agent"},
			want: []string{`"systemctl" "restart" "reames-agent-gateway.service"`},
		},
		{
			name: "darwin status",
			goos: "darwin",
			opts: Options{Action: "status", Executable: "/Applications/Reames Agent.app/Contents/MacOS/reames-agent"},
			want: []string{`"launchctl" "print"`, "com.reames-agent.reames-agent-gateway"},
		},
		{
			name: "darwin restart",
			goos: "darwin",
			opts: Options{Action: "restart", Executable: "/Applications/Reames Agent.app/Contents/MacOS/reames-agent"},
			want: []string{`"launchctl" "kickstart" "-k"`, "com.reames-agent.reames-agent-gateway"},
		},
		{
			name: "windows status",
			goos: "windows",
			opts: Options{Action: "status", Executable: `C:\Program Files\Reames Agent\reames-agent.exe`},
			want: []string{`"schtasks.exe" "/Query"`, `\\ReamesAgent\\reames-agent-gateway`, `"/FO" "LIST" "/V"`},
		},
		{
			name: "windows restart",
			goos: "windows",
			opts: Options{Action: "restart", Executable: `C:\Program Files\Reames Agent\reames-agent.exe`},
			want: []string{`"schtasks.exe" "/End"`, `"schtasks.exe" "/Run"`, `\\ReamesAgent\\reames-agent-gateway`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := BuildPlan(tc.goos, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			formatted := FormatPlan(plan)
			if len(plan.Files) != 0 || len(plan.Deletes) != 0 {
				t.Fatalf("lifecycle action should not render file mutations: files=%#v deletes=%#v", plan.Files, plan.Deletes)
			}
			for _, want := range tc.want {
				if !strings.Contains(formatted, want) {
					t.Fatalf("formatted lifecycle plan missing %q:\n%s", want, formatted)
				}
			}
		})
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
