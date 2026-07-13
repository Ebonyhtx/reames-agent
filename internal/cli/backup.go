package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"reames-agent/internal/config"
	"reames-agent/internal/homebackup"
	"reames-agent/internal/i18n"
)

func backupCommand(args []string, version string) int {
	if len(args) == 0 {
		backupUsage()
		return 2
	}
	switch args[0] {
	case "create":
		return backupCreateCommand(args[1:], version)
	case "verify", "inspect":
		return backupVerifyCommand(args[1:])
	case "restore":
		return backupRestoreCommand(args[1:])
	default:
		backupUsage()
		return 2
	}
}

func backupCreateCommand(args []string, version string) int {
	fs := flag.NewFlagSet("backup create", flag.ContinueOnError)
	out := fs.String("out", "", "new backup archive path")
	home := fs.String("home", "", "Reames Agent home root (default: current configured home)")
	stateHome := fs.String("state-home", "", "separate state root (default: current configured state root)")
	offline := fs.Bool("offline", false, "confirm all CLI, Desktop, and Gateway writers are stopped")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 0 || strings.TrimSpace(*out) == "" {
		backupUsage()
		return 2
	}
	if !*offline {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "backup create requires --offline after stopping every Reames Agent process, including CLI, Desktop, serve, bot, Gateway, and cron workers")
		return 2
	}
	homePath := strings.TrimSpace(*home)
	homeExplicit := false
	stateExplicit := false
	fs.Visit(func(f *flag.Flag) {
		homeExplicit = homeExplicit || f.Name == "home"
		stateExplicit = stateExplicit || f.Name == "state-home"
	})
	if homeExplicit && homePath == "" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "backup create --home must not be empty")
		return 2
	}
	if homePath == "" {
		homePath = config.ReamesAgentHomeDir()
	}
	statePath := strings.TrimSpace(*stateHome)
	if stateExplicit && statePath == "" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "backup create --state-home must not be empty")
		return 2
	}
	if homeExplicit && !stateExplicit {
		statePath = homePath
	} else if statePath == "" {
		statePath = config.MemoryUserDir()
	}
	if homePath == "" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "cannot resolve Reames Agent home")
		return 1
	}
	roots := []homebackup.Root{{ID: "home", Path: homePath}}
	if statePath != "" && !sameBackupPath(homePath, statePath) {
		roots = append(roots, homebackup.Root{ID: "state", Path: statePath})
	}
	summary, err := homebackup.Create(homebackup.CreateOptions{
		Roots:            roots,
		Destination:      *out,
		CreatedByVersion: version,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	abs, _ := filepath.Abs(*out)
	fmt.Printf("backup created: %s\nfiles: %d directories: %d bytes: %d\nsha256: %s\n", abs, summary.Files, summary.Directories, summary.Bytes, summary.ArchiveSHA256)
	printBackupSensitivityWarning()
	return 0
}

func backupVerifyCommand(args []string) int {
	fs := flag.NewFlagSet("backup verify", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 1 {
		backupUsage()
		return 2
	}
	summary, err := homebackup.Verify(fs.Args()[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	fmt.Printf("backup verified: files=%d directories=%d bytes=%d\nsha256: %s\n", summary.Files, summary.Directories, summary.Bytes, summary.ArchiveSHA256)
	fmt.Println("integrity: embedded manifest is self-consistent; compare sha256 with a separately trusted record before restore")
	printBackupSensitivityWarning()
	return 0
}

func backupRestoreCommand(args []string) int {
	fs := flag.NewFlagSet("backup restore", flag.ContinueOnError)
	home := fs.String("home", "", "new Reames Agent home target")
	stateHome := fs.String("state-home", "", "new separate state target when the archive contains one")
	dryRun := fs.Bool("dry-run", false, "verify archive and targets without writing")
	offline := fs.Bool("offline", false, "confirm all CLI, Desktop, and Gateway writers are stopped")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) != 1 {
		backupUsage()
		return 2
	}
	if !*dryRun && !*offline {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "backup restore requires --offline after stopping every Reames Agent process, including CLI, Desktop, serve, bot, Gateway, and cron workers")
		return 2
	}
	manifest, err := homebackup.ReadManifest(fs.Args()[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	homeTarget := strings.TrimSpace(*home)
	if homeTarget == "" {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "backup restore requires --home NEW_PATH; in-place restore is not supported")
		return 2
	}
	targets := map[string]string{"home": homeTarget}
	for _, root := range manifest.Roots {
		if root.ID != "state" {
			continue
		}
		stateTarget := strings.TrimSpace(*stateHome)
		if stateTarget == "" {
			fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, "this split-root backup requires --state-home NEW_PATH")
			return 2
		}
		targets["state"] = stateTarget
	}
	summary, err := homebackup.Restore(homebackup.RestoreOptions{
		Archive: fs.Args()[0],
		Targets: targets,
		DryRun:  *dryRun,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.M.ErrorPrefix, err)
		return 1
	}
	verb := "restore plan verified"
	if !*dryRun {
		verb = "backup restored"
	}
	fmt.Printf("%s: files=%d directories=%d bytes=%d\nsha256: %s\n", verb, summary.Files, summary.Directories, summary.Bytes, summary.ArchiveSHA256)
	if !*dryRun {
		fmt.Println("known credential stores were not restored; reconfigure provider and channel credentials before starting Gateway")
	}
	return 0
}

func sameBackupPath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	aa = filepath.Clean(aa)
	bb = filepath.Clean(bb)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}

func printBackupSensitivityWarning() {
	fmt.Println("sensitive: known credential stores are excluded, but conversations, memory, and custom config may still contain secrets; keep the archive private (Unix mode is restricted; Windows protection also depends on the destination directory ACL)")
}

func backupUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  reames-agent backup create --offline --out FILE [--home PATH] [--state-home PATH]
  reames-agent backup verify FILE
  reames-agent backup restore [--dry-run | --offline] --home NEW_PATH [--state-home NEW_PATH] FILE`)
}
