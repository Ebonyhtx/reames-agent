package control

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"reames-agent/internal/hook"
	"reames-agent/internal/skill"
	"reames-agent/internal/tool"
)

func TestRuntimeMutationGuardRejectsNewWorkAndActiveControllers(t *testing.T) {
	c := New(Options{})
	release, err := c.BeginRuntimeMutation()
	if err != nil {
		t.Fatalf("reserve idle runtime: %v", err)
	}
	if _, err := c.ExecuteCommand(NewSubmitCommand("blocked", "blocked", ""), CommandScopeTrusted); err == nil {
		t.Fatal("versioned submit started while runtime mutation was reserved")
	}
	c.RunShell("echo blocked")
	if c.Running() {
		t.Fatal("direct shell started while runtime mutation was reserved")
	}
	if err := c.beginRotation(); !errors.Is(err, errRotationInProgress) {
		t.Fatalf("rotation while runtime mutation reserved = %v, want rotation error", err)
	}
	release()
	release() // release is deliberately idempotent for deferred cleanup paths.

	releaseAgain, err := c.BeginRuntimeMutation()
	if err != nil {
		t.Fatalf("reserve runtime after release: %v", err)
	}
	releaseAgain()

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	if _, err := c.BeginRuntimeMutation(); !errors.Is(err, ErrRuntimeMutationBusy) {
		t.Fatalf("reserve active runtime = %v, want ErrRuntimeMutationBusy", err)
	}
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
}

type pluginRuntimeTool string

func (t pluginRuntimeTool) Name() string                                           { return string(t) }
func (t pluginRuntimeTool) Description() string                                    { return string(t) }
func (pluginRuntimeTool) Schema() json.RawMessage                                  { return json.RawMessage(`{"type":"object"}`) }
func (pluginRuntimeTool) Execute(context.Context, json.RawMessage) (string, error) { return "", nil }
func (pluginRuntimeTool) ReadOnly() bool                                           { return true }

func TestRevokePluginRuntimeDisablesOwnedHooksAndSkillEntryPoints(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	reg := tool.NewRegistry()
	for _, name := range []string{"bash", "install_skill", "run_skill", "read_only_skill", "read_skill", "explore", "research", "review", "security_review"} {
		reg.Add(pluginRuntimeTool(name))
	}
	reg.Add(pluginRuntimeTool("mcp__owned__connect"))
	reg.Add(pluginRuntimeTool("mcp__user__connect"))
	hooks := hook.NewRunner([]hook.ResolvedHook{
		{
			HookConfig: hook.HookConfig{Command: "plugin", Env: map[string]string{"REAMES_AGENT_PLUGIN_NAME": "fixture"}},
			Event:      hook.PostToolUse,
			Scope:      hook.ScopePlugin,
		},
		{HookConfig: hook.HookConfig{Command: "global"}, Event: hook.PostToolUse, Scope: hook.ScopeGlobal},
	}, t.TempDir(), nil, nil)
	c := New(Options{
		Registry:        reg,
		WorkspaceRoot:   t.TempDir(),
		Hooks:           hooks,
		Skills:          []skill.Skill{{Name: "fixture-skill", Description: "fixture"}},
		PluginMCPOwners: map[string]string{"owned": "fixture"},
	})

	warnings := c.RevokePluginRuntime("fixture")
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one runtime revocation warning", warnings)
	}
	if got := hooks.Hooks(); len(got) != 1 || got[0].Scope != hook.ScopeGlobal {
		t.Fatalf("hooks after revoke = %+v, want only global hook", got)
	}
	if got := c.Skills(); len(got) != 0 {
		t.Fatalf("skills after revoke = %+v, want paused", got)
	}
	for _, name := range []string{"run_skill", "read_only_skill", "read_skill", "explore", "research", "review", "security_review"} {
		if _, ok := reg.Get(name); ok {
			t.Fatalf("stale skill entry point %q remains registered", name)
		}
	}
	if _, ok := reg.Get("mcp__owned__connect"); ok {
		t.Fatal("plugin-owned MCP placeholder remains registered")
	}
	if _, ok := reg.Get("mcp__user__connect"); !ok {
		t.Fatal("same-session user MCP placeholder should remain registered")
	}
	for _, name := range []string{"bash", "install_skill", "slash_command"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("unrelated tool %q should remain registered", name)
		}
	}
	if got := c.RevokePluginRuntime("fixture"); len(got) != 0 {
		t.Fatalf("second revoke warnings = %v, want idempotent no-op", got)
	}
}

func TestRevokePluginRuntimePreservesUserMCPAfterSameNameTakeover(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(pluginRuntimeTool("mcp__shared__connect"))
	c := New(Options{
		Registry:        reg,
		PluginMCPOwners: map[string]string{"shared": "fixture"},
	})

	if !c.DisconnectMCPServer("shared") {
		t.Fatal("disconnecting the original plugin MCP placeholder failed")
	}
	// A later user-owned connection with the same name must not inherit the
	// package owner that belonged to the disconnected runtime.
	reg.Add(pluginRuntimeTool("mcp__shared__connect"))
	c.RevokePluginRuntime("fixture")
	if _, ok := reg.Get("mcp__shared__connect"); !ok {
		t.Fatal("plugin revocation removed a same-name user MCP takeover")
	}
}
