//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const windowsUpdateHelperFileName = "reames-agent-update-helper.exe"

// installerCommand runs the NSIS updater, forcing $INSTDIR to dir via /D= so the
// update overwrites the current install in place. NSIS requires /D= to be the
// final, unquoted token taken verbatim to the end of the line, so the raw command
// line is set directly — exec.Command would quote a path containing spaces (e.g.
// C:\Users\Jane Doe\...) and NSIS would then mis-parse the target directory.
func installerCommand(name, dir string) *exec.Cmd {
	cmd := exec.Command(name)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: installerCommandLine(name, dir), HideWindow: true}
	return cmd
}

func startWindowsUpdateHandoff(installerPath, installDir, relaunchPath, toVersion, stateHome string) error {
	if err := startWindowsUpdateHelper(installerPath, installDir, relaunchPath, toVersion, stateHome); err != nil {
		return fmt.Errorf("start verified update helper: %w", err)
	}
	return nil
}

func startWindowsUpdateHelper(installerPath, installDir, relaunchPath, toVersion, stateHome string) error {
	if installDir == "" {
		return os.ErrNotExist
	}
	helperPath, err := prepareWindowsUpdateHelper(installDir)
	if err != nil {
		return err
	}
	cmd := exec.Command(helperPath, windowsUpdateHandoffArgs(os.Getpid(), installerPath, installDir, relaunchPath, toVersion, stateHome)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func prepareWindowsUpdateHelper(installDir string) (string, error) {
	src := filepath.Join(installDir, windowsUpdateHelperFileName)
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dir, err := updateCacheDir()
	if err != nil {
		return "", err
	}
	cleanupWindowsUpdateHelpers(dir)
	dst := filepath.Join(dir, "reamesAgent-update-helper-"+time.Now().UTC().Format("20060102150405.000000000")+".exe")
	if err := os.WriteFile(dst, data, 0o700); err != nil {
		return "", err
	}
	return dst, nil
}

func cleanupWindowsUpdateHelpers(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "reamesAgent-update-helper-*.exe"))
	if err != nil {
		return
	}
	for _, name := range matches {
		_ = os.Remove(name)
	}
}
