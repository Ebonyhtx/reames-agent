// Package guardcmd implements the credential-free offline recovery command.
// It intentionally imports repair/config state only, never boot, control,
// provider, plugin hosts, hooks, or the Agent runtime.
package guardcmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"reames-agent/internal/repair"
)

type streams struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

func Run(args []string, version string, in io.Reader, out, stderr io.Writer) int {
	s := streams{in: in, out: out, err: stderr}
	if len(args) == 0 {
		return runLaunch(nil, s)
	}
	switch args[0] {
	case "check", "diagnose":
		return runCheck(args[1:], s)
	case "repair":
		return runRepair(args[1:], s)
	case "launch":
		return runLaunch(args[1:], s)
	case "rollback":
		return runRollback(args[1:], s)
	case "snapshots":
		return runSnapshots(args[1:], s)
	case "restore":
		return runRestore(args[1:], s)
	case "undo":
		return runUndo(args[1:], s)
	case "rebuild":
		return runRebuild(args[1:], s)
	case "disable-plugins":
		return runDisablePlugins(args[1:], s)
	case "version", "--version", "-v":
		fmt.Fprintln(s.out, "reames-agent-guard", version)
		return 0
	case "help", "--help", "-h":
		usage(s.out)
		return 0
	default:
		fmt.Fprintln(s.err, "unknown guard command:", args[0])
		usage(s.err)
		return 2
	}
}

func runCheck(args []string, s streams) int {
	fs := flag.NewFlagSet("guard check", flag.ContinueOnError)
	fs.SetOutput(s.err)
	root := fs.String("root", ".", "project root to inspect")
	app := fs.String("app", "", "installed desktop executable to inspect")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	target := desktopInspectPath(*app)
	report, err := repair.Inspect(repair.InspectOptions{Root: *root, ExecutablePath: target})
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	if *jsonOut {
		if code := printJSON(s.out, report); code != 0 {
			return code
		}
		if report.HasErrors() {
			return 1
		}
		return 0
	}
	fmt.Fprintln(s.out, "Reames Agent Guard")
	fmt.Fprintf(s.out, "  startup: %-10s failures=%d safe-mode=%v\n", report.Startup.Phase, report.Startup.ConsecutiveFailures, report.SafeModeRecommended)
	for _, check := range report.Config.Checks {
		status := "ok"
		if !check.Exists {
			status = "missing"
		} else if !check.Valid {
			status = "invalid"
		}
		fmt.Fprintf(s.out, "  config:  %-8s %-7s %s\n", check.Scope, status, check.Path)
	}
	for _, binary := range report.Binaries {
		status := "missing"
		if binary.Exists && binary.Regular && binary.Error == "" {
			status = "ok"
		} else if binary.Exists {
			status = "invalid"
		}
		fmt.Fprintf(s.out, "  binary:  %-8s %-7s %s\n", binary.Role, status, binary.Path)
	}
	for _, finding := range report.Findings {
		fmt.Fprintf(s.out, "  %-7s %-28s %s\n", finding.Severity, finding.Code, finding.Message)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

func runRepair(args []string, s streams) int {
	fs := flag.NewFlagSet("guard repair", flag.ContinueOnError)
	fs.SetOutput(s.err)
	root := fs.String("root", ".", "project root to inspect")
	project := fs.Bool("project", false, "allow project reames-agent.toml quarantine")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	report, err := repair.InspectAndRepairConfig(repair.ConfigOptions{Root: *root, Apply: true, IncludeProject: *project})
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	if *jsonOut {
		return printJSON(s.out, report)
	}
	for _, action := range report.Applied {
		fmt.Fprintln(s.out, action)
	}
	if len(report.Applied) == 0 {
		fmt.Fprintln(s.out, "no repair was required")
	}
	return 0
}

func runRollback(args []string, s streams) int {
	fs := flag.NewFlagSet("guard rollback", flag.ContinueOnError)
	fs.SetOutput(s.err)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	result, err := repair.RollbackPendingUpdate()
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		if result.MixedInstall {
			fmt.Fprintln(s.err, "refusing launch: the installed release unit may be mixed; reinstall a verified release")
		}
		return 1
	}
	if *jsonOut {
		return printJSON(s.out, result)
	}
	if !result.RolledBack {
		fmt.Fprintln(s.out, "no pending update to roll back")
		return 0
	}
	fmt.Fprintf(s.out, "restored %s from failed %s update\n", result.ToVersion, result.FromVersion)
	return 0
}

func runSnapshots(args []string, s streams) int {
	fs := flag.NewFlagSet("guard snapshots", flag.ContinueOnError)
	fs.SetOutput(s.err)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	snapshots, err := repair.ListConfigSnapshots()
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	if *jsonOut {
		return printJSON(s.out, snapshots)
	}
	for _, snapshot := range snapshots {
		fmt.Fprintf(s.out, "%s  %s  %s\n", snapshot.ID, snapshot.Version, snapshot.RecordedAt)
	}
	return 0
}

func runRestore(args []string, s streams) int {
	fs := flag.NewFlagSet("guard restore", flag.ContinueOnError)
	fs.SetOutput(s.err)
	snapshot := fs.String("snapshot", "", "snapshot id to restore")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || strings.TrimSpace(*snapshot) == "" {
		return 2
	}
	tx, err := repair.RestoreConfigSnapshot(*snapshot)
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	fmt.Fprintln(s.out, "restored config snapshot; undo transaction:", tx.ID)
	return 0
}

func runUndo(args []string, s streams) int {
	fs := flag.NewFlagSet("guard undo", flag.ContinueOnError)
	fs.SetOutput(s.err)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	tx, err := repair.UndoLastRepair()
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	if *jsonOut {
		return printJSON(s.out, tx)
	}
	fmt.Fprintln(s.out, "undid repair:", tx.ID)
	return 0
}

func runRebuild(args []string, s streams) int {
	fs := flag.NewFlagSet("guard rebuild", flag.ContinueOnError)
	fs.SetOutput(s.err)
	target := fs.String("target", "", "tabs|projects|window|zoom|all")
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || strings.TrimSpace(*target) == "" {
		return 2
	}
	applied, err := repair.RebuildDerivedState(*target)
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	if *jsonOut {
		return printJSON(s.out, applied)
	}
	for _, path := range applied {
		fmt.Fprintln(s.out, "quarantined:", path)
	}
	return 0
}

func runDisablePlugins(args []string, s streams) int {
	fs := flag.NewFlagSet("guard disable-plugins", flag.ContinueOnError)
	fs.SetOutput(s.err)
	jsonOut := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	disabled, err := repair.DisableAllPlugins()
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	if *jsonOut {
		return printJSON(s.out, disabled)
	}
	if len(disabled) == 0 {
		fmt.Fprintln(s.out, "no enabled plugins")
		return 0
	}
	for _, name := range disabled {
		fmt.Fprintln(s.out, "disabled plugin:", name)
	}
	return 0
}

func runLaunch(args []string, s streams) int {
	fs := flag.NewFlagSet("guard launch", flag.ContinueOnError)
	fs.SetOutput(s.err)
	app := fs.String("app", "", "sibling desktop executable")
	safeMode := fs.Bool("safe-mode", false, "force Safe Mode")
	detach := fs.Bool("detach", packagedDetachedLauncher(), "start desktop and exit Guard")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if result, failure, err := repair.RecoverFailedInstall(); err != nil {
		fmt.Fprintln(s.err, "update rollback after installer failure failed:", err)
		if !result.RolledBack {
			fmt.Fprintln(s.err, "refusing launch: installer failure left the release unit unverified")
			return 1
		}
	} else if failure != nil && result.RolledBack {
		fmt.Fprintf(s.err, "Reames Agent Guard restored %s after the %s installer failed.\n", result.ToVersion, result.FromVersion)
	}
	path, err := resolveDesktopPath(*app)
	if err != nil {
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	tracker := repair.NewStartupTracker("")
	useSafeMode := *safeMode
	if !useSafeMode && tracker.SafeModeRecommended() {
		state, stateErr := tracker.Read()
		tx, txErr := repair.ReadPendingUpdate()
		if stateErr == nil && txErr == nil && strings.TrimSpace(state.Version) == strings.TrimSpace(tx.ToVersion) {
			result, rollbackErr := repair.RollbackPendingUpdateIfCurrent(tx.ToVersion, tx.CreatedAt)
			if rollbackErr != nil {
				fmt.Fprintln(s.err, "verified update rollback failed:", rollbackErr)
				if result.MixedInstall {
					fmt.Fprintln(s.err, "refusing launch: the installed release unit may be mixed")
					return 1
				}
				useSafeMode = true
			} else if result.RolledBack {
				fmt.Fprintf(s.err, "Reames Agent Guard restored %s after %s failed to start.\n", result.ToVersion, result.FromVersion)
				_ = tracker.MarkClean()
			} else {
				// The transaction changed after the initial attribution read. Never
				// launch normally from evidence that no longer identifies the same
				// installed release unit.
				useSafeMode = true
			}
		} else {
			// Ambiguous provenance/attribution never triggers a binary mutation.
			useSafeMode = true
		}
	}
	childArgs := append([]string(nil), fs.Args()...)
	cmd := exec.Command(path, childArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.in, s.out, s.err
	cmd.Env = os.Environ()
	if useSafeMode {
		cmd.Env = append(cmd.Env, "REAMES_AGENT_SAFE_MODE=1")
	}
	if *detach {
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(s.err, "error:", err)
			return 1
		}
		return 0
	}
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(s.err, "error:", err)
		return 1
	}
	return 0
}

func resolveDesktopPath(explicit string) (string, error) {
	guard, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(guard); err == nil {
		guard = resolved
	}
	dir := filepath.Dir(guard)
	path := strings.TrimSpace(explicit)
	if path == "" {
		name := "reames-agent-desktop"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path = filepath.Join(dir, name)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	base := strings.ToLower(filepath.Base(path))
	if base != "reames-agent-desktop" && base != "reames-agent-desktop.exe" {
		return "", fmt.Errorf("desktop target %q is not an allowed Reames Agent executable", path)
	}
	if filepath.Clean(filepath.Dir(path)) != filepath.Clean(dir) {
		return "", fmt.Errorf("desktop target is outside the current Guard installation")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("desktop target is not a regular file")
	}
	return path, nil
}

func desktopInspectPath(explicit string) string {
	guard, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(guard); err == nil {
		guard = resolved
	}
	path := strings.TrimSpace(explicit)
	if path == "" {
		name := "reames-agent-desktop"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path = filepath.Join(filepath.Dir(guard), name)
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(guard), path)
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func packagedDetachedLauncher() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	name := strings.ToLower(filepath.Base(exe))
	return name == "reames-agent-launcher.exe" || name == "reames agent.exe"
}

func printJSON(w io.Writer, value any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: reames-agent-guard <check|repair|launch|rollback|snapshots|restore|undo|rebuild|disable-plugins> [options]")
}
