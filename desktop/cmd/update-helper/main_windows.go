//go:build windows

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"reames-agent/internal/repair"
)

const parentExitTimeout = 2 * time.Minute

type helperDependencies struct {
	logger          *log.Logger
	waitForExit     func(uint32, time.Duration) error
	setenv          func(string, string) error
	runInstaller    func(string, string) error
	markApplyFailed func(string, string) error
	startRelaunch   func(string, string) error
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return runWithDependencies(args, helperDependencies{
		logger:          newLogger(),
		waitForExit:     waitForProcessExit,
		setenv:          os.Setenv,
		runInstaller:    runInstaller,
		markApplyFailed: repair.MarkUpdateApplyFailed,
		startRelaunch:   startRelaunch,
	})
}

func runWithDependencies(args []string, deps helperDependencies) int {
	var parentPID uint
	var installer, installDir, relaunch, toVersion, stateHome string
	fs := flag.NewFlagSet("reamesAgent-update-helper", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.UintVar(&parentPID, "parent-pid", 0, "Reames Agent process id to wait for before installing")
	fs.StringVar(&installer, "installer", "", "verified NSIS installer path")
	fs.StringVar(&installDir, "install-dir", "", "Reames Agent installation directory")
	fs.StringVar(&relaunch, "relaunch", "", "Reames Agent executable to start after the installer succeeds")
	fs.StringVar(&toVersion, "to-version", "", "replacement release version")
	fs.StringVar(&stateHome, "state-home", "", "Reames Agent repair state directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	logger := deps.logger
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	if installer == "" {
		logger.Print("missing --installer")
		return 2
	}
	if parentPID != 0 {
		if err := deps.waitForExit(uint32(parentPID), parentExitTimeout); err != nil {
			logger.Printf("wait for parent process %d: %v", parentPID, err)
			return 1
		}
	}
	if stateHome != "" {
		if err := deps.setenv("REAMES_AGENT_STATE_HOME", stateHome); err != nil {
			logger.Printf("set repair state home: %v", err)
			return 1
		}
	}
	if err := deps.runInstaller(installer, installDir); err != nil {
		logger.Printf("run installer: %v", err)
		if markErr := deps.markApplyFailed(toVersion, err.Error()); markErr != nil {
			logger.Printf("record installer failure: %v", markErr)
			return 1
		}
		if relaunch != "" {
			if launchErr := deps.startRelaunch(relaunch, installDir); launchErr != nil {
				logger.Printf("relaunch Guard after installer failure: %v", launchErr)
			}
		}
		return 1
	}
	if relaunch != "" {
		if err := deps.startRelaunch(relaunch, installDir); err != nil {
			logger.Printf("relaunch: %v", err)
			return 1
		}
	}
	return 0
}

func newLogger() *log.Logger {
	dir, err := os.UserCacheDir()
	if err == nil {
		dir = filepath.Join(dir, "Reames Agent", "updates")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			if f, err := os.OpenFile(filepath.Join(dir, "update-helper.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				return log.New(f, "", log.LstdFlags)
			}
		}
	}
	return log.New(os.Stderr, "", log.LstdFlags)
}

func waitForProcessExit(pid uint32, timeout time.Duration) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(h)
	waitMS := uint32(timeout / time.Millisecond)
	result, err := windows.WaitForSingleObject(h, waitMS)
	if err != nil {
		return err
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return nil
	case uint32(windows.WAIT_TIMEOUT):
		return fmt.Errorf("timed out after %s", timeout)
	default:
		return fmt.Errorf("unexpected wait result %d", result)
	}
}

func runInstaller(installer, installDir string) error {
	cmd := exec.Command(installer)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: installerCommandLine(installer, installDir), HideWindow: true}
	return cmd.Run()
}

func startRelaunch(relaunch, installDir string) error {
	cmd := exec.Command(relaunch)
	cmd.Dir = installDir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
