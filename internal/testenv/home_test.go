package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolateUserStateRedirectsAndRestoresCallerEnvironment(t *testing.T) {
	callerHome := t.TempDir()
	callerEnvironment := map[string]string{
		"HOME":                    callerHome,
		"USERPROFILE":             callerHome,
		"XDG_CONFIG_HOME":         filepath.Join(callerHome, "caller-config"),
		"XDG_CACHE_HOME":          filepath.Join(callerHome, "caller-xdg-cache"),
		"XDG_STATE_HOME":          filepath.Join(callerHome, "caller-xdg-state"),
		"AppData":                 filepath.Join(callerHome, "caller-appdata"),
		"LocalAppData":            filepath.Join(callerHome, "caller-local-appdata"),
		"TEMP":                    filepath.Join(callerHome, "caller-temp"),
		"TMP":                     filepath.Join(callerHome, "caller-tmp"),
		"TMPDIR":                  filepath.Join(callerHome, "caller-tmpdir"),
		"REAMES_AGENT_HOME":       filepath.Join(callerHome, "explicit-reames-home"),
		"REAMES_AGENT_STATE_HOME": filepath.Join(callerHome, "caller-state"),
		"REAMES_AGENT_CACHE_HOME": filepath.Join(callerHome, "caller-cache"),
	}
	for key, value := range callerEnvironment {
		t.Setenv(key, value)
	}

	cleanup, err := IsolateUserState()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	isolateHome := os.Getenv("HOME")
	if isolateHome == "" || isolateHome == callerHome {
		t.Fatalf("HOME = %q, want a disposable home distinct from caller %q", isolateHome, callerHome)
	}
	if rel, err := filepath.Rel(callerHome, isolateHome); err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("isolated home %q is not contained by caller home %q", isolateHome, callerHome)
	}
	wantIsolatedEnvironment := map[string]string{
		"HOME":            isolateHome,
		"USERPROFILE":     isolateHome,
		"XDG_CONFIG_HOME": filepath.Join(isolateHome, ".config"),
		"XDG_CACHE_HOME":  filepath.Join(isolateHome, ".cache"),
		"XDG_STATE_HOME":  filepath.Join(isolateHome, ".local", "state"),
		"AppData":         filepath.Join(isolateHome, "AppData", "Roaming"),
		"LocalAppData":    filepath.Join(isolateHome, "AppData", "Local"),
		"TEMP":            filepath.Join(isolateHome, "tmp"),
		"TMP":             filepath.Join(isolateHome, "tmp"),
		"TMPDIR":          filepath.Join(isolateHome, "tmp"),
	}
	for key, want := range wantIsolatedEnvironment {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s inside isolated test process = %q, want %q", key, got, want)
		}
	}
	for _, key := range []string{"REAMES_AGENT_HOME", "REAMES_AGENT_STATE_HOME", "REAMES_AGENT_CACHE_HOME"} {
		if _, ok := os.LookupEnv(key); ok {
			t.Fatalf("%s remained set inside isolated test process", key)
		}
	}

	cleanup()
	for key, want := range callerEnvironment {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s after cleanup = %q, want %q", key, got, want)
		}
	}
	if _, err := os.Stat(isolateHome); !os.IsNotExist(err) {
		t.Fatalf("isolated home still exists after cleanup: %v", err)
	}
}

func TestIsolateUserStateReusesInheritedIsolationForHelperProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv(isolatedUserStateEnv, "1")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("REAMES_AGENT_HOME", filepath.Join(home, "focused-home"))

	cleanup, err := IsolateUserState()
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	if got := os.Getenv("HOME"); got != home {
		t.Fatalf("helper HOME = %q, want inherited %q", got, home)
	}
	if got := os.Getenv("REAMES_AGENT_HOME"); got != filepath.Join(home, "focused-home") {
		t.Fatalf("helper REAMES_AGENT_HOME = %q, want focused inherited override", got)
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("helper cleanup removed parent-owned isolation root: %v", err)
	}
}
