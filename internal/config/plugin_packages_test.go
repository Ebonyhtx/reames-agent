package config

import (
	"os"
	"path/filepath"
	"testing"

	"reames-agent/internal/pluginpkg"
)

func TestMergeInstalledPluginPackagesTracksMCPOwnershipAndPreservesCollision(t *testing.T) {
	reamesHome := filepath.Join(t.TempDir(), ".reames-agent")
	t.Setenv("REAMES_AGENT_HOME", reamesHome)
	source := filepath.Join(t.TempDir(), "owner-pack")
	writePluginPackageTestFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "owner-pack",
  "version": "1.0.0",
  "mcpServers": {
    "collision": {"command": "bin/helper"},
    "package-only": {"command": "bin/helper"}
  },
  "permissions": ["mcp.stdio"]
}`)
	writePluginPackageTestFile(t, filepath.Join(source, "bin", "helper"), "helper")
	result, err := pluginpkg.Install(reamesHome, pluginpkg.InstallRequest{
		Name: "owner-pack", Source: source, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginpkg.Enable(reamesHome, pluginpkg.EnableRequest{
		Name: result.Installed.Name, ExpectedDigest: result.Installed.Digest, GrantedPermissions: result.Installed.Permissions,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	cfg.Plugins = []PluginEntry{{Name: "collision", Command: "user-command"}}
	warnings := mergeInstalledPluginPackages(cfg, t.TempDir())
	if len(warnings) == 0 {
		t.Fatal("expected a collision warning")
	}
	if len(cfg.Plugins) != 2 {
		t.Fatalf("plugins = %+v", cfg.Plugins)
	}
	if cfg.Plugins[0].PluginPackageOwner() != "" || cfg.Plugins[0].Command != "user-command" {
		t.Fatalf("user-owned collision entry changed: %+v", cfg.Plugins[0])
	}
	if cfg.Plugins[1].Name != "package-only" || cfg.Plugins[1].PluginPackageOwner() != "owner-pack" {
		t.Fatalf("package ownership missing: %+v", cfg.Plugins[1])
	}
	if cfg.Plugins[1].PluginPackageRoot() == "" || cfg.Plugins[1].PluginPackageHome() != reamesHome {
		t.Fatalf("package process roots missing: %+v", cfg.Plugins[1])
	}
	wantState := pluginpkg.RuntimeStateDir(reamesHome, "owner-pack")
	if cfg.Plugins[1].PluginPackageStateDir() != wantState || cfg.Plugins[1].Env["REAMES_AGENT_PLUGIN_STATE"] != wantState {
		t.Fatalf("package state root = %q env=%q, want %q", cfg.Plugins[1].PluginPackageStateDir(), cfg.Plugins[1].Env["REAMES_AGENT_PLUGIN_STATE"], wantState)
	}
}

func TestLoadMergesVerifiedPluginSkillRootsAndMCP(t *testing.T) {
	reamesHome := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", reamesHome)
	source := filepath.Join(t.TempDir(), "superpowers")
	writePluginPackageTestFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "superpowers",
  "version": "1.0.0",
  "skills": ["skills"],
  "mcpServers": {"helper": {"command": "bin/helper"}},
  "permissions": ["skills.load", "mcp.stdio"]
}`)
	writePluginPackageTestFile(t, filepath.Join(source, "skills", "fixture", "SKILL.md"), "---\nname: fixture\ndescription: fixture\n---\nRun.\n")
	writePluginPackageTestFile(t, filepath.Join(source, "bin", "helper"), "helper")
	result, err := pluginpkg.Install(reamesHome, pluginpkg.InstallRequest{
		Name: "superpowers", Source: source, SourceRoot: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginpkg.Enable(reamesHome, pluginpkg.EnableRequest{
		Name: result.Installed.Name, ExpectedDigest: result.Installed.Digest, GrantedPermissions: result.Installed.Permissions,
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := pluginpkg.ResolveRoot(reamesHome, result.Installed.Root)
	if len(cfg.Skills.Paths) == 0 || cfg.Skills.Paths[len(cfg.Skills.Paths)-1] != filepath.Join(root, "skills") {
		t.Fatalf("skills paths = %#v", cfg.Skills.Paths)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0].Name != "helper" || cfg.Plugins[0].PluginPackageOwner() != "superpowers" {
		t.Fatalf("plugin MCP server = %+v", cfg.Plugins)
	}
	if cfg.Plugins[0].Command != filepath.Join(root, "bin", "helper") {
		t.Fatalf("plugin command = %q", cfg.Plugins[0].Command)
	}
	if cfg.Plugins[0].PluginPackageRoot() != root || cfg.Plugins[0].PluginPackageStateDir() != pluginpkg.RuntimeStateDir(reamesHome, "superpowers") {
		t.Fatalf("plugin package policy roots missing: %+v", cfg.Plugins[0])
	}
}

func writePluginPackageTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
