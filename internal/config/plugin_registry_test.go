package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginRegistryTrustConfigurationIsUserGlobal(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	if err := os.MkdirAll(filepath.Dir(UserConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(UserConfigPath(), []byte(`[plugin_registry]
metadata_url = "https://registry.example/metadata"
targets_url = "https://cdn.example/targets"
trusted_root = "registry/root.json"
index_target = "catalog/plugins.json"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "reames-agent.toml"), []byte(`[plugin_registry]
metadata_url = "https://attacker.invalid/metadata"
trusted_root = "attacker-root.json"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadForRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PluginRegistryMetadataURL() != "https://registry.example/metadata" || cfg.PluginRegistryTargetsURL() != "https://cdn.example/targets" {
		t.Fatalf("project replaced registry endpoints: %+v", cfg.PluginRegistry)
	}
	if got, want := cfg.PluginRegistryTrustedRootPath(), filepath.Join(home, "registry", "root.json"); filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("trusted root = %q, want %q", got, want)
	}
	if cfg.PluginRegistry.IndexTarget != "catalog/plugins.json" {
		t.Fatalf("index target = %q", cfg.PluginRegistry.IndexTarget)
	}
}

func TestPluginRegistryTrustExpansionIgnoresProjectDotEnv(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_REGISTRY_METADATA_TEST", "")
	t.Setenv("REAMES_REGISTRY_TARGETS_TEST", "")
	t.Setenv("REAMES_REGISTRY_ROOT_TEST", "")
	if err := os.MkdirAll(filepath.Dir(UserConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(UserConfigPath(), []byte(`[plugin_registry]
metadata_url = "${REAMES_REGISTRY_METADATA_TEST}"
targets_url = "${REAMES_REGISTRY_TARGETS_TEST}"
trusted_root = "${REAMES_REGISTRY_ROOT_TEST}"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte(`REAMES_REGISTRY_METADATA_TEST=https://attacker.invalid/metadata
REAMES_REGISTRY_TARGETS_TEST=https://attacker.invalid/targets
REAMES_REGISTRY_ROOT_TEST=attacker-root.json
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadForRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.PluginRegistryMetadataURL(); got != "" {
		t.Fatalf("metadata URL expanded from project .env: %q", got)
	}
	if got := cfg.PluginRegistryTargetsURL(); got != "" {
		t.Fatalf("targets URL expanded from project .env: %q", got)
	}
	if got := cfg.PluginRegistryTrustedRootPath(); got != "" {
		t.Fatalf("trusted root expanded from project .env: %q", got)
	}

	t.Setenv("REAMES_REGISTRY_METADATA_TEST", "https://registry.example/metadata")
	t.Setenv("REAMES_REGISTRY_TARGETS_TEST", "https://registry.example/targets")
	t.Setenv("REAMES_REGISTRY_ROOT_TEST", "registry/root.json")
	if got := cfg.PluginRegistryMetadataURL(); got != "https://registry.example/metadata" {
		t.Fatalf("metadata URL process expansion = %q", got)
	}
	if got := cfg.PluginRegistryTargetsURL(); got != "https://registry.example/targets" {
		t.Fatalf("targets URL process expansion = %q", got)
	}
	if got, want := cfg.PluginRegistryTrustedRootPath(), filepath.Join(home, "registry", "root.json"); filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("trusted root process expansion = %q, want %q", got, want)
	}
}

func TestPluginRegistryRenderScopeAndRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.PluginRegistry = PluginRegistryConfig{
		MetadataURL: "https://registry.example/metadata",
		TargetsURL:  "https://registry.example/targets",
		TrustedRoot: "registry/root.json",
		IndexTarget: "plugins.json",
	}
	user := RenderTOMLForScope(cfg, RenderScopeUser)
	if !strings.Contains(user, "[plugin_registry]") || !strings.Contains(user, `trusted_root = "registry/root.json"`) {
		t.Fatalf("user render omitted registry trust:\n%s", user)
	}
	project := RenderTOMLForScope(cfg, RenderScopeProject)
	if strings.Contains(project, "[plugin_registry]") {
		t.Fatalf("project render included user-global registry trust:\n%s", project)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := LoadForEdit(path)
	if loaded.PluginRegistry != cfg.PluginRegistry {
		t.Fatalf("registry round trip = %+v, want %+v", loaded.PluginRegistry, cfg.PluginRegistry)
	}
}

func TestPluginRegistryTrustedRootEmptyStaysEmpty(t *testing.T) {
	if got := Default().PluginRegistryTrustedRootPath(); got != "" {
		t.Fatalf("empty trusted root resolved to %q", got)
	}
}
