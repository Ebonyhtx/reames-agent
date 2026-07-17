package cli

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reames-agent/internal/boot"
	"reames-agent/internal/control"
)

// runWorkModeCommand switches the orthogonal economy/balanced/delivery axis.
// It reuses the model-switch build/swap path so transcript, session path,
// lifecycle state and interactive approval remain attached to one session.
func (m *chatTUI) runWorkModeCommand(input string) tea.Cmd {
	args := tokenizeArgs(input)
	if len(args) < 2 {
		current := m.workMode
		if current == "" {
			current = boot.WorkModeBalanced
		}
		m.notice(fmt.Sprintf("work-mode: %s (options: economy|balanced|delivery)", current))
		return nil
	}
	if len(args) != 2 {
		m.notice("usage: /work-mode economy|balanced|delivery")
		return nil
	}
	mode, ok := boot.ParseWorkMode(args[1])
	if !ok {
		m.notice(fmt.Sprintf("work-mode %q must be economy, balanced, or delivery", args[1]))
		return nil
	}
	publicMode := boot.WorkModeName(mode)
	current := strings.TrimSpace(m.workMode)
	if current == "" {
		current = boot.WorkModeBalanced
	}
	if publicMode == current {
		m.notice("work-mode is already " + publicMode)
		return nil
	}
	if m.buildWorkModeController == nil {
		m.notice("work-mode switching is unavailable in this session")
		return nil
	}
	if m.modelSwitchPending {
		m.notice("wait for the current session rebuild before changing work-mode")
		return nil
	}
	status := cliRuntimeStatus(m.ctrl)
	switch {
	case status.PendingPrompt:
		m.notice("answer pending prompts before changing work-mode")
		return nil
	case status.Running:
		m.notice("finish or cancel the current turn before changing work-mode")
		return nil
	case status.BackgroundJobs > 0:
		m.notice("stop background jobs before changing work-mode")
		return nil
	}
	if err := m.ctrl.Snapshot(); err != nil {
		m.notice("work-mode: snapshot failed: " + err.Error())
		return nil
	}
	carried := control.CaptureSessionHistory(m.ctrl)
	prevPath := m.ctrl.SessionPath()
	ref := m.modelRef
	if strings.TrimSpace(ref) == "" {
		_, resolved, err := m.currentConfigProvider()
		if err != nil {
			m.notice("work-mode: " + err.Error())
			return nil
		}
		ref = resolved
	}
	oldCtrl := m.ctrl
	autoApprove := m.ctrl.AutoApproveTools()
	build := m.buildWorkModeController
	m.modelSwitchPending = true
	m.pendingModelSwitch = func() tea.Msg {
		c, err := build(ref, mode, carried, prevPath)
		if err != nil {
			return modelSwitchMsg{ref: ref, workMode: publicMode, errorPrefix: "work-mode", err: err}
		}
		c.SetAutoApproveTools(autoApprove)
		return modelSwitchMsg{
			ref:           ref,
			workMode:      publicMode,
			successNotice: "work-mode set to " + publicMode,
			errorPrefix:   "work-mode",
			ctrl:          c,
			oldCtrl:       oldCtrl,
			label:         c.Label(),
			commands:      c.Commands(),
			skills:        c.Skills(),
			host:          c.Host(),
		}
	}
	m.notice("switching work-mode to " + publicMode + "…")
	return m.pendingModelSwitch
}
