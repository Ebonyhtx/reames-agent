package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/pluginregistry"
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
		!strings.Contains(registryUsage, "digest <checkout> [subpath]") ||
		!strings.Contains(registryUsage, "audit <repository> --root <root.json>") {
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
		!strings.Contains(pluginHelp, "plugin registry digest <checkout> [subpath]") ||
		!strings.Contains(pluginHelp, "plugin registry audit <repository> --root <root.json>") {
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
		{name: "audit missing repository", args: []string{"registry", "audit", "--root", "root.json"}, wantRC: 2, wantErr: "exactly one repository"},
		{name: "audit missing root", args: []string{"registry", "audit", "repository"}, wantRC: 2, wantErr: "requires --root"},
		{name: "audit bad time", args: []string{"registry", "audit", "repository", "--root", "root.json", "--at", "tomorrow"}, wantRC: 2, wantErr: "RFC3339"},
		{name: "audit duplicate root", args: []string{"registry", "audit", "repository", "--root", "one.json", "--root=two.json"}, wantRC: 2, wantErr: "only once"},
		{name: "audit unknown flag", args: []string{"registry", "audit", "repository", "--root", "root.json", "--trust-on-first-use"}, wantRC: 2, wantErr: "unknown plugin registry audit flag"},
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

func TestPluginRegistryAuditDoesNotRequireRegistryConfiguration(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())
	repository := t.TempDir()
	errOut := captureStderr(t, func() {
		if rc := pluginCommand([]string{"registry", "audit", repository, "--root", filepath.Join(repository, "missing-root.json")}); rc != 1 {
			t.Fatalf("plugin registry audit rc = %d, want local file failure", rc)
		}
	})
	if strings.Contains(errOut, "plugin registry is not configured") || !strings.Contains(errOut, "trusted root") {
		t.Fatalf("local registry audit error = %q", errOut)
	}
}

func TestPluginRegistryAuditEmitsMachineReadableExternalBoundary(t *testing.T) {
	old := auditPluginRegistry
	t.Cleanup(func() { auditPluginRegistry = old })
	auditPluginRegistry = func(_ context.Context, opts pluginregistry.AuditOptions) (*pluginregistry.AuditReport, error) {
		if opts.RepositoryDir != "repository" || opts.TrustedRootPath != "root.json" || opts.IndexTarget != "nested/plugins.json" {
			t.Fatalf("audit options = %+v", opts)
		}
		wantTime := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
		if !opts.ReferenceTime.Equal(wantTime) {
			t.Fatalf("audit reference time = %s, want %s", opts.ReferenceTime, wantTime)
		}
		return &pluginregistry.AuditReport{
			SchemaVersion: 1, Policy: "reames-production-v1", ReferenceTime: wantTime,
			ExternalRequired: []string{"production HSM evidence"},
		}, nil
	}
	out := captureStdout(t, func() {
		if rc := pluginCommand([]string{"registry", "audit", "repository", "--root", "root.json", "--index", "nested/plugins.json", "--at", "2026-07-16T10:00:00Z"}); rc != 0 {
			t.Fatalf("plugin registry audit rc = %d, want 0", rc)
		}
	})
	var report pluginregistry.AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode audit JSON: %v\n%s", err, out)
	}
	if report.SchemaVersion != 1 || len(report.ExternalRequired) != 1 {
		t.Fatalf("audit report = %+v", report)
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
