package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/config"
)

func TestAppServerInitializesWithoutLoadingProviderCredentials(t *testing.T) {
	isolateCLIConfigHome(t)
	t.Setenv("DEEPSEEK_API_KEY", "")
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString(`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"fixture","version":"1.0"}}}` + "\n")
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })
	out := captureStdout(t, func() {
		if rc := Run([]string{"app-server"}, "test-version"); rc != 0 {
			t.Fatalf("app-server initialize rc=%d", rc)
		}
	})
	for _, want := range []string{`"userAgent":"reames-agent/test-version"`, `"platformFamily"`, `"platformOs"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("initialize output missing %s: %s", want, out)
		}
	}
	if strings.Contains(out, `"jsonrpc"`) {
		t.Fatalf("App-Server output must omit jsonrpc: %s", out)
	}
}

func TestAppServerRejectsUnsupportedTransport(t *testing.T) {
	isolateCLIConfigHome(t)
	if rc := appServerCommand([]string{"--listen", "ws://127.0.0.1:0"}, "test"); rc != 2 {
		t.Fatalf("unsupported transport rc=%d", rc)
	}
}

func TestAppServerSandboxProjectionDoesNotUnderstateAccess(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Sandbox.Bash = "off"
	if got := appServerSandbox(cfg, root); got.Type != "dangerFullAccess" || len(got.WritableRoots) != 0 {
		t.Fatalf("unconfined sandbox projection = %+v", got)
	}

	extra := t.TempDir()
	cfg.Sandbox.Bash = "enforce"
	cfg.Sandbox.Network = true
	cfg.Sandbox.AllowWrite = []string{extra, extra}
	got := appServerSandbox(cfg, root)
	if got.Type != "workspaceWrite" || !got.NetworkAccess || len(got.WritableRoots) != 2 {
		t.Fatalf("confined sandbox projection = %+v", got)
	}
	if got.WritableRoots[0] != filepath.Clean(root) || got.WritableRoots[1] != filepath.Clean(extra) {
		t.Fatalf("writable roots = %v", got.WritableRoots)
	}
}
