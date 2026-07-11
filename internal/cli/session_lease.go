package cli

import "reames-agent/internal/control"

// sessionLeaseResumeRefusal is the startup-time refusal for `reamesAgent
// [--resume|--continue]` and `reames-agent run --resume/--continue`: it names the
// holder and offers the two ways out (close the holder, or continue in a
// duplicated session via --copy).
func sessionLeaseResumeRefusal(err error) string {
	return control.SessionInUseMessage(err) +
		"; close the other Reames Agent window or process, or rerun with --copy to continue in a duplicated session"
}

// sessionLeaseHeldNotice is the in-TUI refusal for /resume and /switch, where
// exiting to rerun with --copy is not the natural move.
func sessionLeaseHeldNotice(err error) string {
	return control.SessionInUseMessage(err) + "; " + control.SessionLeaseCloseHint
}

// rebindSessionLease moves the chat TUI's session lease to path before the
// controller binds it for writing. A nil keeper (tests, persistence disabled)
// gates nothing. On error the keeper still guards the previous session.
func (m *chatTUI) rebindSessionLease(path string) error {
	if m.leases == nil {
		return nil
	}
	return m.leases.Rebind(path)
}

// restoreSessionLease re-points the lease at the controller's current session
// after a switch attempt moved it but the switch itself then failed.
// Best-effort: the old lease was released during the rebind, so in the
// (unlikely) case another runtime grabbed it in between this stays silent and
// the next write surfaces the conflict.
func (m *chatTUI) restoreSessionLease() {
	if m.leases == nil {
		return
	}
	_ = m.leases.Rebind(m.ctrl.SessionPath())
}

// followSessionLease re-points the TUI's session lease at the controller's
// current session file after an operation that rotated it to a fresh path
// (/new, /clear, /branch, fork). A fresh path cannot be held by anyone else,
// so failure is theoretical — but never silent.
func (m *chatTUI) followSessionLease() {
	if m.leases == nil {
		return
	}
	if err := m.leases.Rebind(m.ctrl.SessionPath()); err != nil {
		m.notice(sessionLeaseHeldNotice(err))
	}
}
