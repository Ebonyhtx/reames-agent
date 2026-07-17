package guardcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckJSONDoesNotRequireConfigOrCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := Run([]string{"check", "--json", "--root", t.TempDir()}, "v1", strings.NewReader(""), &out, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, stderr=%q output=%q", code, stderr.String(), out.String())
	}
	if !strings.Contains(out.String(), `"config.invalid"`) || strings.Contains(out.String(), "must-not-load") {
		t.Fatalf("unexpected report: %s", out.String())
	}
}

func TestVersionAndUnknownCommand(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := Run([]string{"version"}, "v1.2.3", strings.NewReader(""), &out, &stderr); code != 0 || out.String() != "reames-agent-guard v1.2.3\n" {
		t.Fatalf("version = %d %q %q", code, out.String(), stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Run([]string{"unknown"}, "v1", strings.NewReader(""), &out, &stderr); code != 2 {
		t.Fatalf("unknown code = %d", code)
	}
}
