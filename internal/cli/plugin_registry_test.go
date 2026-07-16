package cli

import (
	"strings"
	"testing"
)

func TestPluginRegistryCLIUsageDoesNotRequireConfiguration(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())

	registryUsage := captureStderr(t, func() {
		if rc := pluginCommand([]string{"registry"}); rc != 2 {
			t.Fatalf("plugin registry rc = %d, want 2", rc)
		}
	})
	if !strings.Contains(registryUsage, "plugin registry search [query]") ||
		!strings.Contains(registryUsage, "show <name>") ||
		!strings.Contains(registryUsage, "refresh") ||
		!strings.Contains(registryUsage, "digest <checkout> [subpath]") {
		t.Fatalf("registry usage is incomplete:\n%s", registryUsage)
	}

	pluginHelp := captureStderr(t, func() {
		if rc := pluginCommand([]string{"help"}); rc != 0 {
			t.Fatalf("plugin help rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(pluginHelp, "plugin registry search [query]") ||
		!strings.Contains(pluginHelp, "plugin registry show <name>") ||
		!strings.Contains(pluginHelp, "plugin registry refresh") ||
		!strings.Contains(pluginHelp, "plugin registry digest <checkout> [subpath]") {
		t.Fatalf("plugin help omitted registry commands:\n%s", pluginHelp)
	}

	registryHelp := captureStderr(t, func() {
		if rc := pluginCommand([]string{"registry", "help"}); rc != 0 {
			t.Fatalf("plugin registry help rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(registryHelp, "plugin registry search [query]") {
		t.Fatalf("registry help is incomplete:\n%s", registryHelp)
	}
}

func TestPluginRegistryCLIUnconfiguredFailsClosed(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())

	errOut := captureStderr(t, func() {
		if rc := pluginCommand([]string{"registry", "search"}); rc != 1 {
			t.Fatalf("unconfigured registry search rc = %d, want 1", rc)
		}
	})
	if !strings.Contains(errOut, "plugin registry") || !strings.Contains(errOut, "not configured") {
		t.Fatalf("unconfigured registry error = %q", errOut)
	}
}

func TestPluginRegistryCLIValidatesArgumentsBeforeNetworkAccess(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())

	tests := []struct {
		name    string
		args    []string
		wantRC  int
		wantErr string
	}{
		{name: "help", args: []string{"registry", "help"}, wantRC: 0, wantErr: "usage:"},
		{name: "search extra arguments", args: []string{"registry", "search", "one", "two"}, wantRC: 2, wantErr: "at most one query"},
		{name: "show missing name", args: []string{"registry", "show"}, wantRC: 2, wantErr: "exactly one plugin name"},
		{name: "show extra name", args: []string{"registry", "show", "one", "two"}, wantRC: 2, wantErr: "exactly one plugin name"},
		{name: "refresh extra argument", args: []string{"registry", "refresh", "extra"}, wantRC: 2, wantErr: "accepts no arguments"},
		{name: "digest missing checkout", args: []string{"registry", "digest"}, wantRC: 2, wantErr: "requires a checkout"},
		{name: "digest extra argument", args: []string{"registry", "digest", "repo", "subpath", "extra"}, wantRC: 2, wantErr: "requires a checkout"},
		{name: "unknown command", args: []string{"registry", "unknown"}, wantRC: 2, wantErr: "unknown plugin registry command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errOut := captureStderr(t, func() {
				if rc := pluginCommand(tt.args); rc != tt.wantRC {
					t.Fatalf("plugin %v rc = %d, want %d", tt.args, rc, tt.wantRC)
				}
			})
			if !strings.Contains(errOut, tt.wantErr) {
				t.Fatalf("plugin %v stderr = %q, want %q", tt.args, errOut, tt.wantErr)
			}
		})
	}
}

func TestPluginRegistryDigestDoesNotRequireRegistryConfiguration(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	errOut := captureStderr(t, func() {
		if rc := pluginCommand([]string{"registry", "digest", t.TempDir()}); rc != 1 {
			t.Fatalf("plugin registry digest rc = %d, want local Git failure", rc)
		}
	})
	if strings.Contains(errOut, "plugin registry is not configured") || !strings.Contains(errOut, "Git commit") {
		t.Fatalf("local registry digest error = %q", errOut)
	}
}
