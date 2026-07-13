package gatewayservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"reames-agent/internal/fileutil"
)

const linuxInstallRollbackTimeout = 15 * time.Second

type commandRunner func(context.Context, Command) (string, error)

type linuxServiceState struct {
	enabled bool
	active  bool
}

type linuxInstallSnapshot struct {
	unitExists bool
	unitData   []byte
	unitMode   os.FileMode
	service    linuxServiceState
}

type linuxInstallProgress struct {
	unitWritten      bool
	reloadAttempted  bool
	enableAttempted  bool
	runtimeAttempted bool
}

type applyDeps struct {
	goos               string
	lstat              func(string) (os.FileInfo, error)
	readFile           func(string) ([]byte, error)
	mkdirAll           func(string, os.FileMode) error
	atomicWriteFile    func(string, []byte, os.FileMode) error
	remove             func(string) error
	runCommand         commandRunner
	probeLinuxState    func(context.Context, Options, bool, commandRunner) (linuxServiceState, []string, error)
	newRollbackContext func(context.Context) (context.Context, context.CancelFunc)
}

func defaultApplyDeps() applyDeps {
	return applyDeps{
		goos:            runtime.GOOS,
		lstat:           os.Lstat,
		readFile:        os.ReadFile,
		mkdirAll:        os.MkdirAll,
		atomicWriteFile: fileutil.AtomicWriteFile,
		remove:          os.Remove,
		runCommand:      executeCommand,
		probeLinuxState: probeLinuxServiceState,
		newRollbackContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.WithoutCancel(parent), linuxInstallRollbackTimeout)
		},
	}
}

// Apply builds and applies a lifecycle operation. With DryRun, it only returns
// the rendered plan. Actual install defaults to user-level service management.
func Apply(ctx context.Context, opts Options) (Result, error) {
	return applyWithDeps(ctx, opts, defaultApplyDeps())
}

func applyWithDeps(ctx context.Context, opts Options, deps applyDeps) (Result, error) {
	normalized, err := NormalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}
	plan, err := BuildPlan(deps.goos, normalized)
	if err != nil {
		return Result{}, err
	}
	if normalized.DryRun {
		return Result{Plan: plan}, nil
	}
	if normalized.Scope == "system" {
		return Result{Plan: plan}, errors.New("system-scope gateway service changes require manual approval; re-run with --dry-run and install the rendered plan as administrator/root")
	}
	if plan.GOOS == "linux" && plan.Action == "install" {
		return applyLinuxInstall(ctx, normalized, plan, deps)
	}
	return applyGenericPlan(ctx, plan, deps)
}

func applyGenericPlan(ctx context.Context, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	for _, file := range plan.Files {
		if err := deps.mkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return result, fmt.Errorf("create service definition directory: %w", err)
		}
		if err := deps.atomicWriteFile(file.Path, []byte(file.Content), file.Mode); err != nil {
			return result, fmt.Errorf("write service definition %s: %w", file.Path, err)
		}
	}
	for _, command := range plan.Commands {
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			return result, err
		}
	}
	for _, path := range plan.Deletes {
		if err := deps.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("delete service definition %s: %w", path, err)
		}
	}
	for _, command := range plan.PostCommands {
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			return result, err
		}
	}
	return result, nil
}

func applyLinuxInstall(ctx context.Context, opts Options, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	if len(plan.Files) != 1 {
		return result, fmt.Errorf("linux gateway install rendered %d service definitions, want 1", len(plan.Files))
	}
	unit := plan.Files[0]
	snapshot, outputs, err := snapshotLinuxInstall(ctx, opts, unit.Path, deps)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		return result, err
	}
	if err := deps.mkdirAll(filepath.Dir(unit.Path), 0o755); err != nil {
		return result, fmt.Errorf("create service definition directory: %w", err)
	}
	progress := linuxInstallProgress{unitWritten: true}
	if err := deps.atomicWriteFile(unit.Path, []byte(unit.Content), unit.Mode); err != nil {
		applyErr := fmt.Errorf("write service definition %s: %w", unit.Path, err)
		rollbackOutputs, rollbackErr := rollbackLinuxInstall(ctx, opts, unit.Path, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback linux gateway service install: %w", rollbackErr))
		}
		return result, applyErr
	}

	for _, command := range plan.Commands {
		progress.recordAttempt(command)
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			rollbackOutputs, rollbackErr := rollbackLinuxInstall(ctx, opts, unit.Path, snapshot, progress, deps)
			result.Outputs = append(result.Outputs, rollbackOutputs...)
			if rollbackErr != nil {
				return result, errors.Join(err, fmt.Errorf("rollback linux gateway service install: %w", rollbackErr))
			}
			return result, err
		}
	}
	return result, nil
}

func (progress *linuxInstallProgress) recordAttempt(command Command) {
	if command.Name != "systemctl" {
		return
	}
	switch systemctlVerb(command) {
	case "daemon-reload":
		progress.reloadAttempted = true
	case "enable":
		progress.enableAttempted = true
	case "restart", "start":
		progress.runtimeAttempted = true
	}
}

func snapshotLinuxInstall(ctx context.Context, opts Options, unitPath string, deps applyDeps) (linuxInstallSnapshot, []string, error) {
	var snapshot linuxInstallSnapshot
	info, err := deps.lstat(unitPath)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return snapshot, nil, fmt.Errorf("existing service definition %s is not a regular file", unitPath)
		}
		data, readErr := deps.readFile(unitPath)
		if readErr != nil {
			return snapshot, nil, fmt.Errorf("read existing service definition %s: %w", unitPath, readErr)
		}
		snapshot.unitExists = true
		snapshot.unitData = data
		snapshot.unitMode = info.Mode().Perm()
	case errors.Is(err, os.ErrNotExist):
		// A missing definition is the expected state for a fresh install.
	default:
		return snapshot, nil, fmt.Errorf("inspect existing service definition %s: %w", unitPath, err)
	}

	state, outputs, err := deps.probeLinuxState(ctx, opts, snapshot.unitExists, deps.runCommand)
	if err != nil {
		return snapshot, outputs, fmt.Errorf("snapshot linux gateway service state: %w", err)
	}
	snapshot.service = state
	return snapshot, outputs, nil
}

func rollbackLinuxInstall(parent context.Context, opts Options, unitPath string, snapshot linuxInstallSnapshot, progress linuxInstallProgress, deps applyDeps) ([]string, error) {
	if !progress.unitWritten {
		return nil, nil
	}
	ctx, cancel := deps.newRollbackContext(parent)
	defer cancel()

	var outputs []string
	var rollbackErrors []error
	run := func(command Command) {
		if err := runAndRecord(ctx, &outputs, command, deps.runCommand); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	serviceName := opts.Name + ".service"
	serviceCommand := func(verb string) Command {
		return Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, verb, serviceName)}
	}

	if !snapshot.unitExists {
		// Undo enablement and runtime effects before deleting a fresh unit so
		// systemctl can still resolve the definition and its generated links.
		if progress.runtimeAttempted && !snapshot.service.active {
			run(serviceCommand("stop"))
		}
		if progress.enableAttempted && !snapshot.service.enabled {
			run(serviceCommand("disable"))
		}
		definitionRemoved := true
		if err := deps.remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("remove new service definition %s: %w", unitPath, err))
			definitionRemoved = false
		}
		if progress.reloadAttempted && definitionRemoved {
			run(Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "daemon-reload")})
		}
		return outputs, degradedLinuxInstallRollback(errors.Join(rollbackErrors...))
	}

	if err := deps.atomicWriteFile(unitPath, snapshot.unitData, snapshot.unitMode); err != nil {
		return outputs, degradedLinuxInstallRollback(fmt.Errorf("restore service definition %s: %w", unitPath, err))
	}
	if progress.reloadAttempted {
		before := len(rollbackErrors)
		run(Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "daemon-reload")})
		if len(rollbackErrors) != before {
			return outputs, degradedLinuxInstallRollback(errors.Join(rollbackErrors...))
		}
	}
	if progress.enableAttempted {
		if snapshot.service.enabled {
			run(serviceCommand("enable"))
		} else {
			run(serviceCommand("disable"))
		}
	}
	if progress.runtimeAttempted {
		if snapshot.service.active {
			run(serviceCommand("restart"))
		} else {
			run(serviceCommand("stop"))
		}
	}
	return outputs, degradedLinuxInstallRollback(errors.Join(rollbackErrors...))
}

func degradedLinuxInstallRollback(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w; service definition or manager state may be degraded; manual repair is required", err)
}

func probeLinuxServiceState(ctx context.Context, opts Options, unitExists bool, run commandRunner) (linuxServiceState, []string, error) {
	serviceName := opts.Name + ".service"
	var state linuxServiceState
	var outputs []string

	enabledStatus, output, err := probeSystemctlStatus(ctx, run,
		Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "is-enabled", serviceName)})
	outputs = appendOutput(outputs, output)
	if err != nil {
		return state, outputs, err
	}
	if unitExists {
		switch enabledStatus {
		case "enabled":
			state.enabled = true
		case "disabled":
		default:
			return state, outputs, fmt.Errorf("gateway service enablement %q cannot be restored exactly; unmask or normalize the service before reinstalling", enabledStatus)
		}
	} else if enabledStatus != "not-found" {
		return state, outputs, fmt.Errorf("gateway service has state %q without the expected unit file; uninstall or repair it before installing", enabledStatus)
	}

	activeStatus, output, err := probeSystemctlStatus(ctx, run,
		Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "is-active", serviceName)})
	outputs = appendOutput(outputs, output)
	if err != nil {
		return state, outputs, err
	}
	if unitExists {
		switch activeStatus {
		case "active":
			state.active = true
		case "inactive":
		default:
			return state, outputs, fmt.Errorf("gateway service state %q cannot be restored exactly; stop or repair the service before reinstalling", activeStatus)
		}
	} else if activeStatus != "inactive" && activeStatus != "unknown" {
		return state, outputs, fmt.Errorf("gateway service is %q without the expected unit file; stop or repair it before installing", activeStatus)
	}
	return state, outputs, nil
}

func probeSystemctlStatus(ctx context.Context, run commandRunner, command Command) (string, string, error) {
	output, commandErr := run(ctx, command)
	status := lastOutputLine(output)
	if status != "" && (commandErr == nil || isExitError(commandErr)) {
		return status, output, nil
	}
	if commandErr != nil {
		return "", output, fmt.Errorf("%s: %w", shellLine(command), commandErr)
	}
	return "", output, fmt.Errorf("%s returned an empty state", shellLine(command))
}

func lastOutputLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(lines[len(lines)-1]))
}

func isExitError(err error) bool {
	var exitErr interface{ ExitCode() int }
	return errors.As(err, &exitErr)
}

func systemctlVerb(command Command) string {
	for _, arg := range command.Args {
		if arg == "--user" || strings.HasPrefix(arg, "--") {
			continue
		}
		return arg
	}
	return ""
}

func executeCommand(ctx context.Context, command Command) (string, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runAndRecord(ctx context.Context, outputs *[]string, command Command, run commandRunner) error {
	output, err := run(ctx, command)
	*outputs = appendOutput(*outputs, output)
	if err != nil {
		return fmt.Errorf("%s %s: %w", command.Name, strings.Join(command.Args, " "), err)
	}
	return nil
}

func appendOutput(outputs []string, output string) []string {
	if output = strings.TrimSpace(output); output != "" {
		return append(outputs, output)
	}
	return outputs
}
