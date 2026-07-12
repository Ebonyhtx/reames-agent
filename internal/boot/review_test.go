package boot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reames-agent/internal/config"
	"reames-agent/internal/skill"
)

func TestReviewSubagentRegistryUsesForegroundOnlyBash(t *testing.T) {
	reg := reviewSubagentRegistry(skill.Skill{AllowedTools: []string{
		"bash",
		"wait",
		"bash_output",
		"kill_shell",
		"task",
	}}, config.Default())

	for _, hidden := range []string{"wait", "bash_output", "kill_shell", "task"} {
		if _, ok := reg.Get(hidden); ok {
			t.Fatalf("review subagent registry should hide %q; got %v", hidden, reg.Names())
		}
	}
	bash, ok := reg.Get("bash")
	if !ok {
		t.Fatalf("review subagent registry should keep bash; got %v", reg.Names())
	}
	if strings.Contains(string(bash.Schema()), "run_in_background") {
		t.Fatalf("review subagent bash schema should not include run_in_background: %s", bash.Schema())
	}
	if _, err := bash.Execute(context.Background(), json.RawMessage(`{"command":"sleep 1","run_in_background":true}`)); err == nil || !strings.Contains(err.Error(), "background bash is unavailable in subagents") {
		t.Fatalf("review subagent background bash should return a clear error, got %v", err)
	}
}
