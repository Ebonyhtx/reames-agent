package agent

import (
	"context"
	"encoding/json"
	"reames-agent/internal/event"
	"strings"
	"testing"

	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

// stubGate denies any call whose tool name is in deny; everything else allows.
type stubGate struct {
	deny    map[string]bool
	checked []string
}

type structuredApprovalTool struct {
	executions int
	previews   int
}

func (*structuredApprovalTool) Name() string            { return "sensitive_tool" }
func (*structuredApprovalTool) Description() string     { return "test sensitive tool" }
func (*structuredApprovalTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (*structuredApprovalTool) ReadOnly() bool          { return false }
func (*structuredApprovalTool) InvocationReadOnly(args json.RawMessage) bool {
	var in struct {
		Apply bool `json:"apply"`
	}
	return json.Unmarshal(args, &in) == nil && !in.Apply
}
func (t *structuredApprovalTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.executions++
	return "applied", nil
}
func (t *structuredApprovalTool) PreviewApproval(_ context.Context, args json.RawMessage) (tool.ApprovalPlan, bool, error) {
	t.previews++
	var in struct {
		Apply bool `json:"apply"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return tool.ApprovalPlan{}, true, err
	}
	if !in.Apply {
		return tool.ApprovalPlan{}, false, nil
	}
	return tool.ApprovalPlan{
		PlanID: "plan-1", Operation: "install", Scope: "project",
		Actions: []tool.ApprovalAction{{Kind: "skill", Action: "copy_skill", RiskLevel: "medium", Name: "demo", Target: "/repo/.agents/demo"}},
	}, true, nil
}

type preflightDenyGate struct{ structuredStubGate }

func (*preflightDenyGate) CheckStructuredApprovalPreflight(context.Context, string, json.RawMessage) (bool, string, error) {
	return false, "denied before preview", nil
}

type structuredStubGate struct {
	ordinaryCalls   int
	structuredCalls int
	plan            tool.ApprovalPlan
	allow           bool
}

func (g *structuredStubGate) Check(context.Context, string, json.RawMessage, bool) (bool, string, error) {
	g.ordinaryCalls++
	return true, "", nil
}
func (g *structuredStubGate) CheckStructuredApproval(_ context.Context, _ string, _ json.RawMessage, plan tool.ApprovalPlan) (bool, string, error) {
	g.structuredCalls++
	g.plan = plan
	if !g.allow {
		return false, "declined", nil
	}
	return true, "", nil
}

func (g *stubGate) Check(ctx context.Context, toolName string, args json.RawMessage, readOnly bool) (bool, string, error) {
	g.checked = append(g.checked, toolName)
	if g.deny[toolName] {
		return false, "denied by test policy", nil
	}
	return true, "", nil
}

// TestGateBlocksDeniedCall proves executeOne consults the gate after the
// plan-mode check: a denied tool returns a "blocked:" result plus a notice and
// never runs, while an allowed tool runs normally.
func TestGateBlocksDeniedCall(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "bash", readOnly: false})
	reg.Add(fakeTool{name: "read_file", readOnly: true})

	g := &stubGate{deny: map[string]bool{"bash": true}}
	a := New(nil, reg, NewSession(""), Options{Gate: g}, event.Discard)

	blocked := a.executeOne(context.Background(), provider.ToolCall{Name: "bash", Arguments: `{"command":"rm -rf /"}`})
	if !strings.HasPrefix(blocked.output, "blocked:") {
		t.Errorf("denied call result = %q, want a 'blocked:' result", blocked.output)
	}
	if !blocked.blocked || blocked.errMsg == "" {
		t.Errorf("denied call should surface a user-facing block notice, got %+v", blocked)
	}

	ok := a.executeOne(context.Background(), provider.ToolCall{Name: "read_file", Arguments: `{"path":"/a"}`})
	if !strings.Contains(ok.output, "done") {
		t.Errorf("allowed call should run, got %q", ok.output)
	}

	if len(g.checked) != 2 {
		t.Errorf("gate consulted %d times, want 2 (%v)", len(g.checked), g.checked)
	}
}

// TestNilGateRunsEverything confirms gating is opt-in: with no gate wired, a
// writer call runs unimpeded (backward-compatible default).
func TestNilGateRunsEverything(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "write_file", readOnly: false})

	a := New(nil, reg, NewSession(""), Options{}, event.Discard) // no Gate
	out := a.executeOne(context.Background(), provider.ToolCall{Name: "write_file", Arguments: `{"path":"/a"}`})
	if strings.HasPrefix(out.output, "blocked:") {
		t.Errorf("nil gate should not block: %q", out.output)
	}
}

func TestStructuredApprovalIsFailClosedAndSkipsOrdinaryGate(t *testing.T) {
	t.Run("deny rule blocks before preview", func(t *testing.T) {
		reg := tool.NewRegistry()
		tl := &structuredApprovalTool{}
		reg.Add(tl)
		a := New(nil, reg, NewSession(""), Options{Gate: &preflightDenyGate{}}, event.Discard)

		out := a.executeOne(context.Background(), provider.ToolCall{Name: tl.Name(), Arguments: `{"apply":true}`})
		if !out.blocked || !strings.Contains(out.output, "denied before preview") || tl.previews != 0 || tl.executions != 0 {
			t.Fatalf("preflight outcome = %+v, previews=%d executions=%d", out, tl.previews, tl.executions)
		}
	})

	t.Run("host without structured approval", func(t *testing.T) {
		reg := tool.NewRegistry()
		tl := &structuredApprovalTool{}
		reg.Add(tl)
		a := New(nil, reg, NewSession(""), Options{Gate: &stubGate{}}, event.Discard)

		out := a.executeOne(context.Background(), provider.ToolCall{Name: tl.Name(), Arguments: `{"apply":true}`})
		if !out.blocked || !strings.Contains(out.output, "structured human approval") || tl.previews != 0 || tl.executions != 0 {
			t.Fatalf("unsupported host outcome = %+v, previews=%d executions=%d", out, tl.previews, tl.executions)
		}
	})

	t.Run("exact plan approved", func(t *testing.T) {
		reg := tool.NewRegistry()
		tl := &structuredApprovalTool{}
		reg.Add(tl)
		gate := &structuredStubGate{allow: true}
		a := New(nil, reg, NewSession(""), Options{Gate: gate}, event.Discard)

		out := a.executeOne(context.Background(), provider.ToolCall{Name: tl.Name(), Arguments: `{"apply":true}`})
		if out.blocked || out.output != "applied" || tl.executions != 1 {
			t.Fatalf("approved outcome = %+v, executions = %d", out, tl.executions)
		}
		if gate.structuredCalls != 1 || gate.ordinaryCalls != 0 || gate.plan.PlanID != "plan-1" || len(gate.plan.Actions) != 1 {
			t.Fatalf("gate calls ordinary=%d structured=%d plan=%+v", gate.ordinaryCalls, gate.structuredCalls, gate.plan)
		}
	})

	t.Run("planning remains ordinary read-only", func(t *testing.T) {
		reg := tool.NewRegistry()
		tl := &structuredApprovalTool{}
		reg.Add(tl)
		gate := &structuredStubGate{allow: true}
		a := New(nil, reg, NewSession(""), Options{Gate: gate}, event.Discard)

		out := a.executeOne(context.Background(), provider.ToolCall{Name: tl.Name(), Arguments: `{"apply":false}`})
		if out.blocked || gate.ordinaryCalls != 1 || gate.structuredCalls != 0 || tl.executions != 1 {
			t.Fatalf("planning outcome = %+v, gate=%+v, executions=%d", out, gate, tl.executions)
		}
	})
}
