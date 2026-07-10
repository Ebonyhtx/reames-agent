package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// TestMain isolates user config/state/cache dirs for the whole package. Without
// this, tests that persist desktop state, sessions, cache, or CLI-style config
// can leak into the developer's real Reames Agent directories.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "reamesAgent-desktop-test")
	if err != nil {
		os.Exit(1)
	}
	os.Setenv("HOME", dir)
	os.Setenv("REAMES_AGENT_CREDENTIALS_STORE", "file")
	os.Setenv("USERPROFILE", dir)
	os.Setenv("XDG_CONFIG_HOME", dir+"/config")
	os.Setenv("REAMES_AGENT_STATE_HOME", dir+"/state")
	os.Setenv("REAMES_AGENT_CACHE_HOME", dir+"/cache")
	os.Setenv("AppData", dir)
	// Neutralize the Wails runtime-event bridge for the whole test binary:
	// outside a running Wails app, runtime.EventsEmit log.Fatals on the plain
	// contexts tests use, killing the process from any emitting code path.
	// Tests that assert on runtime events install their own capture through
	// the per-instance runtimeEvents.emit hook, which takes precedence.
	runtimeEventsEmitFallback = func(context.Context, string, ...interface{}) {}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestWindowsWebview2GPUDisabled(t *testing.T) {
	oldChannel := channel
	t.Cleanup(func() {
		channel = oldChannel
		os.Unsetenv(disableWebview2GPUEnv)
	})

	tests := []struct {
		name    string
		channel string
		env     string
		want    bool
	}{
		{name: "stable default keeps gpu", channel: "stable", want: false},
		{name: "canary default disables gpu", channel: "canary", want: true},
		{name: "env enables fallback", channel: "stable", env: "1", want: true},
		{name: "env disables canary fallback", channel: "canary", env: "0", want: false},
		{name: "truthy env", channel: "stable", env: "yes", want: true},
		{name: "falsey env", channel: "canary", env: "off", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel = tt.channel
			if tt.env == "" {
				os.Unsetenv(disableWebview2GPUEnv)
			} else {
				os.Setenv(disableWebview2GPUEnv, tt.env)
			}
			if got := windowsWebview2GPUDisabled(); got != tt.want {
				t.Fatalf("windowsWebview2GPUDisabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinuxWebviewGpuPolicyDisablesGpuWithoutAccessibleRenderNode(t *testing.T) {
	glob := filepath.Join(t.TempDir(), "renderD*")

	if got := linuxWebviewGpuPolicy(glob); got != linux.WebviewGpuPolicyNever {
		t.Fatalf("linuxWebviewGpuPolicy() = %v, want %v", got, linux.WebviewGpuPolicyNever)
	}
}

func TestLinuxWebviewGpuPolicyDisablesGpuForInaccessibleRenderNode(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "renderD128"), 0o700); err != nil {
		t.Fatal(err)
	}

	if got := linuxWebviewGpuPolicy(filepath.Join(dir, "renderD*")); got != linux.WebviewGpuPolicyNever {
		t.Fatalf("linuxWebviewGpuPolicy() = %v, want %v", got, linux.WebviewGpuPolicyNever)
	}
}

func TestLinuxWebviewGpuPolicyKeepsOnDemandWithAccessibleRenderNode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "renderD128"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if got := linuxWebviewGpuPolicy(filepath.Join(dir, "renderD*")); got != linux.WebviewGpuPolicyOnDemand {
		t.Fatalf("linuxWebviewGpuPolicy() = %v, want %v", got, linux.WebviewGpuPolicyOnDemand)
	}
}

func TestParseHomeFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "no home", args: nil, want: ""},
		{name: "no home with other args", args: []string{"-devserver", "localhost:34115"}, want: ""},
		{name: "space separated", args: []string{"--home", "/tmp/my-home"}, want: "/tmp/my-home"},
		{name: "equals form", args: []string{"--home=/tmp/my-home"}, want: "/tmp/my-home"},
		{name: "relative path", args: []string{"--home", "relative/path"}, want: "relative/path"},
		{name: "wails flags before home", args: []string{"-devserver", "localhost:34115", "--home", "/tmp/home"}, want: "/tmp/home"},
		{name: "wails flags after home", args: []string{"--home", "/tmp/home", "-devserver", "localhost:34115"}, want: "/tmp/home"},
		{name: "duplicate flagged", args: []string{"--home", "/a", "--home", "/b"}, wantErr: true},
		{name: "duplicate mixed forms", args: []string{"--home=/a", "--home", "/b"}, wantErr: true},
		{name: "missing value", args: []string{"--home"}, wantErr: true},
		{name: "empty value", args: []string{"--home="}, wantErr: true},
		{name: "blank separated value", args: []string{"--home", "   "}, wantErr: true},
		{name: "blank equals value", args: []string{"--home=   "}, wantErr: true},
		{name: "next long flag is not a value", args: []string{"--home", "--devserver", "localhost:34115"}, wantErr: true},
		{name: "missing value at end", args: []string{"--devserver", "--home"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHomeFlag(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got home=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseHomeFlag(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestConfigureDesktopHomeOverridesEnvironmentAndResolvesRelativePath(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", filepath.Join(t.TempDir(), "old"))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	got, err := configureDesktopHome([]string{"--home", filepath.Join("relative", "home")})
	if err != nil {
		t.Fatalf("configureDesktopHome: %v", err)
	}
	want := filepath.Join(cwd, "relative", "home")
	if got != want {
		t.Fatalf("configured home = %q, want %q", got, want)
	}
	if env := os.Getenv("REAMES_AGENT_HOME"); env != want {
		t.Fatalf("REAMES_AGENT_HOME = %q, want %q", env, want)
	}
}

func TestConfigureDesktopHomeWithoutFlagPreservesEnvironment(t *testing.T) {
	want := filepath.Join(t.TempDir(), "existing")
	t.Setenv("REAMES_AGENT_HOME", want)
	got, err := configureDesktopHome([]string{"-devserver", "localhost:34115"})
	if err != nil {
		t.Fatalf("configureDesktopHome: %v", err)
	}
	if got != "" {
		t.Fatalf("configured home = %q, want no command-line override", got)
	}
	if env := os.Getenv("REAMES_AGENT_HOME"); env != want {
		t.Fatalf("REAMES_AGENT_HOME = %q, want preserved %q", env, want)
	}
}

func TestWindowsWebviewUserDataPathUsesIsolatedHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "isolated")
	t.Setenv("REAMES_AGENT_HOME", home)
	if got, want := windowsWebviewUserDataPath(""), filepath.Join(home, "webview2"); got != want {
		t.Fatalf("windowsWebviewUserDataPath() = %q, want %q", got, want)
	}

	override := filepath.Join(t.TempDir(), "override")
	if got, want := windowsWebviewUserDataPath(override), filepath.Join(override, "webview2"); got != want {
		t.Fatalf("windowsWebviewUserDataPath(override) = %q, want %q", got, want)
	}
}

func TestWindowsWebviewUserDataPathKeepsDefaultForNormalLaunch(t *testing.T) {
	t.Setenv("REAMES_AGENT_HOME", "")
	if got := windowsWebviewUserDataPath(""); got != "" {
		t.Fatalf("windowsWebviewUserDataPath() = %q, want Wails default", got)
	}
}
