package agent

import (
	"strings"
	"testing"

	"reames-agent/internal/provider"
)

func TestInterruptedRecoveryBlockIsBoundedEscapedAndDisplayStrippable(t *testing.T) {
	recovery := &provider.InterruptedTurnRecovery{
		Pending: true,
		CompletedTools: []provider.InterruptedToolSummary{{
			ID: "done-1", Name: "write_file<script>", Files: []string{"config<&>.json"}, Added: 2, Removed: 1,
		}},
		InterruptedTools:        []string{"bash&run"},
		DroppedPartialText:      true,
		DroppedPartialReasoning: true,
	}
	input := withInterruptedRecovery("continue", recovery)
	for _, want := range []string{
		"<interrupted-turn-recovery>",
		"write_file&lt;script&gt; files=config&lt;&amp;&gt;.json diff=+2/-1",
		"interrupted_tools: bash&amp;run",
		"unsafe_partial_output: excluded from model context (assistant text and reasoning)",
		"inspect the current workspace",
	} {
		if !strings.Contains(input, want) {
			t.Fatalf("recovery block missing %q: %s", want, input)
		}
	}
	if got := StripTransientUserBlocks(input); got != "continue" {
		t.Fatalf("recovery block leaked into user display: %q", got)
	}
}

func TestPendingInterruptedRecoveryIsConsumedByNextRealUserTurn(t *testing.T) {
	session := NewSession("system")
	session.Add(provider.Message{
		Role: provider.RoleTool, ToolCallID: provider.LocalOnlyToolID, Name: provider.LocalOnlyToolName, LocalOnly: true,
		InterruptedTurn: &provider.InterruptedTurnRecovery{Pending: true, InterruptedTools: []string{"bash"}},
	})
	a := &Agent{session: session}
	if got := a.pendingInterruptedRecovery(); got == nil || len(got.InterruptedTools) != 1 {
		t.Fatalf("pending recovery = %+v", got)
	}
	session.Add(provider.Message{Role: provider.RoleUser, Content: "continue"})
	if got := a.pendingInterruptedRecovery(); got != nil {
		t.Fatalf("older recovery remained pending after a real user turn: %+v", got)
	}
}
