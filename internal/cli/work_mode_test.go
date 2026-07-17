package cli

import (
	"errors"
	"testing"

	"reames-agent/internal/boot"
	"reames-agent/internal/control"
)

func TestParseCLIWorkMode(t *testing.T) {
	for _, tc := range []struct {
		in       string
		want     string
		explicit bool
		valid    bool
	}{
		{"", boot.TokenModeFull, false, true},
		{"economy", boot.TokenModeEconomy, true, true},
		{"balanced", boot.TokenModeFull, true, true},
		{"delivery", boot.TokenModeDelivery, true, true},
		{"turbo", "", true, false},
	} {
		got, explicit, err := parseCLIWorkMode(tc.in)
		if (err == nil) != tc.valid || got != tc.want || explicit != tc.explicit {
			t.Fatalf("parseCLIWorkMode(%q) = %q, %v, %v; want %q, %v, valid=%v", tc.in, got, explicit, err, tc.want, tc.explicit, tc.valid)
		}
	}
}

func TestWorkModeForResumePathInheritsPersistedDelivery(t *testing.T) {
	path := t.TempDir() + "/session.jsonl"
	if err := persistCLIWorkMode(path, boot.TokenModeDelivery); err != nil {
		t.Fatalf("persistCLIWorkMode: %v", err)
	}
	if got := workModeForResumePath(boot.TokenModeFull, false, path); got != boot.TokenModeDelivery {
		t.Fatalf("inherited work mode = %q, want delivery", got)
	}
	if got := workModeForResumePath(boot.TokenModeEconomy, true, path); got != boot.TokenModeEconomy {
		t.Fatalf("explicit work mode = %q, want economy", got)
	}
}

func TestWorkModeCommandRebuildsAtomicallyAndPreservesApproval(t *testing.T) {
	oldCtrl := control.New(control.Options{Label: "model"})
	oldCtrl.SetAutoApproveTools(true)
	m := newTestChatTUI()
	m.ctrl = oldCtrl
	m.modelRef = "provider/model"
	m.workMode = "balanced"
	internalMode := boot.TokenModeFull
	m.workModeState = &internalMode
	m.buildWorkModeController = func(ref, mode string, _ control.SessionHistorySnapshot, _ string) (*control.Controller, error) {
		if ref != "provider/model" || mode != boot.TokenModeDelivery {
			t.Fatalf("build args = ref:%q mode:%q", ref, mode)
		}
		return control.New(control.Options{Label: "model"}), nil
	}

	cmd := m.runWorkModeCommand("/work-mode delivery")
	if cmd == nil {
		t.Fatal("delivery switch should schedule a rebuild")
	}
	msg := cmd()
	switchMsg, ok := msg.(modelSwitchMsg)
	if !ok {
		t.Fatalf("switch message = %T, want modelSwitchMsg", msg)
	}
	if switchMsg.ctrl == nil || !switchMsg.ctrl.AutoApproveTools() {
		t.Fatal("rebuilt controller did not preserve auto-approval state")
	}
	updated, _ := m.Update(msg)
	got := updated.(chatTUI)
	if got.ctrl == oldCtrl || got.workMode != "delivery" || internalMode != boot.TokenModeDelivery {
		t.Fatalf("updated work mode = %q internal=%q ctrlChanged=%v", got.workMode, internalMode, got.ctrl != oldCtrl)
	}
}

func TestWorkModeCommandBuildFailureKeepsCurrentController(t *testing.T) {
	oldCtrl := control.New(control.Options{Label: "model"})
	m := newTestChatTUI()
	m.ctrl = oldCtrl
	m.modelRef = "provider/model"
	m.workMode = "balanced"
	m.buildWorkModeController = func(string, string, control.SessionHistorySnapshot, string) (*control.Controller, error) {
		return nil, errors.New("build failed")
	}

	cmd := m.runWorkModeCommand("/work-mode delivery")
	if cmd == nil {
		t.Fatal("delivery switch should schedule a rebuild")
	}
	updated, _ := m.Update(cmd())
	got := updated.(chatTUI)
	if got.ctrl != oldCtrl || got.workMode != "balanced" {
		t.Fatalf("failed switch changed state: ctrlChanged=%v workMode=%q", got.ctrl != oldCtrl, got.workMode)
	}
}
