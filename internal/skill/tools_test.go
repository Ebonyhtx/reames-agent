package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/event"
	"reames-agent/internal/tool"
)

func TestPluginSkillOwnershipAndCollisionAreFailClosed(t *testing.T) {
	pluginRoot := t.TempDir()
	writeSkill(t, pluginRoot, "helper/SKILL.md", "---\ndescription: helper\n---\nCall search.")

	owned := New(Options{HomeDir: t.TempDir(), CustomPaths: []string{pluginRoot}, PluginPaths: map[string][]string{pluginRoot: {"search-plugin"}}, DisableBuiltins: true})
	if skills := owned.List(); len(skills) != 1 || skills[0].Plugin != "search-plugin" {
		t.Fatalf("owned plugin skill = %+v", skills)
	}

	ambiguous := New(Options{HomeDir: t.TempDir(), CustomPaths: []string{pluginRoot}, PluginPaths: map[string][]string{pluginRoot: {"one", "two"}}, DisableBuiltins: true})
	if skills := ambiguous.List(); len(skills) != 1 || skills[0].Plugin != "" {
		t.Fatalf("ambiguous package root inherited an owner: %+v", skills)
	}

	plain := New(Options{HomeDir: t.TempDir(), CustomPaths: []string{pluginRoot}, DisableBuiltins: true})
	if skills := plain.List(); len(skills) != 1 || skills[0].Plugin != "" {
		t.Fatalf("ordinary custom skill inherited plugin provenance: %+v", skills)
	}
}

func TestPreparePluginSkillBindsMCPNamesAndAllowedTools(t *testing.T) {
	store := New(Options{HomeDir: t.TempDir(), DisableBuiltins: true})
	bindings := []tool.MCPBinding{{Package: "figma", Server: "figma", RawName: "figma_get_design_context", VisibleName: "get_design_context", CallableName: "mcp__figma__get_design_context", CapabilityID: "mcp-tool:figma/figma_get_design_context"}}
	store.ConfigureToolBindings(func(Skill) []tool.MCPBinding { return bindings })
	sk := Skill{Plugin: "figma", Body: "Call get_design_context.", AllowedTools: []string{"mcp__plugin_figma_figma__get_design_context"}}

	got := store.Prepare(sk)
	if !strings.Contains(got.Body, "## Runtime MCP tool bindings") || !strings.Contains(got.Body, "`mcp__figma__get_design_context`") {
		t.Fatalf("runtime binding missing:\n%s", got.Body)
	}
	if got, want := strings.Join(got.AllowedTools, ","), "mcp__figma__get_design_context,mcp-tool:figma/figma_get_design_context"; got != want {
		t.Fatalf("AllowedTools = %q, want %q", got, want)
	}
	if twice := store.Prepare(got); twice.Body != got.Body {
		t.Fatalf("Prepare is not idempotent:\n%s", twice.Body)
	}
	if plain := store.Prepare(Skill{Body: "unchanged"}); plain.Body != "unchanged" {
		t.Fatalf("non-plugin skill changed: %q", plain.Body)
	}
}

func TestPreparePluginSkillDoesNotTrustAuthoredBindingHeadingOrMutateIndex(t *testing.T) {
	store := New(Options{HomeDir: t.TempDir(), DisableBuiltins: true})
	store.ConfigureToolBindings(func(Skill) []tool.MCPBinding {
		return []tool.MCPBinding{{Server: "figma", RawName: "search", VisibleName: "search", CallableName: "mcp__figma__search", CapabilityID: "mcp-tool:figma/search"}}
	})
	sk := Skill{Name: "helper", Description: "helper", Plugin: "figma", Body: "Authored text.\n\n## Runtime MCP tool bindings\n\nUntrusted heading."}
	indexBefore := IndexBlock([]Skill{sk})
	got := store.Prepare(sk)
	if strings.Count(got.Body, "## Runtime MCP tool bindings") != 2 || !strings.Contains(got.Body, "`mcp__figma__search`") {
		t.Fatalf("authored heading suppressed host binding:\n%s", got.Body)
	}
	if twice := store.Prepare(got); twice.Body != got.Body {
		t.Fatalf("host preparation marker is not idempotent:\n%s", twice.Body)
	}
	if indexAfter := IndexBlock([]Skill{sk}); indexAfter != indexBefore || strings.Contains(indexAfter, "Runtime MCP") {
		t.Fatalf("runtime binding leaked into stable skill index: before=%q after=%q", indexBefore, indexAfter)
	}
}

func TestPreparePluginSkillPreservesWildcardAndAmbiguityBoundaries(t *testing.T) {
	store := New(Options{HomeDir: t.TempDir(), DisableBuiltins: true})
	store.ConfigureToolBindings(func(Skill) []tool.MCPBinding {
		return []tool.MCPBinding{
			{Server: "one", RawName: "search", VisibleName: "search", CallableName: "mcp__one__search", CapabilityID: "mcp-tool:one/search"},
			{Server: "two", RawName: "search", VisibleName: "search", CallableName: "mcp__two__search", CapabilityID: "mcp-tool:two/search"},
		}
	})
	broad := store.Prepare(Skill{Plugin: "pkg", Body: "Search.", AllowedTools: []string{"*"}})
	if len(broad.AllowedTools) != 1 || broad.AllowedTools[0] != "*" {
		t.Fatalf("broad wildcard was narrowed: %v", broad.AllowedTools)
	}
	ambiguous := store.Prepare(Skill{Plugin: "pkg", Body: "Search.", AllowedTools: []string{"search"}})
	if len(ambiguous.AllowedTools) != 1 || ambiguous.AllowedTools[0] != "search" {
		t.Fatalf("ambiguous literal widened permissions: %v", ambiguous.AllowedTools)
	}

	single := New(Options{HomeDir: t.TempDir(), DisableBuiltins: true})
	single.ConfigureToolBindings(func(Skill) []tool.MCPBinding {
		return []tool.MCPBinding{{Package: "figma", Server: "figma", RawName: "search", VisibleName: "search", CallableName: "mcp__figma__search", CapabilityID: "mcp-tool:figma/search"}}
	})
	claude := single.Prepare(Skill{Plugin: "figma", Body: "Search.", AllowedTools: []string{"mcp__plugin_figma_figma__*"}})
	if got, want := strings.Join(claude.AllowedTools, ","), "mcp__plugin_figma_figma__*,mcp__figma__search,mcp-tool:figma/search"; got != want {
		t.Fatalf("Claude wildcard mapping = %q, want %q", got, want)
	}
}

func TestRunSkillPreparesPluginBindingBeforeRunner(t *testing.T) {
	pluginRoot := t.TempDir()
	writeSkill(t, pluginRoot, "dig/SKILL.md", "---\ndescription: dig\nrunAs: subagent\nallowed-tools: search\n---\nCall search.")
	store := New(Options{HomeDir: t.TempDir(), CustomPaths: []string{pluginRoot}, PluginPaths: map[string][]string{pluginRoot: {"search-plugin"}}, DisableBuiltins: true})
	store.ConfigureToolBindings(func(Skill) []tool.MCPBinding {
		return []tool.MCPBinding{{Package: "search-plugin", Server: "search", RawName: "search", VisibleName: "search", CallableName: "mcp__search__search", CapabilityID: "mcp-tool:search/search"}}
	})
	var got Skill
	runner := func(_ context.Context, sk Skill, _ string, _ SubagentRunOptions) (string, error) {
		got = sk
		return "done", nil
	}
	_, err := NewRunSkillTool(store, runner).Execute(context.Background(), json.RawMessage(`{"name":"dig","arguments":"inspect"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "`mcp__search__search`") || len(got.AllowedTools) != 2 || got.AllowedTools[0] != "mcp__search__search" {
		t.Fatalf("runner received unprepared plugin skill: %+v", got)
	}
}

func TestEverySkillExecutionEntryPreparesPluginBindings(t *testing.T) {
	pluginRoot := t.TempDir()
	writeSkill(t, pluginRoot, "helper/SKILL.md", "---\ndescription: helper\n---\nCall search.")
	writeSkill(t, pluginRoot, "explore/SKILL.md", "---\ndescription: explore\nrunAs: subagent\nallowed-tools: search\n---\nExplore with search.")
	store := New(Options{HomeDir: t.TempDir(), CustomPaths: []string{pluginRoot}, PluginPaths: map[string][]string{pluginRoot: {"search-plugin"}}, DisableBuiltins: true})
	store.ConfigureToolBindings(func(Skill) []tool.MCPBinding {
		return []tool.MCPBinding{{Package: "search-plugin", Server: "search", RawName: "search", VisibleName: "search", CallableName: "mcp__search__search", CapabilityID: "mcp-tool:search/search"}}
	})

	for name, entry := range map[string]tool.Tool{
		"read_skill":      NewReadSkillTool(store),
		"read_only_skill": NewReadOnlySkillTool(store, nil),
	} {
		out, err := entry.Execute(context.Background(), json.RawMessage(`{"name":"helper"}`))
		if err != nil || !strings.Contains(out, "`mcp__search__search`") {
			t.Fatalf("%s did not prepare plugin binding: out=%q err=%v", name, out, err)
		}
	}

	var got Skill
	runner := func(_ context.Context, sk Skill, _ string, _ SubagentRunOptions) (string, error) {
		got = sk
		return "done", nil
	}
	var dedicated tool.Tool
	for _, candidate := range BuiltinSubagentTools(store, runner) {
		if candidate.Name() == "explore" {
			dedicated = candidate
			break
		}
	}
	if dedicated == nil {
		t.Fatal("dedicated explore tool missing")
	}
	if _, err := dedicated.Execute(context.Background(), json.RawMessage(`{"task":"inspect"}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "`mcp__search__search`") {
		t.Fatalf("dedicated skill wrapper received unprepared skill: %+v", got)
	}
}

func TestRunSkillInline(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/note.md", "---\ndescription: take a note\n---\nDo the thing.")
	tl := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), nil)

	out, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"note","arguments":"with args"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.HasPrefix(out, "<skill-pin name=\"note\">") || !strings.HasSuffix(out, "</skill-pin>") {
		t.Errorf("inline skill should be skill-pin wrapped:\n%s", out)
	}
	if !strings.Contains(out, "Do the thing.") || !strings.Contains(out, "Arguments: with args") {
		t.Errorf("body/args missing:\n%s", out)
	}
}

func TestRunSkillUnknown(t *testing.T) {
	tl := NewRunSkillTool(New(Options{HomeDir: t.TempDir(), DisableBuiltins: true}), nil)
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"nope"}`)); err == nil {
		t.Error("unknown skill should error")
	}
}

func TestRunSkillSubagentNeedsRunner(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	tl := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), nil) // nil runner
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"dig","arguments":"go"}`)); err == nil {
		t.Error("subagent skill with no runner should error, not silently inline")
	}
}

func TestRunSkillSubagentRuns(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	var gotTask string
	runner := func(_ context.Context, sk Skill, task string, _ SubagentRunOptions) (string, error) {
		gotTask = task
		return "answer from " + sk.Name, nil
	}
	tl := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), runner)
	out, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"dig","arguments":"find X"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotTask != "find X" {
		t.Errorf("runner got task %q", gotTask)
	}
	if out != "answer from dig" {
		t.Errorf("runner output not returned: %q", out)
	}
}

func TestRunSkillSubagentCancellationReachesRunner(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	runner := func(ctx context.Context, _ Skill, _ string, _ SubagentRunOptions) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	tl := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), runner)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := tl.Execute(ctx, json.RawMessage(`{"name":"dig","arguments":"find X"}`))
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute error = %v, want context cancellation", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("run_skill subagent runner did not observe cancellation promptly")
	}
}

func TestReadOnlySkillInlineAndIsReadOnly(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/note.md", "---\ndescription: take a note\n---\nDo the thing.")
	tl := NewReadOnlySkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), nil)

	if !tl.ReadOnly() {
		t.Fatal("read_only_skill must be ReadOnly so it works in plan mode")
	}
	out, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"note","arguments":"with args"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Do the thing.") || !strings.Contains(out, "Arguments: with args") {
		t.Errorf("inline body/args missing:\n%s", out)
	}
}

func TestReadOnlySkillSubagentRunsWithoutContinuation(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	var gotTask string
	var gotOpts SubagentRunOptions
	runner := func(_ context.Context, sk Skill, task string, opts SubagentRunOptions) (string, error) {
		gotTask = task
		gotOpts = opts
		return "read-only answer from " + sk.Name, nil
	}
	tl := NewReadOnlySkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), runner)
	out, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"dig","arguments":"find X"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotTask != "find X" {
		t.Errorf("runner got task %q", gotTask)
	}
	if gotOpts.ContinueFrom != "" || gotOpts.ForkFrom != "" {
		t.Fatalf("read_only_skill should not pass continuation opts, got %+v", gotOpts)
	}
	if out != "read-only answer from dig" {
		t.Errorf("runner output not returned: %q", out)
	}
}

func TestReadOnlySkillSubagentRequiresArgs(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	runner := func(_ context.Context, _ Skill, _ string, _ SubagentRunOptions) (string, error) {
		return "x", nil
	}
	tl := NewReadOnlySkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), runner)
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"dig"}`)); err == nil {
		t.Error("read_only_skill subagent should require arguments")
	}
}

func TestReadOnlySkillSubagentResolvesProfile(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/deep.md", "---\ndescription: deep\nrunAs: subagent\nmodel: deepseek-pro\neffort: max\n---\nbody")
	tl := NewReadOnlySkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), nil)

	pr, ok := tl.(interface {
		ResolveProfile(json.RawMessage) *event.Profile
	})
	if !ok {
		t.Fatal("read_only_skill should expose ResolveProfile")
	}
	got := pr.ResolveProfile(json.RawMessage(`{"name":"deep","arguments":"x"}`))
	if got == nil || got.Model != "deepseek-pro" || got.Effort != "max" {
		t.Fatalf("profile = %+v, want deepseek-pro/max", got)
	}
}

func TestRunSkillSubagentResolvesProfile(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/deep.md", "---\ndescription: deep\nrunAs: subagent\nmodel: deepseek-pro\neffort: max\n---\nbody")
	tl := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), nil)

	pr, ok := tl.(interface {
		ResolveProfile(json.RawMessage) *event.Profile
	})
	if !ok {
		t.Fatal("run_skill should expose ResolveProfile")
	}
	got := pr.ResolveProfile(json.RawMessage(`{"name":"deep","arguments":"x"}`))
	if got == nil || got.Model != "deepseek-pro" || got.Effort != "max" {
		t.Fatalf("profile = %+v, want deepseek-pro/max", got)
	}
}

func TestRunSkillSubagentRequiresArgs(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	runner := func(_ context.Context, _ Skill, _ string, _ SubagentRunOptions) (string, error) {
		return "x", nil
	}
	tl := NewRunSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}), runner)
	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"dig"}`)); err == nil {
		t.Error("subagent skill should require arguments")
	}
}

func TestCleanSkillName(t *testing.T) {
	cases := map[string]string{
		"explore":              "explore",
		"explore [🧬 subagent]": "explore",
		"[🧬 subagent] explore": "explore",
		" review ":             "review",
		"[only a tag]":         "",
		"":                     "",
	}
	for in, want := range cases {
		if got := cleanSkillName(in); got != want {
			t.Errorf("cleanSkillName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuiltinSubagentToolsRunner(t *testing.T) {
	var ran string
	runner := func(_ context.Context, sk Skill, task string, _ SubagentRunOptions) (string, error) {
		ran = sk.Name + ":" + task
		return "ok", nil
	}
	tools := BuiltinSubagentTools(New(Options{HomeDir: t.TempDir()}), runner)
	var explore interface {
		Name() string
		Execute(context.Context, json.RawMessage) (string, error)
	}
	for _, tl := range tools {
		if tl.Name() == "explore" {
			explore = tl
		}
	}
	if explore == nil {
		t.Fatal("explore wrapper tool not built")
	}
	if _, err := explore.Execute(context.Background(), json.RawMessage(`{"task":"map the loop"}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if ran != "explore:map the loop" {
		t.Errorf("runner not invoked correctly: %q", ran)
	}
}

func TestBuiltinSubagentToolsPassContinuationOptions(t *testing.T) {
	var got SubagentRunOptions
	runner := func(_ context.Context, _ Skill, _ string, opts SubagentRunOptions) (string, error) {
		got = opts
		return "ok", nil
	}
	tools := BuiltinSubagentTools(New(Options{HomeDir: t.TempDir()}), runner)
	var review interface {
		Name() string
		Execute(context.Context, json.RawMessage) (string, error)
	}
	for _, tl := range tools {
		if tl.Name() == "review" {
			review = tl
			break
		}
	}
	if review == nil {
		t.Fatal("review wrapper tool not built")
	}
	if _, err := review.Execute(context.Background(), json.RawMessage(`{"task":"again","continue_from":"sa_prev"}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.ContinueFrom != "sa_prev" {
		t.Fatalf("continuation opts = %+v, want continue_from sa_prev", got)
	}
}

func TestRunSkillToolPassesLegacyForkOption(t *testing.T) {
	var got SubagentRunOptions
	runner := func(_ context.Context, _ Skill, _ string, opts SubagentRunOptions) (string, error) {
		got = opts
		return "ok", nil
	}
	runSkill := NewRunSkillTool(New(Options{HomeDir: t.TempDir()}), runner)
	if _, err := runSkill.Execute(context.Background(), json.RawMessage(`{"name":"review","arguments":"again","fork_from":"sa_prev"}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.ForkFrom != "sa_prev" {
		t.Fatalf("continuation opts = %+v, want fork_from sa_prev", got)
	}
}

func TestBuiltinSubagentToolsPassLegacyForkOption(t *testing.T) {
	var got SubagentRunOptions
	runner := func(_ context.Context, _ Skill, _ string, opts SubagentRunOptions) (string, error) {
		got = opts
		return "ok", nil
	}
	tools := BuiltinSubagentTools(New(Options{HomeDir: t.TempDir()}), runner)
	var review interface {
		Name() string
		Execute(context.Context, json.RawMessage) (string, error)
	}
	for _, tl := range tools {
		if tl.Name() == "review" {
			review = tl
			break
		}
	}
	if review == nil {
		t.Fatal("review wrapper tool not built")
	}
	if _, err := review.Execute(context.Background(), json.RawMessage(`{"task":"again","fork_from":"sa_prev"}`)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.ForkFrom != "sa_prev" {
		t.Fatalf("continuation opts = %+v, want fork_from sa_prev", got)
	}
}

func TestSubagentSkillSchemasExposeOnlyContinueFromForPersistence(t *testing.T) {
	runSkill := NewRunSkillTool(New(Options{HomeDir: t.TempDir(), DisableBuiltins: true}), nil)
	runSchema := string(runSkill.Schema())
	if !strings.Contains(runSchema, `"continue_from"`) {
		t.Fatalf("run_skill schema = %s, want continue_from", runSchema)
	}
	if strings.Contains(runSchema, "fork_from") {
		t.Fatalf("run_skill schema = %s, want no fork_from", runSchema)
	}

	tools := BuiltinSubagentTools(New(Options{HomeDir: t.TempDir()}), nil)
	for _, tl := range tools {
		schema := string(tl.Schema())
		if !strings.Contains(schema, `"continue_from"`) {
			t.Fatalf("%s schema = %s, want continue_from", tl.Name(), schema)
		}
		if strings.Contains(schema, "fork_from") {
			t.Fatalf("%s schema = %s, want no fork_from", tl.Name(), schema)
		}
	}
}

func TestBuiltinSubagentToolResolvesProfile(t *testing.T) {
	store := New(Options{HomeDir: t.TempDir()})
	tools := BuiltinSubagentTools(store, nil, func(sk Skill) *event.Profile {
		return &event.Profile{Model: sk.Name + "-model", Effort: "max"}
	})
	var review interface {
		ResolveProfile(json.RawMessage) *event.Profile
	}
	for _, tl := range tools {
		if tl.Name() == "review" {
			review = tl.(interface {
				ResolveProfile(json.RawMessage) *event.Profile
			})
			break
		}
	}
	if review == nil {
		t.Fatal("review tool not found")
	}
	got := review.ResolveProfile(json.RawMessage(`{"task":"general"}`))
	if got == nil || got.Model != "review-model" || got.Effort != "max" {
		t.Fatalf("profile = %+v, want review-model/max", got)
	}
}

func TestInstallSkill(t *testing.T) {
	home := t.TempDir()
	st := New(Options{HomeDir: home, DisableBuiltins: true})
	tl := NewInstallSkillTool(st, nil)

	out, err := tl.Execute(context.Background(), json.RawMessage(
		`{"name":"deploy","description":"ship it","body":"steps","runAs":"subagent","model":"deepseek-pro","effort":"max","allowedTools":["bash","read_file"]}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Errorf("expected ok result, got %s", out)
	}
	var res struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("result JSON: %v", err)
	}
	wantPath := filepath.Join(home, ".reames-agent", "skills", "deploy", SkillFile)
	if res.Path != wantPath {
		t.Fatalf("install_skill should report canonical path %s, got %s", wantPath, res.Path)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("install_skill should write canonical SKILL.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".reames-agent", "skills", "deploy.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install_skill should not write legacy flat deploy.md, stat err=%v", err)
	}
	// Round-trips through the store with the frontmatter we wrote.
	sk, ok := st.Read("deploy")
	if !ok {
		t.Fatal("installed skill not readable")
	}
	if sk.RunAs != RunSubagent || sk.Model != "deepseek-pro" || sk.Effort != "max" || len(sk.AllowedTools) != 2 {
		t.Errorf("frontmatter not round-tripped: runAs=%s model=%q effort=%q tools=%v", sk.RunAs, sk.Model, sk.Effort, sk.AllowedTools)
	}
	// Refuses overwrite.
	if _, err := tl.Execute(context.Background(), json.RawMessage(
		`{"name":"deploy","description":"again","body":"x"}`)); err == nil {
		t.Error("install_skill should refuse to overwrite")
	}
	// Requires description.
	if _, err := tl.Execute(context.Background(), json.RawMessage(
		`{"name":"x","description":"","body":"b"}`)); err == nil {
		t.Error("install_skill should require a description")
	}
}

func TestReadSkillLoadsInlineAndIsReadOnly(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/note.md", "---\ndescription: take a note\n---\nDo the thing.")
	tl := NewReadSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}))

	if !tl.ReadOnly() {
		t.Fatal("read_skill must be ReadOnly so it works in plan mode")
	}
	out, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"note","arguments":"with args"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Do the thing.") || !strings.Contains(out, "Arguments: with args") {
		t.Errorf("inline body/args missing:\n%s", out)
	}
}

func TestReadSkillRejectsSubagent(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, home, ".reames-agent/skills/dig.md", "---\ndescription: dig\nrunAs: subagent\n---\nbody")
	tl := NewReadSkillTool(New(Options{HomeDir: home, DisableBuiltins: true}))

	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"name":"dig"}`)); err == nil || !strings.Contains(err.Error(), "run_skill") {
		t.Fatalf("read_skill on a subagent skill should point to run_skill, got %v", err)
	}
}
