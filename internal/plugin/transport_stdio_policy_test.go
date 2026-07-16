package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/processpolicy"
)

func TestOwnedStdioUsesPackagePolicyBeforeSpawn(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	calledShellProbe := false
	previous := stdioShellPATH
	stdioShellPATH = func(context.Context) string {
		calledShellProbe = true
		return filepath.Dir(exe)
	}
	defer func() { stdioShellPATH = previous }()

	_, err = newStdioTransport(context.Background(), Spec{
		Name: "owned", Command: exe,
		PackagePolicy: processpolicy.PackagePolicy{
			Owner: "owned", PackageRoot: t.TempDir(), StateRoot: filepath.Join(t.TempDir(), "state"), HostHome: "relative-home",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "host home must be an absolute path") {
		t.Fatalf("owned stdio policy error = %v", err)
	}
	if calledShellProbe {
		t.Fatal("owned stdio consulted an ambient login shell before confinement")
	}
}

func TestSpecFingerprintIncludesPackageProcessPolicy(t *testing.T) {
	base := Spec{Name: "server", Command: "helper"}
	owned := base
	owned.PackagePolicy = processpolicy.PackagePolicy{Owner: "pack", PackageRoot: "/pack", StateRoot: "/state", WorkspaceRoot: "/work"}
	if SpecFingerprint(base) == SpecFingerprint(owned) {
		t.Fatal("schema cache fingerprint ignored package process policy")
	}
}

func TestOwnedStdioRedactsConfiguredSecretsFromFailureDiagnostics(t *testing.T) {
	stderr := &tailBuffer{limit: 1024}
	_, _ = stderr.Write([]byte("PLUGIN_TOKEN=explicit-plugin-token"))
	transport := &stdioTransport{
		name: "owned", stderr: stderr,
		redact: processpolicy.SensitiveValues(map[string]string{"PLUGIN_TOKEN": "explicit-plugin-token"}),
	}
	err := transport.withStderr(context.Canceled)
	if err == nil || strings.Contains(err.Error(), "explicit-plugin-token") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("redacted diagnostic = %v", err)
	}
}
