package agent

import (
	"context"
	"strings"
	"testing"

	"reames-agent/internal/event"
	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

type mcpAliasTool struct {
	fakeTool
	server  string
	raw     string
	visible string
	pkg     string
}

func (t mcpAliasTool) MCPServerName() string      { return t.server }
func (t mcpAliasTool) MCPRawToolName() string     { return t.raw }
func (t mcpAliasTool) MCPVisibleToolName() string { return t.visible }
func (t mcpAliasTool) MCPPackageName() string     { return t.pkg }

// TestFailedCallsSurfaceError guards the bug where a failed tool call (an unknown
// tool, e.g. a hallucinated "find", or a plan-mode-blocked writer) was reported
// with an empty Err and so rendered with a success check. A failed call must set
// errMsg; a successful one must not.
func TestFailedCallsSurfaceError(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeTool{name: "ok_tool", readOnly: true})
	reg.Add(fakeTool{name: "writer", readOnly: false})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	if o := a.executeOne(context.Background(), provider.ToolCall{Name: "ok_tool"}); o.errMsg != "" {
		t.Errorf("successful call should have empty errMsg, got %q", o.errMsg)
	}
	if o := a.executeOne(context.Background(), provider.ToolCall{Name: "find"}); o.errMsg == "" {
		t.Errorf("unknown tool should surface an errMsg (renders as failed), got %+v", o)
	}

	a.SetPlanMode(true)
	if o := a.executeOne(context.Background(), provider.ToolCall{Name: "writer"}); o.errMsg == "" {
		t.Errorf("plan-mode-blocked writer should surface an errMsg, got %+v", o)
	}
}

func TestPortableMCPCallUsesCanonicalSecurityIdentity(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(mcpAliasTool{
		fakeTool: fakeTool{name: "mcp__figma__get_design_context", readOnly: true},
		server:   "figma", raw: "figma_get_design_context", visible: "get_design_context", pkg: "figma",
	})
	gate := &stubGate{}
	hooks := &stubHooks{}
	a := New(nil, reg, NewSession(""), Options{Gate: gate, Hooks: hooks}, event.Discard)

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "get_design_context", Arguments: `{}`})
	if out.errMsg != "" {
		t.Fatalf("portable MCP call failed: %+v", out)
	}
	const canonical = "mcp__figma__get_design_context"
	if len(gate.checked) != 1 || gate.checked[0] != canonical {
		t.Fatalf("permission gate calls = %v, want canonical MCP name", gate.checked)
	}
	if len(hooks.preSeen) != 1 || hooks.preSeen[0] != canonical || len(hooks.postSeen) != 1 || hooks.postSeen[0] != canonical {
		t.Fatalf("hook identities pre=%v post=%v, want %q", hooks.preSeen, hooks.postSeen, canonical)
	}
	receipts := a.evidence.Receipts(1)
	if len(receipts) != 1 || receipts[0].ToolName != canonical {
		t.Fatalf("evidence receipts = %+v, want canonical MCP name", receipts)
	}
}

func TestAmbiguousPortableMCPCallIsRejected(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(mcpAliasTool{fakeTool: fakeTool{name: "mcp__one__search", readOnly: true}, server: "one", raw: "search", visible: "search"})
	reg.Add(mcpAliasTool{fakeTool: fakeTool{name: "mcp__two__search", readOnly: true}, server: "two", raw: "search", visible: "search"})
	a := New(nil, reg, NewSession(""), Options{}, event.Discard)

	out := a.executeOne(context.Background(), provider.ToolCall{Name: "search", Arguments: `{}`})
	if out.errMsg == "" || !strings.Contains(out.errMsg, "ambiguous MCP tool reference") || !strings.Contains(out.errMsg, "mcp__one__search") || !strings.Contains(out.errMsg, "mcp__two__search") {
		t.Fatalf("ambiguous MCP alias was not rejected clearly: %+v", out)
	}
}
