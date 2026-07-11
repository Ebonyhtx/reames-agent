package cli

import "reames-agent/internal/control"

func cliRuntimeStatus(ctrl control.CommandControl) control.RuntimeStatus {
	if ctrl == nil {
		return control.RuntimeStatus{}
	}
	result, err := ctrl.ExecuteCommand(control.NewStatusCommand(), control.CommandScopeTrusted)
	if err != nil {
		return control.RuntimeStatus{}
	}
	return result.Status
}

func cliCancel(ctrl control.CommandControl) {
	if ctrl == nil {
		return
	}
	_, _ = ctrl.ExecuteCommand(control.NewCancelCommand(), control.CommandScopeTrusted)
}

func cliApprove(ctrl control.CommandControl, id string, allow, session, persist bool) {
	if ctrl == nil {
		return
	}
	_, _ = ctrl.ExecuteCommand(control.NewApprovalCommand(id, allow, session, persist), control.CommandScopeTrusted)
}
