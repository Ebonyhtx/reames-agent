package gatewayservice

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func windowsInstallTestPlan(t *testing.T, opts Options) Plan {
	t.Helper()
	plan, err := BuildPlan("windows", opts)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func windowsTransactionDeps() applyDeps {
	deps := defaultApplyDeps()
	deps.goos = "windows"
	return deps
}

func TestProbeWindowsTaskStateUsesStructuredJSON(t *testing.T) {
	wantXML := "<Task><Settings><Enabled>true</Enabled></Settings></Task>"
	state, outputs, err := probeWindowsTaskState(context.Background(), Options{Name: "gateway"}, func(_ context.Context, command Command) (string, error) {
		if command.Name != "powershell.exe" || !strings.Contains(command.Args[len(command.Args)-1], "ConvertTo-Json -Compress") {
			t.Fatalf("probe command = %+v", command)
		}
		return `{"schema":1,"exists":true,"enabled":true,"running":false,"xml":"<Task><Settings><Enabled>true</Enabled></Settings></Task>"}`, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.exists || !state.enabled || state.running || state.xml != wantXML {
		t.Fatalf("state = %+v", state)
	}
	if len(outputs) != 1 || strings.Contains(outputs[0], wantXML) {
		t.Fatalf("probe output leaked task XML: %v", outputs)
	}
}

func TestWindowsInstallFailureRestoresExactTaskAndState(t *testing.T) {
	opts := Options{Action: "install", Name: "gateway", Scope: "user", Executable: `C:\Reames\reames-agent.exe`, StartNow: true}
	plan := windowsInstallTestPlan(t, opts)
	deps := windowsTransactionDeps()
	oldXML := "<Task><Settings><Enabled>false</Enabled></Settings></Task>"
	current := windowsTaskState{exists: true, enabled: false, running: true, xml: oldXML}
	deps.probeWindowsState = func(context.Context, Options, commandRunner) (windowsTaskState, []string, error) {
		return current, []string{"structured state"}, nil
	}
	rollbackContextCreated := false
	deps.newRollbackContext = func(parent context.Context) (context.Context, context.CancelFunc) {
		rollbackContextCreated = true
		return context.WithCancel(context.WithoutCancel(parent))
	}
	var calls []Command
	deps.runCommand = func(ctx context.Context, command Command) (string, error) {
		calls = append(calls, command)
		if command.Name == "schtasks.exe" {
			switch command.Args[0] {
			case "/End":
				current.running = false
				return "ended old task", nil
			case "/Create":
				current = windowsTaskState{exists: true, enabled: true, xml: "<new/>"}
				return "created replacement", nil
			case "/Run":
				return "forward run failed", errors.New("injected run failure")
			}
		}
		script := command.Args[len(command.Args)-1]
		switch {
		case strings.Contains(script, "Unregister-ScheduledTask"):
			current = windowsTaskState{}
		case strings.Contains(script, "Register-ScheduledTask"):
			if !strings.Contains(script, base64.StdEncoding.EncodeToString([]byte(oldXML))) {
				t.Fatalf("rollback did not carry exact exported XML: %s", script)
			}
			current = windowsTaskState{exists: true, enabled: false, xml: oldXML}
		case strings.Contains(script, "Enable-ScheduledTask"):
			current.enabled = true
		case strings.Contains(script, "Start-ScheduledTask"):
			current.running = true
		case strings.Contains(script, "Disable-ScheduledTask"):
			current.enabled = false
		}
		if rollbackContextCreated && ctx.Err() != nil {
			t.Fatalf("rollback command inherited cancellation: %v", ctx.Err())
		}
		return "rollback output", nil
	}
	result, err := applyWindowsInstall(context.Background(), opts, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "injected run failure") {
		t.Fatalf("applyWindowsInstall error = %v", err)
	}
	if !rollbackContextCreated || current != (windowsTaskState{exists: true, enabled: false, running: true, xml: oldXML}) {
		t.Fatalf("rollback=%t state=%+v", rollbackContextCreated, current)
	}
	outputs := strings.Join(result.Outputs, "\n")
	if !strings.Contains(outputs, "forward run failed") || !strings.Contains(outputs, "rollback output") {
		t.Fatalf("forward/rollback outputs missing:\n%s", outputs)
	}
	if len(calls) < 5 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].Name != "schtasks.exe" || calls[0].Args[0] != "/End" || calls[1].Args[0] != "/Create" {
		t.Fatalf("same-name running task was not ended before replacement: %+v", calls[:2])
	}
}

func TestWindowsCanceledFreshInstallUsesIndependentRollback(t *testing.T) {
	opts := Options{Action: "install", Name: "gateway", Scope: "user", Executable: `C:\Reames\reames-agent.exe`}
	plan := windowsInstallTestPlan(t, opts)
	deps := windowsTransactionDeps()
	current := windowsTaskState{}
	deps.probeWindowsState = func(context.Context, Options, commandRunner) (windowsTaskState, []string, error) {
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
		if command.Name == "schtasks.exe" {
			current = windowsTaskState{exists: true, enabled: true, xml: "<partial/>"}
			cancel()
			return "forward canceled output", ctx.Err()
		}
		if commandCtx.Err() != nil {
			t.Fatalf("rollback command inherited cancellation: %v", commandCtx.Err())
		}
		if strings.Contains(command.Args[len(command.Args)-1], "Unregister-ScheduledTask") {
			current = windowsTaskState{}
		}
		return "rollback delete output", nil
	}
	result, err := applyWindowsInstall(ctx, opts, plan, deps)
	if !errors.Is(err, context.Canceled) || !rollbackContextCreated {
		t.Fatalf("err=%v rollback=%t", err, rollbackContextCreated)
	}
	if current.exists {
		t.Fatalf("partial task survived rollback: %+v", current)
	}
	outputs := strings.Join(result.Outputs, "\n")
	if !strings.Contains(outputs, "forward canceled output") || !strings.Contains(outputs, "rollback delete output") {
		t.Fatalf("outputs = %s", outputs)
	}
}

func TestWindowsUninstallPostconditionFailureRestoresRunningTask(t *testing.T) {
	opts := Options{Action: "uninstall", Name: "gateway", Scope: "user", Executable: `C:\Reames\reames-agent.exe`}
	plan := windowsInstallTestPlan(t, opts)
	deps := windowsTransactionDeps()
	oldXML := "<Task><Settings><Enabled>true</Enabled></Settings></Task>"
	current := windowsTaskState{exists: true, enabled: true, running: true, xml: oldXML}
	probeCalls := 0
	deps.probeWindowsState = func(context.Context, Options, commandRunner) (windowsTaskState, []string, error) {
		probeCalls++
		if probeCalls == 2 {
			return windowsTaskState{exists: true, enabled: true, xml: "<orphan/>"}, []string{"postcondition still present"}, nil
		}
		return current, []string{"structured state"}, nil
	}
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		if command.Name == "schtasks.exe" {
			current = windowsTaskState{}
			return "deleted", nil
		}
		script := command.Args[len(command.Args)-1]
		switch {
		case strings.Contains(script, "Unregister-ScheduledTask"):
			current = windowsTaskState{}
		case strings.Contains(script, "Register-ScheduledTask"):
			current = windowsTaskState{exists: true, enabled: true, xml: oldXML}
		case strings.Contains(script, "Enable-ScheduledTask"):
			current.enabled = true
		case strings.Contains(script, "Start-ScheduledTask"):
			current.running = true
		}
		return "rollback output", nil
	}
	result, err := applyWindowsUninstall(context.Background(), opts, plan, deps)
	if err == nil || !strings.Contains(err.Error(), "verify Windows gateway task uninstall") {
		t.Fatalf("applyWindowsUninstall error = %v", err)
	}
	if current != (windowsTaskState{exists: true, enabled: true, running: true, xml: oldXML}) {
		t.Fatalf("restored state = %+v", current)
	}
	if outputs := strings.Join(result.Outputs, "\n"); !strings.Contains(outputs, "postcondition still present") || !strings.Contains(outputs, "rollback output") {
		t.Fatalf("outputs = %s", outputs)
	}
}

func TestWindowsRollbackFailureIsDegraded(t *testing.T) {
	opts := Options{Action: "install", Name: "gateway", Scope: "user", Executable: `C:\Reames\reames-agent.exe`}
	plan := windowsInstallTestPlan(t, opts)
	deps := windowsTransactionDeps()
	oldXML := "<Task/>"
	current := windowsTaskState{exists: true, enabled: true, xml: oldXML}
	deps.probeWindowsState = func(context.Context, Options, commandRunner) (windowsTaskState, []string, error) {
		return current, nil, nil
	}
	deps.runCommand = func(_ context.Context, command Command) (string, error) {
		if command.Name == "schtasks.exe" {
			current = windowsTaskState{exists: true, enabled: true, xml: "<partial/>"}
			return "forward create failed", errors.New("injected create failure")
		}
		script := command.Args[len(command.Args)-1]
		if strings.Contains(script, "Unregister-ScheduledTask") {
			current = windowsTaskState{}
			return "rollback delete output", nil
		}
		if strings.Contains(script, "Register-ScheduledTask") {
			return "rollback register failed", errors.New("injected restore failure")
		}
		return "rollback output", nil
	}
	result, err := applyWindowsInstall(context.Background(), opts, plan, deps)
	if err == nil {
		t.Fatal("applyWindowsInstall succeeded, want degraded rollback failure")
	}
	for _, want := range []string{"injected create failure", "injected restore failure", "degraded", "manual repair"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte(oldXML))) {
		t.Fatalf("rollback error leaked exported task XML: %v", err)
	}
	if outputs := strings.Join(result.Outputs, "\n"); !strings.Contains(outputs, "forward create failed") || !strings.Contains(outputs, "rollback register failed") {
		t.Fatalf("outputs = %s", outputs)
	}
}

func TestWindowsUninstallAbsentIsIdempotent(t *testing.T) {
	opts := Options{Action: "uninstall", Name: "gateway", Scope: "user", Executable: `C:\Reames\reames-agent.exe`}
	plan := windowsInstallTestPlan(t, opts)
	deps := windowsTransactionDeps()
	deps.probeWindowsState = func(context.Context, Options, commandRunner) (windowsTaskState, []string, error) {
		return windowsTaskState{}, []string{"absent"}, nil
	}
	deps.runCommand = func(context.Context, Command) (string, error) {
		t.Fatal("idempotent uninstall ran a mutation")
		return "", nil
	}
	result, err := applyWindowsUninstall(context.Background(), opts, plan, deps)
	if err != nil || len(result.Outputs) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
