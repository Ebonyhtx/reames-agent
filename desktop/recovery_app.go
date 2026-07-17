package main

import (
	"fmt"
	"os"

	"reames-agent/internal/config"
	"reames-agent/internal/repair"
)

// RunRecoveryAction exposes the bounded recovery control plane to Wails. Safe
// Mode has no Controller by design, so it calls the same repair package directly
// instead of constructing a partial Agent runtime.
func (a *App) RunRecoveryAction(req repair.ActionRequest) (repair.ActionResult, error) {
	display := a.recoveryDisplayOptions()
	var (
		result repair.ActionResult
		err    error
	)
	if ctrl := a.activeCtrl(); ctrl != nil {
		result, err = ctrl.RunRecoveryAction(req)
	} else {
		result, err = repair.ExecuteAction(req, repair.ActionOptions{
			Root:           display.Root,
			ExecutablePath: display.ExecutablePath,
		})
	}
	if err != nil {
		return repair.ActionResult{}, fmt.Errorf("recovery action: %s", repair.RedactTextForDisplay(err.Error(), display))
	}
	result.Affected = redactRecoveryStrings(result.Affected, display)
	result.Report = repair.RedactReportForDisplay(result.Report, display)
	return result, nil
}

func (a *App) recoveryDisplayOptions() repair.DisplayOptions {
	executable, _ := os.Executable()
	root := ""
	if !config.SafeModeRequested() {
		root = a.activeWorkspaceRoot()
	}
	return repair.DisplayOptions{Root: root, ExecutablePath: executable}
}

func redactRecoveryStrings(values []string, opts repair.DisplayOptions) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = repair.RedactTextForDisplay(value, opts)
	}
	return out
}
