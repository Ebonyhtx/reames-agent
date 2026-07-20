package gatewayservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func launchdTransactionDeps(t *testing.T) applyDeps {
	t.Helper()
	deps := defaultApplyDeps()
	deps.goos = "darwin"
	return deps
}

func launchdInstallTestPlan(path string, opts Options) Plan {
	return Plan{
		GOOS:   "darwin",
		Action: "install",
		Files:  []File{{Path: path, Mode: 0o644, Content: "new plist"}},
		Commands: []Command{
			launchdBootstrapCommand(opts, path),
			launchdKickstartCommand(opts),
		},
	}
}

func TestApplyLaunchdInstallRollsBackDefinitionAndRunningState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.reames-agent.gateway.plist")
	if err := os.WriteFile(path, []byte("old plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Name: defaultServiceName, Scope: "user", StartNow: true}
	plan := launchdInstallTestPlan(path, opts)
	deps := launchdTransactionDeps(t)
	current := launchdServiceState{loaded: true, running: true}
	deps.probeLaunchdState = func(context.Context, Options, bool, commandRunner) (launchdServiceState, []string, error) {
		return current, []string{"probe"}, nil
	}
	var calls []string
	kickstarts := 0
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		calls = append(calls, strings.Join(command.Args, " "))
		switch launchdVerb(command) {
		case "bootout":
			current = launchdServiceState{}
		case "bootstrap":
			current.loaded = true
			current.running = false
		case "kickstart":
			kickstarts++
			if kickstarts == 1 {
				return "forward kickstart output", errors.New("injected kickstart failure")
			}
			current = launchdServiceState{loaded: true, running: true}
		}
		return launchdVerb(command) + " output", nil
	}

	result, err := applyLaunchdInstall(context.Background(), opts, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "injected kickstart failure") {
		t.Fatalf("applyLaunchdInstall error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "old plist" {
		t.Fatalf("restored plist = %q, err=%v", data, readErr)
	}
	info, statErr := os.Stat(path)
	if statErr != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("restored plist mode = %v, err=%v", info, statErr)
	}
	if current != (launchdServiceState{loaded: true, running: true}) {
		t.Fatalf("restored launchd state = %+v", current)
	}
	joined := strings.Join(calls, "\n")
	for _, want := range []string{"bootout", "bootstrap", "kickstart"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("commands missing %q:\n%s", want, joined)
		}
	}
	if outputs := strings.Join(result.Outputs, "\n"); !strings.Contains(outputs, "forward kickstart output") || !strings.Contains(outputs, "bootstrap output") {
		t.Fatalf("forward/rollback outputs not preserved:\n%s", outputs)
	}
}

func TestApplyLaunchdInstallUsesFreshRollbackContextAfterCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.reames-agent.gateway.plist")
	opts := Options{Name: defaultServiceName, Scope: "user", StartNow: true}
	plan := launchdInstallTestPlan(path, opts)
	deps := launchdTransactionDeps(t)
	current := launchdServiceState{}
	deps.probeLaunchdState = func(context.Context, Options, bool, commandRunner) (launchdServiceState, []string, error) {
		return current, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	rollbackContextCreated := false
	deps.newRollbackContext = func(parent context.Context) (context.Context, context.CancelFunc) {
		if !errors.Is(parent.Err(), context.Canceled) {
			t.Fatalf("rollback parent error = %v, want canceled", parent.Err())
		}
		rollbackContextCreated = true
		return context.WithCancel(context.WithoutCancel(parent))
	}
	deps.runCommand = func(commandCtx context.Context, command Command) (string, error) {
		if launchdVerb(command) == "bootstrap" && !rollbackContextCreated {
			current.loaded = true
			cancel()
			return "forward canceled output", context.Canceled
		}
		if rollbackContextCreated && commandCtx.Err() != nil {
			t.Fatalf("rollback command inherited cancellation: %v", commandCtx.Err())
		}
		if launchdVerb(command) == "bootout" {
			current = launchdServiceState{}
		}
		return "rollback " + launchdVerb(command) + " output", nil
	}

	result, err := applyLaunchdInstall(ctx, opts, plan, deps)
	if !errors.Is(err, context.Canceled) || !rollbackContextCreated {
		t.Fatalf("apply result err=%v rollbackContext=%v", err, rollbackContextCreated)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("fresh plist survived canceled install rollback: %v", statErr)
	}
	if current.loaded {
		t.Fatalf("replacement service remained loaded: %+v", current)
	}
	if outputs := strings.Join(result.Outputs, "\n"); !strings.Contains(outputs, "forward canceled output") || !strings.Contains(outputs, "rollback bootout output") {
		t.Fatalf("forward/rollback outputs not preserved:\n%s", outputs)
	}
}

func TestApplyLaunchdUninstallRollsBackPostconditionFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "com.reames-agent.gateway.plist")
	if err := os.WriteFile(path, []byte("old plist"), 0o640); err != nil {
		t.Fatal(err)
	}
	opts := Options{Name: defaultServiceName, Scope: "user"}
	plan := Plan{GOOS: "darwin", Action: "uninstall", Deletes: []string{path}}
	deps := launchdTransactionDeps(t)
	current := launchdServiceState{loaded: true, running: true}
	probeCalls := 0
	deps.probeLaunchdState = func(context.Context, Options, bool, commandRunner) (launchdServiceState, []string, error) {
		probeCalls++
		if probeCalls == 2 {
			return current, []string{"postcondition output"}, errors.New("injected postcondition failure")
		}
		return current, []string{"probe output"}, nil
	}
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		switch launchdVerb(command) {
		case "bootout":
			current = launchdServiceState{}
		case "bootstrap":
			current = launchdServiceState{loaded: true}
		case "kickstart":
			current = launchdServiceState{loaded: true, running: true}
		}
		return "rollback " + launchdVerb(command) + " output", nil
	}

	result, err := applyLaunchdUninstall(context.Background(), opts, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "verify launchd gateway service uninstall") {
		t.Fatalf("applyLaunchdUninstall error = %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "old plist" {
		t.Fatalf("restored plist = %q, err=%v", data, readErr)
	}
	info, statErr := os.Stat(path)
	if statErr != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o640) {
		t.Fatalf("restored plist mode = %v, err=%v", info, statErr)
	}
	if current != (launchdServiceState{loaded: true, running: true}) {
		t.Fatalf("restored launchd state = %+v", current)
	}
	if outputs := strings.Join(result.Outputs, "\n"); !strings.Contains(outputs, "postcondition output") || !strings.Contains(outputs, "rollback bootstrap output") {
		t.Fatalf("postcondition/rollback outputs missing:\n%s", outputs)
	}
}

type launchdExitError struct{}

func (launchdExitError) Error() string { return "exit status 3" }
func (launchdExitError) ExitCode() int { return 3 }

func TestProbeLaunchdServiceState(t *testing.T) {
	opts := Options{Name: defaultServiceName, Scope: "user"}
	state, _, err := probeLaunchdServiceState(context.Background(), opts, true, func(context.Context, Command) (string, error) {
		return "service = {\n\tstate = running\n}", nil
	})
	if err != nil || !state.loaded || !state.running {
		t.Fatalf("running state = %+v, err=%v", state, err)
	}
	state, _, err = probeLaunchdServiceState(context.Background(), opts, false, func(context.Context, Command) (string, error) {
		return "Could not find service", launchdExitError{}
	})
	if err != nil || state.loaded {
		t.Fatalf("missing state = %+v, err=%v", state, err)
	}
	_, _, err = probeLaunchdServiceState(context.Background(), opts, false, func(context.Context, Command) (string, error) {
		return "service = {\n\tstate = waiting\n}", nil
	})
	if err == nil || !strings.Contains(err.Error(), "loaded without the expected plist") {
		t.Fatalf("orphaned service error = %v", err)
	}
}
