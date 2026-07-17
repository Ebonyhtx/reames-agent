package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeModeIgnoresBrokenConfigAndCredentials(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	t.Setenv("DEEPSEEK_API_KEY", "")
	for _, path := range []string{filepath.Join(home, "config.toml"), filepath.Join(root, "reames-agent.toml")} {
		if err := os.WriteFile(path, []byte("[broken\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("DEEPSEEK_API_KEY=must-not-load\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadForRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SafeMode() || len(cfg.Plugins) != 0 || cfg.Bot.Enabled || cfg.DesktopCheckUpdates() || cfg.DesktopTelemetry() || cfg.DesktopMetrics() {
		t.Fatalf("unsafe recovery config: %+v", cfg)
	}
}

func TestValidateFileIsReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	raw := []byte("[broken\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(path); err == nil {
		t.Fatal("malformed TOML validated")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("ValidateFile rewrote input: %q", got)
	}
}
