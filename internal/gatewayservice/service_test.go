package gatewayservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		`Environment="REAMES_AGENT_HOME=/home/reames/.reames-agent"`,
		`WorkingDirectory=/srv/work repo`,
		"/home/reames/.reames-agent/.env",
		"service definitions do not embed secret values",
		"Restart=always",
		"Type=simple",
	} {
		if !strings.Contains(FormatPlan(plan), want) {
			t.Fatalf("systemd plan missing %q:\n%s", want, FormatPlan(plan))
		}
	}
	if strings.Contains(unit, "DEEPSEEK_API_KEY") || strings.Contains(unit, "FEISHU_BOT_APP_SECRET") {
		t.Fatalf("systemd unit embedded secret env names:\n%s", unit)
	}
	if len(plan.Commands) != 5 || plan.Commands[0].Name != "systemd-analyze" || plan.Commands[1].Name != "systemctl" {
		t.Fatalf("commands = %#v, want verify + daemon-reload + enable + restart + is-active", plan.Commands)
	}
	formatted := FormatPlan(plan)
	for _, want := range []string{
		`"systemd-analyze" "--user" "verify"`,
		`"systemctl" "--user" "enable" "reames-agent-gateway.service"`,
		`"systemctl" "--user" "restart" "reames-agent-gateway.service"`,
		`"systemctl" "--user" "is-active" "--quiet" "reames-agent-gateway.service"`,
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("install plan missing %q:\n%s", want, formatted)
		}
	}
}

func TestLinuxInstallPlanCanEnableSystemdWatchdog(t *testing.T) {
	plan, err := BuildPlan("linux", Options{
		Action:      "install",
		Executable:  "/opt/reames/reames-agent",
		Home:        "/home/reames/.reames-agent",
		WatchdogSec: 60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatPlan(plan)
	for _, want := range []string{
		"Type=notify",
		"NotifyAccess=main",
		"WatchdogSec=60s",
		"readiness begins only after recovery preflight",
		"heartbeats stop when every configured adapter is unhealthy",
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("watchdog plan missing %q:\n%s", want, formatted)
		}
	}
	if strings.Contains(formatted, "Type=simple") {
		t.Fatalf("watchdog plan retained Type=simple:\n%s", formatted)
	}
}

func TestGatewayWatchdogValidationIsFailClosed(t *testing.T) {
	base := Options{Action: "install", Executable: "/opt/reames/reames-agent"}
	tooShort := base
	tooShort.WatchdogSec = time.Second
	if _, err := BuildPlan("linux", tooShort); err == nil || !strings.Contains(err.Error(), "at least 2s") {
		t.Fatalf("short watchdog error = %v", err)
	}
	nonLinux := base
	nonLinux.Executable = "/Applications/Reames Agent/reames-agent"
	nonLinux.WatchdogSec = 30 * time.Second
	if _, err := BuildPlan("darwin", nonLinux); err == nil || !strings.Contains(err.Error(), "requires Linux systemd") {
		t.Fatalf("darwin watchdog error = %v", err)
	}
	nonInstall := base
	nonInstall.Action = "status"
	nonInstall.WatchdogSec = 30 * time.Second
	if _, err := BuildPlan("linux", nonInstall); err == nil || !strings.Contains(err.Error(), "only configurable during install") {
		t.Fatalf("status watchdog error = %v", err)
	}
}

func TestEveryServiceManagerUsesGatewayRunRecoveryPreflight(t *testing.T) {
	cases := []struct {
		goos string
		exe  string
		home string
		dir  string
	}{
		{goos: "linux", exe: "/opt/reames/reames-agent", home: "/home/reames/.reames-agent", dir: "/srv/work"},
		{goos: "darwin", exe: "/Applications/Reames Agent/reames-agent", home: "/Users/reames/.reames-agent", dir: "/Users/reames/work"},
		{goos: "windows", exe: `C:\Program Files\Reames Agent\reames-agent.exe`, home: `C:\Users\reames\.reames-agent`, dir: `C:\work`},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			plan, err := BuildPlan(tc.goos, Options{Action: "install", Executable: tc.exe, Home: tc.home, Dir: tc.dir})
			if err != nil {
				t.Fatal(err)
			}
			formatted := FormatPlan(plan)
			for _, want := range []string{"gateway", "run", "shared credential-free recovery preflight"} {
				if !strings.Contains(formatted, want) {
					t.Fatalf("%s service plan missing %q:\n%s", tc.goos, want, formatted)
				}
			}
		})
	}
}

func TestLinuxInstallRollbackRemovesNewDefinitionAfterStartFailure(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemd-analyze", Args: []string{"--user", "verify", unitPath}},
			{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
			{Name: "systemctl", Args: []string{"--user", "enable", "reames-agent-gateway.service"}},
			{Name: "systemctl", Args: []string{"--user", "restart", "reames-agent-gateway.service"}},
		},
	}

	var calls []string
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{}, nil, nil
	}
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		line := command.Name + " " + strings.Join(command.Args, " ")
		calls = append(calls, line)
		if strings.Contains(line, " restart ") {
			return "restart failed", errors.New("exit 1")
		}
		return "", nil
	}

	_, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "restart reames-agent-gateway.service") {
		t.Fatalf("applyPlan error = %v, want restart failure", err)
	}
	if _, statErr := os.Stat(unitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("new unit survived failed install: %v", statErr)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "disable reames-agent-gateway.service") {
		t.Fatalf("rollback did not disable the partially installed service:\n%s", joined)
	}
}

func TestLinuxInstallRollbackRestoresRunningDefinitionAndState(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	wantMode := before.Mode().Perm()
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemd-analyze", Args: []string{"--user", "verify", unitPath}},
			{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
			{Name: "systemctl", Args: []string{"--user", "enable", "reames-agent-gateway.service"}},
			{Name: "systemctl", Args: []string{"--user", "restart", "reames-agent-gateway.service"}},
		},
	}

	restarts := 0
	var calls []string
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{active: true, enabled: true}, nil, nil
	}
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		line := command.Name + " " + strings.Join(command.Args, " ")
		calls = append(calls, line)
		if strings.Contains(line, " restart ") {
			restarts++
			if restarts == 1 {
				return "restart failed", errors.New("exit 1")
			}
		}
		return "", nil
	}

	_, err = applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "restart reames-agent-gateway.service") {
		t.Fatalf("applyPlan error = %v, want restart failure", err)
	}
	got, readErr := os.ReadFile(unitPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old unit\n" {
		t.Fatalf("restored unit = %q, want old unit", got)
	}
	info, statErr := os.Stat(unitPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("restored mode = %#o, want original %#o", info.Mode().Perm(), wantMode)
	}
	if restarts != 2 {
		t.Fatalf("restart calls = %d, want failed apply + rollback restart", restarts)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "enable reames-agent-gateway.service") {
		t.Fatalf("rollback did not restore enabled state:\n%s", joined)
	}
}

func TestLinuxInstallRefusesStateThatCannotBeRestoredExactly(t *testing.T) {
	run := func(_ context.Context, command Command) (string, error) {
		if systemctlVerb(command) == "is-enabled" {
			return "static", nil
		}
		return "inactive", nil
	}
	_, _, err := probeLinuxServiceState(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, true, run)
	if err == nil || !strings.Contains(err.Error(), "cannot be restored exactly") {
		t.Fatalf("probeLinuxServiceState error = %v, want exact-state refusal", err)
	}
}

func TestLinuxFreshInstallReportsRollbackDisableFailure(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemctl", Args: []string{"--user", "enable", "reames-agent-gateway.service"}},
			{Name: "systemctl", Args: []string{"--user", "restart", "reames-agent-gateway.service"}},
		},
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{}, nil, nil
	}
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		switch systemctlVerb(command) {
		case "restart":
			return "", errors.New("restart failed")
		case "disable":
			return "", errors.New("disable rollback failed")
		default:
			return "", nil
		}
	}

	_, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "disable rollback failed") {
		t.Fatalf("applyLinuxInstall error = %v, want rollback failure", err)
	}
}

func TestLinuxInstallRollbackRestoreWriteFailureFailsClosedAndReportsDegradedState(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
			{Name: "systemctl", Args: []string{"--user", "enable", "reames-agent-gateway.service"}},
			{Name: "systemctl", Args: []string{"--user", "restart", "reames-agent-gateway.service"}},
		},
	}

	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, []string{"snapshot output"}, nil
	}
	writes := 0
	deps.atomicWriteFile = func(path string, data []byte, mode os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("injected definition restore failure")
		}
		return os.WriteFile(path, data, mode)
	}
	var calls []string
	restarts := 0
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		verb := systemctlVerb(command)
		calls = append(calls, verb)
		if verb == "restart" {
			restarts++
			if restarts == 1 {
				return "forward restart output", errors.New("injected restart failure")
			}
		}
		return "rollback " + verb + " output", nil
	}

	result, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "injected restart failure") || !strings.Contains(err.Error(), "injected definition restore failure") {
		t.Fatalf("applyLinuxInstall error = %v, want forward and rollback errors", err)
	}
	for _, want := range []string{"rollback linux gateway service install", "restore service definition " + unitPath, "degraded", "manual repair"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("applyLinuxInstall error = %v, want %q", err, want)
		}
	}
	if writes != 2 {
		t.Fatalf("atomic writes = %d, want new definition + failed old definition restore", writes)
	}
	data, readErr := os.ReadFile(unitPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "new unit\n" {
		t.Fatalf("unit after degraded rollback = %q, want new unit still on disk", data)
	}
	if joinedCalls := strings.Join(calls, ","); joinedCalls != "daemon-reload,enable,restart" {
		t.Fatalf("rollback changed manager state after definition restore failure: calls=%s", joinedCalls)
	}
	joinedOutputs := strings.Join(result.Outputs, "\n")
	for _, want := range []string{"snapshot output", "forward restart output"} {
		if !strings.Contains(joinedOutputs, want) {
			t.Fatalf("result outputs missing %q:\n%s", want, joinedOutputs)
		}
	}
}

func TestLinuxInstallRollbackReloadFailureStopsStateRestore(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
			{Name: "systemctl", Args: []string{"--user", "enable", "reames-agent-gateway.service"}},
			{Name: "systemctl", Args: []string{"--user", "restart", "reames-agent-gateway.service"}},
		},
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, nil, nil
	}
	var calls []string
	reloads := 0
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		verb := systemctlVerb(command)
		calls = append(calls, verb)
		switch verb {
		case "restart":
			return "", errors.New("injected forward restart failure")
		case "daemon-reload":
			reloads++
			if reloads == 2 {
				return "", errors.New("injected rollback reload failure")
			}
		}
		return "", nil
	}

	_, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if err == nil {
		t.Fatal("applyLinuxInstall succeeded, want rollback reload failure")
	}
	for _, want := range []string{"injected forward restart failure", "injected rollback reload failure", "degraded", "manual repair"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("applyLinuxInstall error = %v, want %q", err, want)
		}
	}
	if got := strings.Join(calls, ","); got != "daemon-reload,enable,restart,daemon-reload" {
		t.Fatalf("commands after rollback reload failure = %s", got)
	}
	data, readErr := os.ReadFile(unitPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "old unit\n" {
		t.Fatalf("restored unit = %q, want old unit", data)
	}
}

func TestLinuxFreshInstallRollbackRemoveFailureSkipsReloadAndReportsDegradedState(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	plan := Plan{
		GOOS:     "linux",
		Action:   "install",
		Files:    []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}},
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{}, nil, nil
	}
	deps.remove = func(string) error { return errors.New("injected unit remove failure") }
	var calls []string
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		calls = append(calls, systemctlVerb(command))
		return "", errors.New("injected forward reload failure")
	}

	_, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if err == nil {
		t.Fatal("applyLinuxInstall succeeded, want rollback remove failure")
	}
	for _, want := range []string{"injected forward reload failure", "injected unit remove failure", "degraded", "manual repair"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("applyLinuxInstall error = %v, want %q", err, want)
		}
	}
	if got := strings.Join(calls, ","); got != "daemon-reload" {
		t.Fatalf("commands after fresh unit remove failure = %s", got)
	}
	data, readErr := os.ReadFile(unitPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "new unit\n" {
		t.Fatalf("unit after degraded rollback = %q, want new unit", data)
	}
}

func TestLinuxRollbackUsesFreshContextAfterForwardCancellation(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		},
	}
	ctx, cancelForward := context.WithCancel(context.Background())
	defer cancelForward()
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{}, nil, nil
	}
	rollbackContextCreated := false
	deps.newRollbackContext = func(parent context.Context) (context.Context, context.CancelFunc) {
		if !errors.Is(parent.Err(), context.Canceled) {
			t.Fatalf("rollback parent error = %v, want canceled forward context", parent.Err())
		}
		rollbackContextCreated = true
		return context.WithCancel(context.Background())
	}
	calls := 0
	deps.runCommand = func(commandCtx context.Context, _ Command) (string, error) {
		calls++
		if calls == 1 {
			cancelForward()
			return "forward canceled output", context.Canceled
		}
		if commandCtx.Err() != nil {
			t.Fatalf("rollback command inherited cancellation: %v", commandCtx.Err())
		}
		return "rollback reload output", nil
	}

	result, err := applyLinuxInstall(ctx, Options{Name: defaultServiceName, Scope: "user"}, plan, deps)
	if !errors.Is(err, context.Canceled) || !rollbackContextCreated || calls != 2 {
		t.Fatalf("apply result err=%v rollbackContext=%v calls=%d", err, rollbackContextCreated, calls)
	}
	if _, statErr := os.Stat(unitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("fresh unit survived canceled install rollback: %v", statErr)
	}
	if got := strings.Join(result.Outputs, "\n"); !strings.Contains(got, "forward canceled output") || !strings.Contains(got, "rollback reload output") {
		t.Fatalf("forward/rollback outputs not preserved:\n%s", got)
	}
}

func TestLinuxRollbackRestoresAllSupportedOldServiceStates(t *testing.T) {
	states := []struct {
		name        string
		state       linuxServiceState
		wantEnable  string
		wantRuntime string
	}{
		{name: "enabled-active", state: linuxServiceState{enabled: true, active: true}, wantEnable: "enable", wantRuntime: "restart"},
		{name: "enabled-inactive", state: linuxServiceState{enabled: true}, wantEnable: "enable", wantRuntime: "stop"},
		{name: "disabled-active", state: linuxServiceState{active: true}, wantEnable: "disable", wantRuntime: "restart"},
		{name: "disabled-inactive", state: linuxServiceState{}, wantEnable: "disable", wantRuntime: "stop"},
	}
	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
			if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			plan := Plan{
				GOOS:   "linux",
				Action: "install",
				Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
				Commands: []Command{
					{Name: "systemctl", Args: []string{"--user", "enable", "reames-agent-gateway.service"}},
					{Name: "systemctl", Args: []string{"--user", "restart", "reames-agent-gateway.service"}},
				},
			}
			deps := defaultApplyDeps()
			deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
				return tc.state, nil, nil
			}
			var calls []string
			failed := false
			deps.runCommand = func(_ context.Context, command Command) (string, error) {
				verb := systemctlVerb(command)
				calls = append(calls, verb)
				if verb == "restart" && !failed {
					failed = true
					return "", errors.New("forward restart failed")
				}
				return "", nil
			}
			if _, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps); err == nil {
				t.Fatal("applyLinuxInstall succeeded, want injected restart failure")
			}
			if len(calls) < 4 || calls[len(calls)-2] != tc.wantEnable || calls[len(calls)-1] != tc.wantRuntime {
				t.Fatalf("rollback calls = %v, want final %s,%s", calls, tc.wantEnable, tc.wantRuntime)
			}
		})
	}
}

func TestLinuxStartNowFalseRollbackLeavesServiceStateUntouched(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		GOOS:   "linux",
		Action: "install",
		Files:  []File{{Path: unitPath, Content: "new unit\n", Mode: 0o644}},
		Commands: []Command{
			{Name: "systemd-analyze", Args: []string{"--user", "verify", unitPath}},
			{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		},
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, nil, nil
	}
	var verbs []string
	reloads := 0
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		if command.Name != "systemctl" {
			return "", nil
		}
		verb := systemctlVerb(command)
		verbs = append(verbs, verb)
		if verb == "daemon-reload" {
			reloads++
			if reloads == 1 {
				return "", errors.New("forward reload failed")
			}
		}
		return "", nil
	}
	if _, err := applyLinuxInstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, plan, deps); err == nil {
		t.Fatal("applyLinuxInstall succeeded, want reload failure")
	}
	if got := strings.Join(verbs, ","); got != "daemon-reload,daemon-reload" {
		t.Fatalf("StartNow=false rollback mutated enable/runtime state: %s", got)
	}
}

func linuxUninstallTestPlan(unitPath string) Plan {
	return Plan{
		GOOS:     "linux",
		Action:   "uninstall",
		Commands: []Command{{Name: "systemctl", Args: []string{"--user", "disable", "--now", "reames-agent-gateway.service"}}},
		Deletes:  []string{unitPath},
		PostCommands: []Command{{
			Name: "systemctl",
			Args: []string{"--user", "daemon-reload"},
		}},
	}
}

func TestLinuxUninstallTransactionVerifiesAbsentState(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := defaultApplyDeps()
	probeCalls := 0
	deps.probeLinuxState = func(_ context.Context, _ Options, unitExists bool, _ commandRunner) (linuxServiceState, []string, error) {
		probeCalls++
		switch probeCalls {
		case 1:
			if !unitExists {
				t.Fatal("snapshot probe did not observe the existing unit")
			}
			return linuxServiceState{enabled: true, active: true}, []string{"snapshot state"}, nil
		case 2:
			if unitExists {
				t.Fatal("postcondition probe still expected the unit to exist")
			}
			return linuxServiceState{}, []string{"verified absent"}, nil
		default:
			t.Fatalf("probe calls = %d, want 2", probeCalls)
			return linuxServiceState{}, nil, nil
		}
	}
	var calls []string
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		verb := systemctlVerb(command)
		calls = append(calls, verb)
		return verb + " output", nil
	}

	result, err := applyLinuxUninstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(unitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unit survived successful uninstall: %v", statErr)
	}
	if got := strings.Join(calls, ","); got != "disable,daemon-reload" {
		t.Fatalf("uninstall commands = %s", got)
	}
	outputs := strings.Join(result.Outputs, "\n")
	for _, want := range []string{"snapshot state", "disable output", "daemon-reload output", "verified absent"} {
		if !strings.Contains(outputs, want) {
			t.Fatalf("uninstall outputs missing %q:\n%s", want, outputs)
		}
	}
}

func TestLinuxUninstallIsIdempotentWhenDefinitionAndManagerStateAreAbsent(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "missing.service")
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(_ context.Context, _ Options, unitExists bool, _ commandRunner) (linuxServiceState, []string, error) {
		if unitExists {
			t.Fatal("missing unit reported as existing")
		}
		return linuxServiceState{}, []string{"already absent"}, nil
	}
	deps.remove = func(string) error {
		t.Fatal("idempotent uninstall attempted a delete")
		return nil
	}
	deps.runCommand = func(context.Context, Command) (string, error) {
		t.Fatal("idempotent uninstall invoked systemctl mutation")
		return "", nil
	}

	result, err := applyLinuxUninstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(result.Outputs, "\n"); got != "already absent" {
		t.Fatalf("idempotent uninstall outputs = %q", got)
	}
}

func TestLinuxUninstallReloadFailureRestoresDefinitionAndState(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, nil, nil
	}
	var calls []string
	reloads := 0
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		verb := systemctlVerb(command)
		calls = append(calls, verb)
		if verb == "daemon-reload" {
			reloads++
			if reloads == 1 {
				return "forward reload output", errors.New("injected uninstall reload failure")
			}
		}
		return "rollback " + verb + " output", nil
	}

	result, err := applyLinuxUninstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if err == nil || !strings.Contains(err.Error(), "injected uninstall reload failure") {
		t.Fatalf("uninstall error = %v", err)
	}
	data, readErr := os.ReadFile(unitPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "old unit\n" {
		t.Fatalf("restored unit = %q", data)
	}
	info, statErr := os.Stat(unitPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("restored mode = %#o, want %#o", info.Mode().Perm(), before.Mode().Perm())
	}
	if got := strings.Join(calls, ","); got != "disable,daemon-reload,daemon-reload,enable,restart" {
		t.Fatalf("uninstall rollback commands = %s", got)
	}
	outputs := strings.Join(result.Outputs, "\n")
	for _, want := range []string{"forward reload output", "rollback daemon-reload output", "rollback enable output", "rollback restart output"} {
		if !strings.Contains(outputs, want) {
			t.Fatalf("uninstall rollback outputs missing %q:\n%s", want, outputs)
		}
	}
}

func TestLinuxUninstallDeleteFailureRestoresManagerStateWithoutReload(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, nil, nil
	}
	deps.remove = func(string) error { return errors.New("injected uninstall delete failure") }
	var calls []string
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		calls = append(calls, systemctlVerb(command))
		return "", nil
	}

	_, err := applyLinuxUninstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if err == nil || !strings.Contains(err.Error(), "injected uninstall delete failure") {
		t.Fatalf("uninstall error = %v", err)
	}
	if got := strings.Join(calls, ","); got != "disable,enable,restart" {
		t.Fatalf("delete-failure rollback commands = %s", got)
	}
	if data, readErr := os.ReadFile(unitPath); readErr != nil || string(data) != "old unit\n" {
		t.Fatalf("unit after delete failure = %q, %v", data, readErr)
	}
}

func TestLinuxUninstallPostconditionFailureRollsBack(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := defaultApplyDeps()
	probeCalls := 0
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		probeCalls++
		if probeCalls == 1 {
			return linuxServiceState{enabled: true, active: true}, nil, nil
		}
		return linuxServiceState{}, []string{"orphan manager state"}, errors.New("service still loaded")
	}
	var calls []string
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		calls = append(calls, systemctlVerb(command))
		return "", nil
	}

	result, err := applyLinuxUninstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if err == nil || !strings.Contains(err.Error(), "verify linux gateway service uninstall") || !strings.Contains(err.Error(), "service still loaded") {
		t.Fatalf("uninstall postcondition error = %v", err)
	}
	if got := strings.Join(calls, ","); got != "disable,daemon-reload,daemon-reload,enable,restart" {
		t.Fatalf("postcondition rollback commands = %s", got)
	}
	if data, readErr := os.ReadFile(unitPath); readErr != nil || string(data) != "old unit\n" {
		t.Fatalf("unit after postcondition rollback = %q, %v", data, readErr)
	}
	if !strings.Contains(strings.Join(result.Outputs, "\n"), "orphan manager state") {
		t.Fatalf("postcondition diagnostics missing from outputs: %#v", result.Outputs)
	}
}

func TestLinuxUninstallRestoreWriteFailureFailsClosed(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, nil, nil
	}
	deps.atomicWriteFile = func(string, []byte, os.FileMode) error {
		return errors.New("injected uninstall restore failure")
	}
	var calls []string
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		verb := systemctlVerb(command)
		calls = append(calls, verb)
		if verb == "daemon-reload" {
			return "", errors.New("injected forward reload failure")
		}
		return "", nil
	}

	_, err := applyLinuxUninstall(context.Background(), Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if err == nil {
		t.Fatal("uninstall succeeded, want degraded rollback failure")
	}
	for _, want := range []string{"injected forward reload failure", "injected uninstall restore failure", "degraded", "manual repair"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("uninstall error = %v, want %q", err, want)
		}
	}
	if got := strings.Join(calls, ","); got != "disable,daemon-reload" {
		t.Fatalf("manager state changed after restore write failure: %s", got)
	}
	if _, statErr := os.Stat(unitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unit unexpectedly exists after degraded restore: %v", statErr)
	}
}

func TestLinuxUninstallRollbackUsesFreshContextAfterCancellation(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "reames-agent-gateway.service")
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancelForward := context.WithCancel(context.Background())
	defer cancelForward()
	deps := defaultApplyDeps()
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		return linuxServiceState{enabled: true, active: true}, nil, nil
	}
	rollbackContextCreated := false
	deps.newRollbackContext = func(parent context.Context) (context.Context, context.CancelFunc) {
		if !errors.Is(parent.Err(), context.Canceled) {
			t.Fatalf("rollback parent error = %v, want canceled", parent.Err())
		}
		rollbackContextCreated = true
		return context.WithCancel(context.Background())
	}
	var calls []string
	deps.runCommand = func(commandCtx context.Context, command Command) (string, error) {
		verb := systemctlVerb(command)
		calls = append(calls, verb)
		if len(calls) == 1 {
			cancelForward()
			return "forward canceled output", context.Canceled
		}
		if commandCtx.Err() != nil {
			t.Fatalf("rollback command inherited cancellation: %v", commandCtx.Err())
		}
		return "rollback " + verb + " output", nil
	}

	result, err := applyLinuxUninstall(ctx, Options{Name: defaultServiceName, Scope: "user"}, linuxUninstallTestPlan(unitPath), deps)
	if !errors.Is(err, context.Canceled) || !rollbackContextCreated {
		t.Fatalf("uninstall err=%v rollbackContext=%v", err, rollbackContextCreated)
	}
	if got := strings.Join(calls, ","); got != "disable,enable,restart" {
		t.Fatalf("canceled uninstall rollback commands = %s", got)
	}
	if data, readErr := os.ReadFile(unitPath); readErr != nil || string(data) != "old unit\n" {
		t.Fatalf("unit after canceled uninstall = %q, %v", data, readErr)
	}
	outputs := strings.Join(result.Outputs, "\n")
	for _, want := range []string{"forward canceled output", "rollback enable output", "rollback restart output"} {
		if !strings.Contains(outputs, want) {
			t.Fatalf("canceled uninstall outputs missing %q:\n%s", want, outputs)
		}
	}
}

func TestApplyWithDepsUsesLinuxUninstallTransaction(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	unitPath := filepath.Join(configHome, "systemd", "user", defaultServiceName+".service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte("old unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := defaultApplyDeps()
	deps.goos = "linux"
	probeCalls := 0
	deps.probeLinuxState = func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error) {
		probeCalls++
		if probeCalls == 1 {
			return linuxServiceState{enabled: true, active: true}, nil, nil
		}
		return linuxServiceState{}, nil, nil
	}
	deps.runCommand = func(context.Context, Command) (string, error) { return "", nil }

	result, err := applyWithDeps(context.Background(), Options{Action: "uninstall", Executable: "/usr/bin/reames-agent"}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.Deletes) != 1 || result.Plan.Deletes[0] != unitPath {
		t.Fatalf("uninstall plan deletes = %#v, want %s", result.Plan.Deletes, unitPath)
	}
	if probeCalls != 2 {
		t.Fatalf("transaction probes = %d, want snapshot + postcondition", probeCalls)
	}
	if _, statErr := os.Stat(unitPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("applyWithDeps left unit behind: %v", statErr)
	}
}

func TestApplyDryRunAndSystemScopeHaveNoSideEffects(t *testing.T) {
	deps := defaultApplyDeps()
	deps.goos = "linux"
	called := false
	deps.mkdirAll = func(string, os.FileMode) error { called = true; return nil }
	deps.atomicWriteFile = func(string, []byte, os.FileMode) error { called = true; return nil }
	deps.runCommand = func(context.Context, Command) (string, error) { called = true; return "", nil }
	opts := Options{Action: "install", Executable: "/usr/bin/reames-agent", DryRun: true}
	result, err := applyWithDeps(context.Background(), opts, deps)
	if err != nil || called || len(result.Plan.Files) != 1 {
		t.Fatalf("dry-run result=%+v err=%v called=%v", result, err, called)
	}
	opts.DryRun = false
	opts.Scope = "system"
	if _, err := applyWithDeps(context.Background(), opts, deps); err == nil || !strings.Contains(err.Error(), "manual approval") || called {
		t.Fatalf("system-scope err=%v called=%v", err, called)
	}
}

func TestSystemdUnitEscapesDirectiveSpecificValues(t *testing.T) {
	unit := systemdUnit(Options{
		Executable: `/opt/Reames Agent/reames-agent`,
		Home:       `/home/reames/$USER/100% ready`,
		Dir:        `/srv/work "quoted"`,
	})
	for _, want := range []string{
		`ExecStart="/opt/Reames Agent/reames-agent" "gateway" "run"`,
		`Environment="REAMES_AGENT_HOME=/home/reames/$USER/100%% ready"`,
		`WorkingDirectory=/srv/work "quoted"`,
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("systemd unit missing %q:\n%s", want, unit)
		}
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
	formatted := FormatPlan(plan)
	for _, want := range []string{"/Users/reames/.reames-agent/.env", "service definitions do not embed secret values"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("launchd plan missing %q:\n%s", want, formatted)
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
	formatted := FormatPlan(plan)
	for _, want := range []string{`C:\Users\reames\.reames-agent\.env`, "service definitions do not embed secret values"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("windows plan missing %q:\n%s", want, formatted)
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

func TestGatewayServicePlanDocumentsDefaultCredentialHomeWhenHomeUnset(t *testing.T) {
	plan, err := BuildPlan("linux", Options{Action: "status", Executable: "reames-agent"})
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatPlan(plan)
	if !strings.Contains(formatted, "no --home supplied") || !strings.Contains(formatted, "platform default Reames Agent home") {
		t.Fatalf("formatted plan missing default-home credential note:\n%s", formatted)
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

func TestInstallRejectsRelativePersistentPaths(t *testing.T) {
	tests := []struct {
		name string
		goos string
		opts Options
	}{
		{
			name: "linux executable",
			goos: "linux",
			opts: Options{Action: "install", Executable: "bin/reames-agent", Home: "/home/reames/.reames-agent", Dir: "/srv/work"},
		},
		{
			name: "linux home",
			goos: "linux",
			opts: Options{Action: "install", Executable: "/opt/reames-agent", Home: ".reames-agent", Dir: "/srv/work"},
		},
		{
			name: "linux working directory",
			goos: "linux",
			opts: Options{Action: "install", Executable: "/opt/reames-agent", Home: "/home/reames/.reames-agent", Dir: "work"},
		},
		{
			name: "windows working directory",
			goos: "windows",
			opts: Options{Action: "install", Executable: `C:\Program Files\Reames Agent\reames-agent.exe`, Home: `C:\Users\reames\.reames-agent`, Dir: "work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildPlan(tt.goos, tt.opts); err == nil || !strings.Contains(err.Error(), "must be an absolute") {
				t.Fatalf("BuildPlan error = %v, want absolute-path rejection", err)
			}
		})
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
	if len(linux.Commands) != 1 || len(linux.PostCommands) != 1 {
		t.Fatalf("linux uninstall commands = %#v post = %#v, want disable then post-delete reload", linux.Commands, linux.PostCommands)
	}
	formatted := FormatPlan(linux)
	disableAt := strings.Index(formatted, `"disable" "--now"`)
	deleteAt := strings.Index(formatted, "delete ")
	reloadAt := strings.Index(formatted, `run after delete: "systemctl" "--user" "daemon-reload"`)
	if disableAt < 0 || deleteAt <= disableAt || reloadAt <= deleteAt {
		t.Fatalf("linux uninstall order must be disable -> delete -> daemon-reload:\n%s", formatted)
	}

	darwin, err := BuildPlan("darwin", Options{Action: "uninstall", Executable: "reames-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(darwin.Deletes) != 1 || !strings.HasSuffix(darwin.Deletes[0], ".plist") {
		t.Fatalf("darwin uninstall deletes = %#v, want plist", darwin.Deletes)
	}
}
