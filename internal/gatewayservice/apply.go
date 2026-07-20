package gatewayservice

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

const serviceRollbackTimeout = 15 * time.Second

type commandRunner func(context.Context, Command) (string, error)

type linuxServiceState struct {
	enabled bool
	active  bool
}

type linuxServiceSnapshot struct {
	unitExists bool
	unitData   []byte
	unitMode   os.FileMode
	service    linuxServiceState
}

type launchdServiceState struct {
	loaded  bool
	running bool
}

type launchdServiceSnapshot struct {
	plistExists bool
	plistData   []byte
	plistMode   os.FileMode
	service     launchdServiceState
}

type launchdInstallProgress struct {
	bootoutAttempted   bool
	plistWritten       bool
	bootstrapAttempted bool
}

type launchdUninstallProgress struct {
	bootoutAttempted bool
	plistRemoved     bool
}

type windowsTaskState struct {
	exists  bool
	enabled bool
	running bool
	xml     string
}

type windowsTaskProgress struct {
	mutationAttempted bool
}

type linuxInstallProgress struct {
	unitWritten      bool
	reloadAttempted  bool
	enableAttempted  bool
	runtimeAttempted bool
}

type linuxUninstallProgress struct {
	disableAttempted bool
	unitRemoved      bool
	reloadAttempted  bool
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
	probeLaunchdState  func(context.Context, Options, bool, commandRunner) (launchdServiceState, []string, error)
	probeWindowsState  func(context.Context, Options, commandRunner) (windowsTaskState, []string, error)
	newRollbackContext func(context.Context) (context.Context, context.CancelFunc)
}

func defaultApplyDeps() applyDeps {
	return applyDeps{
		goos:              runtime.GOOS,
		lstat:             os.Lstat,
		readFile:          os.ReadFile,
		mkdirAll:          os.MkdirAll,
		atomicWriteFile:   fileutil.AtomicWriteFile,
		remove:            os.Remove,
		runCommand:        executeCommand,
		probeLinuxState:   probeLinuxServiceState,
		probeLaunchdState: probeLaunchdServiceState,
		probeWindowsState: probeWindowsTaskState,
		newRollbackContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.WithoutCancel(parent), serviceRollbackTimeout)
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
	if plan.GOOS == "linux" {
		switch plan.Action {
		case "install":
			return applyLinuxInstall(ctx, normalized, plan, deps)
		case "uninstall":
			return applyLinuxUninstall(ctx, normalized, plan, deps)
		}
	}
	if plan.GOOS == "darwin" {
		switch plan.Action {
		case "install":
			return applyLaunchdInstall(ctx, normalized, plan, deps)
		case "uninstall":
			return applyLaunchdUninstall(ctx, normalized, plan, deps)
		}
	}
	if plan.GOOS == "windows" {
		switch plan.Action {
		case "install":
			return applyWindowsInstall(ctx, normalized, plan, deps)
		case "uninstall":
			return applyWindowsUninstall(ctx, normalized, plan, deps)
		}
	}
	return applyGenericPlan(ctx, plan, deps)
}

func applyLaunchdInstall(ctx context.Context, opts Options, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	if len(plan.Files) != 1 {
		return result, fmt.Errorf("launchd gateway install rendered %d service definitions, want 1", len(plan.Files))
	}
	plist := plan.Files[0]
	snapshot, outputs, err := snapshotLaunchdService(ctx, opts, plist.Path, deps)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		return result, err
	}
	progress := launchdInstallProgress{}
	fail := func(applyErr error) (Result, error) {
		rollbackOutputs, rollbackErr := rollbackLaunchdInstall(ctx, opts, plist.Path, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback launchd gateway service install: %w", rollbackErr))
		}
		return result, applyErr
	}

	if opts.StartNow && snapshot.service.loaded {
		progress.bootoutAttempted = true
		if err := runAndRecord(ctx, &result.Outputs, launchdBootoutCommand(opts, plist.Path), deps.runCommand); err != nil {
			return fail(err)
		}
	}
	if err := deps.mkdirAll(filepath.Dir(plist.Path), 0o755); err != nil {
		return fail(fmt.Errorf("create service definition directory: %w", err))
	}
	progress.plistWritten = true
	if err := deps.atomicWriteFile(plist.Path, []byte(plist.Content), plist.Mode); err != nil {
		return fail(fmt.Errorf("write service definition %s: %w", plist.Path, err))
	}
	for _, command := range plan.Commands {
		if launchdVerb(command) == "bootstrap" {
			progress.bootstrapAttempted = true
		}
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			return fail(err)
		}
	}
	if opts.StartNow {
		state, outputs, err := deps.probeLaunchdState(ctx, opts, true, deps.runCommand)
		result.Outputs = append(result.Outputs, outputs...)
		if err != nil || !state.loaded || !state.running {
			if err == nil {
				err = fmt.Errorf("service is loaded=%t running=%t", state.loaded, state.running)
			}
			return fail(fmt.Errorf("verify launchd gateway service install: %w", err))
		}
	}
	return result, nil
}

func applyLaunchdUninstall(ctx context.Context, opts Options, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	if len(plan.Deletes) != 1 {
		return result, fmt.Errorf("launchd gateway uninstall rendered %d service definitions to delete, want 1", len(plan.Deletes))
	}
	plistPath := plan.Deletes[0]
	snapshot, outputs, err := snapshotLaunchdService(ctx, opts, plistPath, deps)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		return result, err
	}
	if !snapshot.plistExists && !snapshot.service.loaded {
		return result, nil
	}
	progress := launchdUninstallProgress{}
	fail := func(applyErr error) (Result, error) {
		rollbackOutputs, rollbackErr := rollbackLaunchdUninstall(ctx, opts, plistPath, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback launchd gateway service uninstall: %w", rollbackErr))
		}
		return result, applyErr
	}
	if snapshot.service.loaded {
		progress.bootoutAttempted = true
		if err := runAndRecord(ctx, &result.Outputs, launchdBootoutCommand(opts, plistPath), deps.runCommand); err != nil {
			return fail(err)
		}
	}
	if err := deps.remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("delete service definition %s: %w", plistPath, err))
	}
	progress.plistRemoved = true
	state, outputs, err := deps.probeLaunchdState(ctx, opts, false, deps.runCommand)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil || state.loaded {
		if err == nil {
			err = errors.New("service is still loaded")
		}
		return fail(fmt.Errorf("verify launchd gateway service uninstall: %w", err))
	}
	return result, nil
}

func applyWindowsInstall(ctx context.Context, opts Options, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	snapshot, outputs, err := deps.probeWindowsState(ctx, opts, deps.runCommand)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		return result, fmt.Errorf("snapshot Windows gateway task state: %w", err)
	}
	progress := windowsTaskProgress{}
	fail := func(applyErr error) (Result, error) {
		rollbackOutputs, rollbackErr := rollbackWindowsTask(ctx, opts, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback Windows gateway task install: %w", rollbackErr))
		}
		return result, applyErr
	}
	if opts.StartNow && snapshot.exists && snapshot.running {
		progress.mutationAttempted = true
		if err := runAndRecord(ctx, &result.Outputs, windowsEndTaskCommand(opts), deps.runCommand); err != nil {
			return fail(err)
		}
	}
	for _, command := range plan.Commands {
		progress.mutationAttempted = true
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			return fail(err)
		}
	}
	state, outputs, err := deps.probeWindowsState(ctx, opts, deps.runCommand)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil || !state.exists || !state.enabled || (opts.StartNow && !state.running) {
		if err == nil {
			err = fmt.Errorf("task is exists=%t enabled=%t running=%t", state.exists, state.enabled, state.running)
		}
		return fail(fmt.Errorf("verify Windows gateway task install: %w", err))
	}
	return result, nil
}

func applyWindowsUninstall(ctx context.Context, opts Options, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	snapshot, outputs, err := deps.probeWindowsState(ctx, opts, deps.runCommand)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		return result, fmt.Errorf("snapshot Windows gateway task state: %w", err)
	}
	if !snapshot.exists {
		return result, nil
	}
	progress := windowsTaskProgress{}
	fail := func(applyErr error) (Result, error) {
		rollbackOutputs, rollbackErr := rollbackWindowsTask(ctx, opts, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback Windows gateway task uninstall: %w", rollbackErr))
		}
		return result, applyErr
	}
	for _, command := range plan.Commands {
		progress.mutationAttempted = true
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			return fail(err)
		}
	}
	state, outputs, err := deps.probeWindowsState(ctx, opts, deps.runCommand)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil || state.exists {
		if err == nil {
			err = errors.New("task still exists")
		}
		return fail(fmt.Errorf("verify Windows gateway task uninstall: %w", err))
	}
	return result, nil
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
	snapshot, outputs, err := snapshotLinuxService(ctx, opts, unit.Path, deps)
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

func applyLinuxUninstall(ctx context.Context, opts Options, plan Plan, deps applyDeps) (Result, error) {
	result := Result{Plan: plan}
	if len(plan.Deletes) != 1 {
		return result, fmt.Errorf("linux gateway uninstall rendered %d service definitions to delete, want 1", len(plan.Deletes))
	}
	unitPath := plan.Deletes[0]
	snapshot, outputs, err := snapshotLinuxService(ctx, opts, unitPath, deps)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		return result, err
	}
	if !snapshot.unitExists {
		// A cleanly absent unit and manager state make uninstall idempotent. The
		// snapshot probe has already rejected orphaned or ambiguous state.
		return result, nil
	}

	progress := linuxUninstallProgress{}
	for _, command := range plan.Commands {
		if command.Name == "systemctl" && systemctlVerb(command) == "disable" {
			progress.disableAttempted = true
		}
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			rollbackOutputs, rollbackErr := rollbackLinuxUninstall(ctx, opts, unitPath, snapshot, progress, deps)
			result.Outputs = append(result.Outputs, rollbackOutputs...)
			if rollbackErr != nil {
				return result, errors.Join(err, fmt.Errorf("rollback linux gateway service uninstall: %w", rollbackErr))
			}
			return result, err
		}
	}

	if err := deps.remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		applyErr := fmt.Errorf("delete service definition %s: %w", unitPath, err)
		rollbackOutputs, rollbackErr := rollbackLinuxUninstall(ctx, opts, unitPath, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback linux gateway service uninstall: %w", rollbackErr))
		}
		return result, applyErr
	}
	progress.unitRemoved = true

	for _, command := range plan.PostCommands {
		if command.Name == "systemctl" && systemctlVerb(command) == "daemon-reload" {
			progress.reloadAttempted = true
		}
		if err := runAndRecord(ctx, &result.Outputs, command, deps.runCommand); err != nil {
			rollbackOutputs, rollbackErr := rollbackLinuxUninstall(ctx, opts, unitPath, snapshot, progress, deps)
			result.Outputs = append(result.Outputs, rollbackOutputs...)
			if rollbackErr != nil {
				return result, errors.Join(err, fmt.Errorf("rollback linux gateway service uninstall: %w", rollbackErr))
			}
			return result, err
		}
	}

	_, outputs, err = deps.probeLinuxState(ctx, opts, false, deps.runCommand)
	result.Outputs = append(result.Outputs, outputs...)
	if err != nil {
		applyErr := fmt.Errorf("verify linux gateway service uninstall: %w", err)
		rollbackOutputs, rollbackErr := rollbackLinuxUninstall(ctx, opts, unitPath, snapshot, progress, deps)
		result.Outputs = append(result.Outputs, rollbackOutputs...)
		if rollbackErr != nil {
			return result, errors.Join(applyErr, fmt.Errorf("rollback linux gateway service uninstall: %w", rollbackErr))
		}
		return result, applyErr
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

func snapshotLinuxService(ctx context.Context, opts Options, unitPath string, deps applyDeps) (linuxServiceSnapshot, []string, error) {
	var snapshot linuxServiceSnapshot
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

func rollbackLinuxInstall(parent context.Context, opts Options, unitPath string, snapshot linuxServiceSnapshot, progress linuxInstallProgress, deps applyDeps) ([]string, error) {
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
		return outputs, degradedLinuxServiceRollback(errors.Join(rollbackErrors...))
	}

	if err := deps.atomicWriteFile(unitPath, snapshot.unitData, snapshot.unitMode); err != nil {
		return outputs, degradedLinuxServiceRollback(fmt.Errorf("restore service definition %s: %w", unitPath, err))
	}
	if progress.reloadAttempted {
		before := len(rollbackErrors)
		run(Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "daemon-reload")})
		if len(rollbackErrors) != before {
			return outputs, degradedLinuxServiceRollback(errors.Join(rollbackErrors...))
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
	return outputs, degradedLinuxServiceRollback(errors.Join(rollbackErrors...))
}

func rollbackLinuxUninstall(parent context.Context, opts Options, unitPath string, snapshot linuxServiceSnapshot, progress linuxUninstallProgress, deps applyDeps) ([]string, error) {
	if !progress.disableAttempted && !progress.unitRemoved && !progress.reloadAttempted {
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

	if progress.unitRemoved {
		if err := deps.atomicWriteFile(unitPath, snapshot.unitData, snapshot.unitMode); err != nil {
			return outputs, degradedLinuxServiceRollback(fmt.Errorf("restore service definition %s: %w", unitPath, err))
		}
		before := len(rollbackErrors)
		run(Command{Name: "systemctl", Args: systemctlArgs(opts.Scope, "daemon-reload")})
		if len(rollbackErrors) != before {
			return outputs, degradedLinuxServiceRollback(errors.Join(rollbackErrors...))
		}
	}
	if progress.disableAttempted {
		if snapshot.service.enabled {
			run(serviceCommand("enable"))
		} else {
			run(serviceCommand("disable"))
		}
		if snapshot.service.active {
			run(serviceCommand("restart"))
		} else {
			run(serviceCommand("stop"))
		}
	}
	return outputs, degradedLinuxServiceRollback(errors.Join(rollbackErrors...))
}

func degradedLinuxServiceRollback(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w; service definition or manager state may be degraded; manual repair is required", err)
}

func snapshotLaunchdService(ctx context.Context, opts Options, plistPath string, deps applyDeps) (launchdServiceSnapshot, []string, error) {
	var snapshot launchdServiceSnapshot
	info, err := deps.lstat(plistPath)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return snapshot, nil, fmt.Errorf("existing service definition %s is not a regular file", plistPath)
		}
		data, readErr := deps.readFile(plistPath)
		if readErr != nil {
			return snapshot, nil, fmt.Errorf("read existing service definition %s: %w", plistPath, readErr)
		}
		snapshot.plistExists = true
		snapshot.plistData = data
		snapshot.plistMode = info.Mode().Perm()
	case errors.Is(err, os.ErrNotExist):
	default:
		return snapshot, nil, fmt.Errorf("inspect existing service definition %s: %w", plistPath, err)
	}
	state, outputs, err := deps.probeLaunchdState(ctx, opts, snapshot.plistExists, deps.runCommand)
	if err != nil {
		return snapshot, outputs, fmt.Errorf("snapshot launchd gateway service state: %w", err)
	}
	snapshot.service = state
	return snapshot, outputs, nil
}

func rollbackLaunchdInstall(parent context.Context, opts Options, plistPath string, snapshot launchdServiceSnapshot, progress launchdInstallProgress, deps applyDeps) ([]string, error) {
	if !progress.bootoutAttempted && !progress.plistWritten && !progress.bootstrapAttempted {
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

	if progress.bootstrapAttempted {
		state, probeOutputs, err := deps.probeLaunchdState(ctx, opts, true, deps.runCommand)
		outputs = append(outputs, probeOutputs...)
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("inspect replacement launchd state: %w", err))
		} else if state.loaded {
			run(launchdBootoutCommand(opts, plistPath))
		}
	}
	if snapshot.plistExists {
		if err := deps.atomicWriteFile(plistPath, snapshot.plistData, snapshot.plistMode); err != nil {
			return outputs, degradedLaunchdServiceRollback(fmt.Errorf("restore service definition %s: %w", plistPath, err))
		}
	} else if progress.plistWritten {
		if err := deps.remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return outputs, degradedLaunchdServiceRollback(fmt.Errorf("remove new service definition %s: %w", plistPath, err))
		}
	}
	restoreLaunchdManager(ctx, opts, plistPath, snapshot.service, &outputs, &rollbackErrors, deps)
	return outputs, degradedLaunchdServiceRollback(errors.Join(rollbackErrors...))
}

func rollbackLaunchdUninstall(parent context.Context, opts Options, plistPath string, snapshot launchdServiceSnapshot, progress launchdUninstallProgress, deps applyDeps) ([]string, error) {
	if !progress.bootoutAttempted && !progress.plistRemoved {
		return nil, nil
	}
	ctx, cancel := deps.newRollbackContext(parent)
	defer cancel()
	var outputs []string
	var rollbackErrors []error
	if snapshot.plistExists {
		if err := deps.atomicWriteFile(plistPath, snapshot.plistData, snapshot.plistMode); err != nil {
			return outputs, degradedLaunchdServiceRollback(fmt.Errorf("restore service definition %s: %w", plistPath, err))
		}
	}
	restoreLaunchdManager(ctx, opts, plistPath, snapshot.service, &outputs, &rollbackErrors, deps)
	return outputs, degradedLaunchdServiceRollback(errors.Join(rollbackErrors...))
}

func restoreLaunchdManager(ctx context.Context, opts Options, plistPath string, want launchdServiceState, outputs *[]string, rollbackErrors *[]error, deps applyDeps) {
	run := func(command Command) {
		if err := runAndRecord(ctx, outputs, command, deps.runCommand); err != nil {
			*rollbackErrors = append(*rollbackErrors, err)
		}
	}
	current, probeOutputs, err := deps.probeLaunchdState(ctx, opts, want.loaded, deps.runCommand)
	*outputs = append(*outputs, probeOutputs...)
	if err != nil {
		*rollbackErrors = append(*rollbackErrors, fmt.Errorf("inspect launchd state during rollback: %w", err))
		return
	}
	if want.loaded && !current.loaded {
		run(launchdBootstrapCommand(opts, plistPath))
		current, probeOutputs, err = deps.probeLaunchdState(ctx, opts, true, deps.runCommand)
		*outputs = append(*outputs, probeOutputs...)
		if err != nil {
			*rollbackErrors = append(*rollbackErrors, fmt.Errorf("inspect launchd state after restore bootstrap: %w", err))
			return
		}
	}
	if want.loaded && want.running != current.running {
		if want.running {
			run(launchdKickstartCommand(opts))
		} else {
			run(launchdStopCommand(opts))
		}
	}
	state, probeOutputs, err := deps.probeLaunchdState(ctx, opts, want.loaded, deps.runCommand)
	*outputs = append(*outputs, probeOutputs...)
	if err != nil {
		*rollbackErrors = append(*rollbackErrors, fmt.Errorf("verify restored launchd state: %w", err))
		return
	}
	if state != want {
		*rollbackErrors = append(*rollbackErrors, fmt.Errorf("verify restored launchd state: got loaded=%t running=%t, want loaded=%t running=%t", state.loaded, state.running, want.loaded, want.running))
	}
}

func degradedLaunchdServiceRollback(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w; launchd definition or manager state may be degraded; manual repair is required", err)
}

func probeLaunchdServiceState(ctx context.Context, opts Options, plistExists bool, run commandRunner) (launchdServiceState, []string, error) {
	command := Command{Name: "launchctl", Args: []string{"print", launchdTarget(opts)}}
	output, commandErr := run(ctx, command)
	outputs := appendOutput(nil, output)
	if commandErr == nil {
		state := launchdServiceState{loaded: true, running: launchdOutputRunning(output)}
		if !plistExists {
			return state, outputs, errors.New("gateway service is loaded without the expected plist; boot it out or repair it before continuing")
		}
		return state, outputs, nil
	}
	if isExitError(commandErr) && launchdOutputNotFound(output) {
		return launchdServiceState{}, outputs, nil
	}
	return launchdServiceState{}, outputs, fmt.Errorf("%s: %w", shellLine(command), commandErr)
}

func launchdOutputRunning(output string) bool {
	for _, line := range strings.Split(strings.ToLower(output), "\n") {
		if strings.TrimSpace(line) == "state = running" {
			return true
		}
	}
	return false
}

func launchdOutputNotFound(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "could not find service") || strings.Contains(lower, "service not found") || strings.Contains(lower, "no such process")
}

func launchdTarget(opts Options) string {
	return launchdDomain(opts.Scope) + "/" + launchdLabel(opts)
}

func launchdVerb(command Command) string {
	if command.Name != "launchctl" || len(command.Args) == 0 {
		return ""
	}
	return command.Args[0]
}

func launchdBootstrapCommand(opts Options, plistPath string) Command {
	return Command{Name: "launchctl", Args: []string{"bootstrap", launchdDomain(opts.Scope), plistPath}}
}

func launchdBootoutCommand(opts Options, plistPath string) Command {
	return Command{Name: "launchctl", Args: []string{"bootout", launchdDomain(opts.Scope), plistPath}}
}

func launchdKickstartCommand(opts Options) Command {
	return Command{Name: "launchctl", Args: []string{"kickstart", "-k", launchdTarget(opts)}}
}

func launchdStopCommand(opts Options) Command {
	return Command{Name: "launchctl", Args: []string{"kill", "TERM", launchdTarget(opts)}}
}

func rollbackWindowsTask(parent context.Context, opts Options, snapshot windowsTaskState, progress windowsTaskProgress, deps applyDeps) ([]string, error) {
	if !progress.mutationAttempted {
		return nil, nil
	}
	ctx, cancel := deps.newRollbackContext(parent)
	defer cancel()
	var outputs []string
	var rollbackErrors []error
	run := func(label string, command Command) {
		output, err := deps.runCommand(ctx, command)
		outputs = appendOutput(outputs, output)
		if err != nil {
			// Do not include command.Args here: Register-ScheduledTask carries the
			// exact exported XML as base64, which may contain sensitive legacy task
			// arguments even though Reames-generated definitions contain no secrets.
			rollbackErrors = append(rollbackErrors, fmt.Errorf("%s: %w", label, err))
		}
	}

	// A failed create/delete may still have replaced or removed the task. Always
	// clear the current definition before restoring the exact exported XML.
	run("remove replacement Windows gateway task", windowsDeleteTaskCommand(opts))
	if snapshot.exists {
		if strings.TrimSpace(snapshot.xml) == "" {
			return outputs, degradedWindowsTaskRollback(errors.New("snapshot task XML is empty"))
		}
		run("restore Windows gateway task definition", windowsRegisterTaskCommand(opts, snapshot.xml))
		// Temporarily enable the task so a previously running-but-disabled task
		// can be restarted before its disabled flag is restored.
		run("enable restored Windows gateway task", windowsSetTaskEnabledCommand(opts, true))
		current, probeOutputs, err := deps.probeWindowsState(ctx, opts, deps.runCommand)
		outputs = append(outputs, probeOutputs...)
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("inspect restored Windows gateway task: %w", err))
		} else if snapshot.running != current.running {
			run("restore Windows gateway task runtime state", windowsSetTaskRunningCommand(opts, snapshot.running))
		}
		if !snapshot.enabled {
			run("disable restored Windows gateway task", windowsSetTaskEnabledCommand(opts, false))
		}
	}
	state, probeOutputs, err := deps.probeWindowsState(ctx, opts, deps.runCommand)
	outputs = append(outputs, probeOutputs...)
	if err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("verify restored Windows gateway task: %w", err))
	} else if state.exists != snapshot.exists || state.enabled != snapshot.enabled || state.running != snapshot.running {
		rollbackErrors = append(rollbackErrors, fmt.Errorf(
			"verify restored Windows gateway task: got exists=%t enabled=%t running=%t, want exists=%t enabled=%t running=%t",
			state.exists, state.enabled, state.running, snapshot.exists, snapshot.enabled, snapshot.running,
		))
	}
	return outputs, degradedWindowsTaskRollback(errors.Join(rollbackErrors...))
}

func degradedWindowsTaskRollback(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w; Windows Scheduled Task definition or runtime state may be degraded; manual repair is required", err)
}

func probeWindowsTaskState(ctx context.Context, opts Options, run commandRunner) (windowsTaskState, []string, error) {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
Import-Module ScheduledTasks -ErrorAction Stop
$task = Get-ScheduledTask -TaskPath %s -TaskName %s -ErrorAction SilentlyContinue
if ($null -eq $task) {
  [ordered]@{schema=1;exists=$false;enabled=$false;running=$false;xml=''} | ConvertTo-Json -Compress
  exit 0
}
$xml = Export-ScheduledTask -TaskPath %s -TaskName %s -ErrorAction Stop
[ordered]@{schema=1;exists=$true;enabled=[bool]$task.Settings.Enabled;running=([string]$task.State -eq 'Running');xml=[string]$xml} | ConvertTo-Json -Compress
`, powershellLiteral(windowsTaskPath()), powershellLiteral(opts.Name), powershellLiteral(windowsTaskPath()), powershellLiteral(opts.Name))
	command := windowsPowerShellCommand(script)
	output, commandErr := run(ctx, command)
	if commandErr != nil {
		return windowsTaskState{}, appendOutput(nil, output), fmt.Errorf("%s: %w", shellLine(command), commandErr)
	}
	var wire struct {
		Schema  int    `json:"schema"`
		Exists  bool   `json:"exists"`
		Enabled bool   `json:"enabled"`
		Running bool   `json:"running"`
		XML     string `json:"xml"`
	}
	clean := strings.TrimPrefix(strings.TrimSpace(output), "\ufeff")
	if err := json.Unmarshal([]byte(clean), &wire); err != nil {
		return windowsTaskState{}, nil, fmt.Errorf("parse structured Windows task state: %w", err)
	}
	if wire.Schema != 1 {
		return windowsTaskState{}, nil, fmt.Errorf("parse structured Windows task state: schema=%d, want 1", wire.Schema)
	}
	if wire.Exists && strings.TrimSpace(wire.XML) == "" {
		return windowsTaskState{}, nil, errors.New("parse structured Windows task state: existing task has empty exported XML")
	}
	state := windowsTaskState{exists: wire.Exists, enabled: wire.Enabled, running: wire.Running, xml: wire.XML}
	return state, []string{fmt.Sprintf("Windows Scheduled Task state: exists=%t enabled=%t running=%t", state.exists, state.enabled, state.running)}, nil
}

func windowsPowerShellCommand(script string) Command {
	return Command{Name: "powershell.exe", Args: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", strings.TrimSpace(script)}}
}

func windowsEndTaskCommand(opts Options) Command {
	return Command{Name: "schtasks.exe", Args: []string{"/End", "/TN", windowsTaskPath() + opts.Name}}
}

func windowsDeleteTaskCommand(opts Options) Command {
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Module ScheduledTasks -ErrorAction Stop; $task=Get-ScheduledTask -TaskPath %s -TaskName %s -ErrorAction SilentlyContinue; if ($null -ne $task) { $task | Unregister-ScheduledTask -Confirm:$false -ErrorAction Stop }`, powershellLiteral(windowsTaskPath()), powershellLiteral(opts.Name))
	return windowsPowerShellCommand(script)
}

func windowsRegisterTaskCommand(opts Options, xml string) Command {
	xmlBase64 := base64.StdEncoding.EncodeToString([]byte(xml))
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Module ScheduledTasks -ErrorAction Stop; $xml=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String(%s)); Register-ScheduledTask -TaskPath %s -TaskName %s -Xml $xml -Force -ErrorAction Stop | Out-Null`, powershellLiteral(xmlBase64), powershellLiteral(windowsTaskPath()), powershellLiteral(opts.Name))
	return windowsPowerShellCommand(script)
}

func windowsSetTaskEnabledCommand(opts Options, enabled bool) Command {
	verb := "Disable-ScheduledTask"
	if enabled {
		verb = "Enable-ScheduledTask"
	}
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Module ScheduledTasks -ErrorAction Stop; Get-ScheduledTask -TaskPath %s -TaskName %s -ErrorAction Stop | %s -ErrorAction Stop | Out-Null`, powershellLiteral(windowsTaskPath()), powershellLiteral(opts.Name), verb)
	return windowsPowerShellCommand(script)
}

func windowsSetTaskRunningCommand(opts Options, running bool) Command {
	verb := "Stop-ScheduledTask"
	if running {
		verb = "Start-ScheduledTask"
	}
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Module ScheduledTasks -ErrorAction Stop; Get-ScheduledTask -TaskPath %s -TaskName %s -ErrorAction Stop | %s -ErrorAction Stop`, powershellLiteral(windowsTaskPath()), powershellLiteral(opts.Name), verb)
	return windowsPowerShellCommand(script)
}

func powershellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
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
