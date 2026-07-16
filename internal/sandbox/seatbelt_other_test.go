//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxWriteDirsSkipsMissingDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := linuxWriteDirs()
	if !containsPath(got, filepath.Join(home, ".cache")) {
		t.Fatalf("existing cache dir missing from linux write dirs: %v", got)
	}
	for _, missing := range []string{".cargo", ".npm", "go"} {
		if containsPath(got, filepath.Join(home, missing)) {
			t.Fatalf("missing dir %s should not be bound: %v", missing, got)
		}
	}
}

func TestStrictBwrapArgsOmitCachesAndInstallChildPolicy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cache := filepath.Join(home, ".cache")
	if err := os.Mkdir(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(home, ".netrc")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := CommandOptions{
		Writable: true, Env: []string{"PATH=/bin", "PLUGIN_TOKEN=explicit"}, Dir: "/tmp",
	}
	child, err := childExecCommand([]string{"/bin/true"}, opts.Env)
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := "/tmp/reames-plugin-generation"
	args := bwrapArgsForArgs(Spec{
		Mode:            "enforce",
		Strict:          true,
		Network:         true,
		ReadRoots:       []string{packageRoot},
		ForbidReadPaths: []string{secret},
	}, child, opts)
	joined := strings.Join(args, "\x00")
	if strings.Contains(joined, cache) {
		t.Fatalf("strict args exposed user cache for writes: %v", args)
	}
	for _, leaked := range []string{"PLUGIN_TOKEN", "explicit"} {
		if strings.Contains(joined, leaked) {
			t.Fatalf("strict wrapper argv exposed child environment %q: %v", leaked, args)
		}
	}
	for _, want := range []string{
		"--unshare-pid", "--die-with-parent", "--ro-bind\x00/dev/null\x00" + secret,
		"--ro-bind\x00" + packageRoot + "\x00" + packageRoot,
		"--ro-bind\x00" + child[0] + "\x00" + child[0],
		"--chdir\x00/tmp\x00" + child[0] + "\x00" + ChildExecHelperCommand + "\x00--\x00/bin/true",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("strict args missing %q: %v", want, args)
		}
	}
	privateTmp := strings.Index(joined, "--tmpfs\x00/tmp")
	readOnlyPackage := strings.Index(joined, "--ro-bind\x00"+packageRoot+"\x00"+packageRoot)
	readOnlyHelper := strings.Index(joined, "--ro-bind\x00"+child[0]+"\x00"+child[0])
	if privateTmp < 0 || readOnlyPackage < privateTmp || readOnlyHelper < privateTmp {
		t.Fatalf("trusted read-only mounts must follow the private /tmp overlay: %v", args)
	}
}

func containsPath(paths []string, want string) bool {
	absWant, err := filepath.Abs(want)
	if err != nil {
		return false
	}
	for _, p := range paths {
		if p == absWant {
			return true
		}
	}
	return false
}
