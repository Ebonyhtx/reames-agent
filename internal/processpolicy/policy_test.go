package processpolicy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reames-agent/internal/sandbox"
)

func TestCoreEnvironmentDropsAmbientSecretsAndRuntimeInjection(t *testing.T) {
	got := CoreEnvironment([]string{
		"PATH=/bin",
		"HOME=/home/test",
		"OPENAI_API_KEY=host-secret",
		"GH_TOKEN=host-token",
		"LD_PRELOAD=/tmp/evil.so",
		"NODE_OPTIONS=--require=/tmp/evil.js",
	})
	joined := strings.Join(got, "\n")
	for _, leaked := range []string{"OPENAI_API_KEY", "GH_TOKEN", "LD_PRELOAD", "NODE_OPTIONS", "host-secret", "host-token"} {
		if strings.Contains(joined, leaked) {
			t.Fatalf("core environment leaked %q:\n%s", leaked, joined)
		}
	}
	wants := []string{"PATH=/bin"}
	if runtime.GOOS != "windows" {
		wants = append(wants, "HOME=/home/test")
	}
	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Fatalf("core environment missing %q:\n%s", want, joined)
		}
	}
}

func TestSensitiveEnvironmentDiagnosticsAreRedacted(t *testing.T) {
	env := map[string]string{
		"PLUGIN_API_KEY": "sk-explicit-plugin-secret",
		"DB_PWD":         "database-password",
		"PWD":            "/workspace",
		"PATH":           "/bin",
	}
	values := SensitiveValues(env)
	got := RedactValues("key=sk-explicit-plugin-secret db=database-password cwd=/workspace", values)
	for _, leaked := range []string{"sk-explicit-plugin-secret", "database-password"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("diagnostic leaked %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "cwd=/workspace") {
		t.Fatalf("bare PWD path was incorrectly redacted: %s", got)
	}
}

func TestPackageChildEnvironmentUsesExplicitEnvAndTrustedOwnership(t *testing.T) {
	home := t.TempDir()
	pkg := t.TempDir()
	workspace := t.TempDir()
	state := filepath.Join(home, "plugins", "demo", "state")
	t.Setenv("REAMES_AGENT_TEST_HOST_SECRET_TOKEN", "ambient-secret")
	p := PackagePolicy{Owner: "demo", PackageRoot: pkg, StateRoot: state, WorkspaceRoot: workspace, HostHome: home, Network: true}

	env := p.ChildEnvironment(map[string]string{
		"PLUGIN_TOKEN":             "explicit-secret",
		"REAMES_AGENT_PLUGIN_ROOT": "spoofed",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "REAMES_AGENT_TEST_HOST_SECRET_TOKEN") || strings.Contains(joined, "ambient-secret") {
		t.Fatalf("ambient secret leaked:\n%s", joined)
	}
	for _, want := range []string{
		"PLUGIN_TOKEN=explicit-secret",
		"REAMES_AGENT_PLUGIN_NAME=demo",
		"REAMES_AGENT_PLUGIN_ROOT=" + pkg,
		"REAMES_AGENT_PLUGIN_STATE=" + state,
		"REAMES_AGENT_WORKSPACE_ROOT=" + workspace,
		"TMPDIR=" + filepath.Join(state, "tmp"),
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("child environment missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "spoofed") {
		t.Fatalf("manifest spoofed trusted package root:\n%s", joined)
	}
}

func TestPackagePrepareCreatesStateResolveBeneathHostHome(t *testing.T) {
	home := t.TempDir()
	p := PackagePolicy{
		Owner: "demo", PackageRoot: t.TempDir(), StateRoot: filepath.Join(home, "plugins", "demo", "state"),
		WorkspaceRoot: t.TempDir(), HostHome: home,
	}
	if err := p.Prepare(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p.StateRoot)
	if err != nil || !info.IsDir() {
		t.Fatalf("managed state root missing: info=%v err=%v", info, err)
	}
}

func TestPackagePrepareRejectsStateSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(home, "plugins", "demo")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(parent, "state")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	p := PackagePolicy{Owner: "demo", PackageRoot: t.TempDir(), StateRoot: filepath.Join(parent, "state"), HostHome: home}
	if err := p.Prepare(); err == nil {
		t.Fatal("state symlink escape should fail closed")
	}
}

func TestWrapCommandUsesStrictSandboxAndSensitiveBarriers(t *testing.T) {
	home := t.TempDir()
	pkg := t.TempDir()
	workspace := t.TempDir()
	state := filepath.Join(home, "plugins", "demo", "state")
	credential := filepath.Join(home, ".env")
	if err := os.WriteFile(credential, []byte("API_KEY=secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := PackagePolicy{Owner: "demo", PackageRoot: pkg, StateRoot: state, WorkspaceRoot: workspace, HostHome: home, Network: true}
	childEnv := p.ChildEnvironment(map[string]string{"PLUGIN_TOKEN": "explicit"})

	previous := commandArgsWithOptions
	defer func() { commandArgsWithOptions = previous }()
	var gotSpec sandbox.Spec
	var gotOpts sandbox.CommandOptions
	commandArgsWithOptions = func(spec sandbox.Spec, args []string, opts sandbox.CommandOptions) ([]string, bool) {
		gotSpec = spec
		gotOpts = opts
		return append([]string{"sandbox-wrapper"}, args...), true
	}
	argv, hostEnv, err := p.WrapCommand([]string{"plugin-bin", "serve"}, childEnv, pkg, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 3 || argv[0] != "sandbox-wrapper" {
		t.Fatalf("wrapped argv = %v", argv)
	}
	if !gotSpec.Strict || !gotSpec.Network || !gotOpts.Writable || gotOpts.Dir != pkg {
		t.Fatalf("sandbox contract not preserved: spec=%+v opts=%+v", gotSpec, gotOpts)
	}
	for _, want := range []string{state, workspace} {
		if !containsPath(gotSpec.WriteRoots, want) {
			t.Fatalf("write root %q missing: %v", want, gotSpec.WriteRoots)
		}
	}
	if !containsPath(gotSpec.ReadRoots, pkg) {
		t.Fatalf("immutable package read root %q missing: %v", pkg, gotSpec.ReadRoots)
	}
	if !containsPath(gotSpec.ForbidReadPaths, credential) {
		t.Fatalf("credential barrier missing: %v", gotSpec.ForbidReadPaths)
	}
	if !containsEnv(gotOpts.Env, "PLUGIN_TOKEN", "explicit") {
		t.Fatalf("child env missing explicit token: %v", gotOpts.Env)
	}
	hostJoined := strings.Join(hostEnv, "\n")
	for _, raw := range []string{"PLUGIN_TOKEN", "explicit"} {
		if strings.Contains(hostJoined, raw) {
			t.Fatalf("trusted wrapper environment exposed raw child data %q: %v", raw, hostEnv)
		}
	}
}

func TestWrapCommandFailsClosedWhenSandboxUnavailable(t *testing.T) {
	home := t.TempDir()
	p := PackagePolicy{Owner: "demo", PackageRoot: t.TempDir(), StateRoot: filepath.Join(home, "plugins", "demo", "state"), HostHome: home}
	previous := commandArgsWithOptions
	defer func() { commandArgsWithOptions = previous }()
	commandArgsWithOptions = func(_ sandbox.Spec, args []string, _ sandbox.CommandOptions) ([]string, bool) { return args, false }
	if _, _, err := p.WrapCommand([]string{"plugin-bin"}, p.ChildEnvironment(nil), "", true); err == nil || !strings.Contains(err.Error(), "refusing to run unconfined") {
		t.Fatalf("unavailable sandbox error = %v", err)
	}
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	wantInfo, _ := os.Stat(want)
	for _, path := range paths {
		path = filepath.Clean(path)
		if runtime.GOOS == "windows" && strings.EqualFold(path, want) {
			return true
		}
		pathInfo, _ := os.Stat(path)
		if wantInfo != nil && pathInfo != nil && os.SameFile(wantInfo, pathInfo) {
			return true
		}
		if path == want {
			return true
		}
	}
	return false
}

func containsEnv(env []string, key, value string) bool {
	want := key + "=" + value
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
