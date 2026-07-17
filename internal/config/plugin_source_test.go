package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginConfigSourcesAreStableCategories(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "reames-agent.toml")
	if err := os.WriteFile(tomlPath, []byte("[[plugins]]\nname = \"toml-server\"\ncommand = \"server\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := mergeTOMLPlugins([]string{tomlPath})
	if err != nil || len(entries) != 1 || entries[0].PluginConfigSource() != "toml" {
		t.Fatalf("TOML plugin source = %+v, err=%v", entries, err)
	}
	mcpPath := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"json-server":{"command":"server"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err = loadMCPJSON(mcpPath)
	if err != nil || len(entries) != 1 || entries[0].PluginConfigSource() != "mcp_json" {
		t.Fatalf("MCP JSON plugin source = %+v, err=%v", entries, err)
	}
	if got := (PluginEntry{Name: "new"}).PluginConfigSource(); got != "toml" {
		t.Fatalf("new configured plugin source = %q", got)
	}
	if got := (PluginEntry{Name: "pkg", packageOwner: "owner"}).PluginConfigSource(); got != "plugin_package:owner" {
		t.Fatalf("package plugin source = %q", got)
	}
}
