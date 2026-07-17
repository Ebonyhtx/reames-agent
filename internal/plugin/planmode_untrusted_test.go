package plugin

import "testing"

// remoteTool and lazyTool report PlanModeUntrustedReadOnly()==true for a
// declared external reader without receipt authority, even when ReadOnly()
// deliberately remains false for ordinary writer-posture approval.
func TestPluginToolsPlanModeUntrustedReadOnly(t *testing.T) {
	cases := []struct {
		name            string
		declared        bool
		readOnly        bool
		readOnlyTrusted bool
		destructive     bool
		want            bool
	}{
		{"declared reader without receipt stays untrusted", true, false, false, false, true},
		{"compatibility readOnlyHint only is untrusted", false, true, false, false, true},
		{"receipt-backed reader is trusted", true, true, true, false, false},
		{"opaque writer is not an untrusted reader", false, false, false, false, false},
		{"not read-only even if trusted-flagged", false, false, true, false, false},
		{"destructive hint is never a plan-mode reader", true, false, false, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := &remoteTool{declaredReadOnly: c.declared, readOnly: c.readOnly, readOnlyTrusted: c.readOnlyTrusted, destructive: c.destructive}
			if got := rt.PlanModeUntrustedReadOnly(); got != c.want {
				t.Errorf("remoteTool.PlanModeUntrustedReadOnly() = %v, want %v", got, c.want)
			}
			lt := &lazyTool{declaredReadOnly: c.declared, readOnly: c.readOnly, readOnlyTrusted: c.readOnlyTrusted, destructive: c.destructive}
			if got := lt.PlanModeUntrustedReadOnly(); got != c.want {
				t.Errorf("lazyTool.PlanModeUntrustedReadOnly() = %v, want %v", got, c.want)
			}
		})
	}
}
