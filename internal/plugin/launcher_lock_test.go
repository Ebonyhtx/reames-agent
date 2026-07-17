package plugin

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"reames-agent/internal/mcptrust"
)

func TestPreparePersistentLauncherBindsExactVersionContentToo(t *testing.T) {
	original := resolveLauncherLocatorForTrust
	resolveLauncherLocatorForTrust = func(context.Context, Spec, launcherLocator) (string, string, error) {
		return "@scope/server@1.2.3", "integrity-digest", nil
	}
	t.Cleanup(func() { resolveLauncherLocatorForTrust = original })
	manager := mcptrust.NewManager(filepath.Join(t.TempDir(), mcptrust.StateFilename), t.TempDir())
	spec := Spec{Name: "server", Command: "npx", Args: []string{"@scope/server@1.2.3"}, TrustManager: manager}
	locked, lock, err := preparePersistentLauncher(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if lock == nil || lock.ContentSHA256 != "integrity-digest" || locked.LauncherDigest == "" {
		t.Fatalf("exact launcher content lock = %+v, spec=%+v", lock, locked)
	}
}

func TestMutableLauncherLocatorRequiresExactPins(t *testing.T) {
	for _, tc := range []struct {
		spec      Spec
		mutable   bool
		immutable bool
	}{
		{Spec{Command: "npx", Args: []string{"-y", "@scope/server"}}, true, false},
		{Spec{Command: "npx", Args: []string{"@scope/server@1.2.3"}}, true, true},
		{Spec{Command: "uvx", Args: []string{"--from", "server==2.4.1", "server"}}, true, true},
		{Spec{Command: "uvx", Args: []string{"server==2.*"}}, true, false},
		{Spec{Command: "npx", Args: []string{"git+https://example.com/server.git@" + strings.Repeat("a", 40)}}, true, true},
		{Spec{Command: "npx", Args: []string{"git+ssh://example.com/server.git@" + strings.Repeat("a", 40)}}, true, false},
		{Spec{Command: "node", Args: []string{"server.js"}}, false, false},
	} {
		locator, mutable := mutableLauncherLocator(tc.spec)
		if mutable != tc.mutable {
			t.Errorf("mutableLauncherLocator(%+v) mutable = %v, want %v", tc.spec, mutable, tc.mutable)
			continue
		}
		if mutable && immutableLauncherLocator(locator) != tc.immutable {
			t.Errorf("immutableLauncherLocator(%+v) = %v, want %v", locator, immutableLauncherLocator(locator), tc.immutable)
		}
	}
}

func TestResolveGitLocatorRejectsNonHTTPSBeforeNetwork(t *testing.T) {
	locator := "git+ssh://example.com/server.git@" + strings.Repeat("a", 40)
	if _, _, err := resolveGitLocator(context.Background(), Spec{}, locator); err == nil || !strings.Contains(err.Error(), "git+https") {
		t.Fatalf("non-HTTPS git locator error = %v", err)
	}
}

func TestApplyLauncherResolutionPinsExactArgsAndOfflineMode(t *testing.T) {
	spec := Spec{Name: "search", Command: "npx", Args: []string{"-y", "@scope/server", "--stdio"}}
	locator, ok := mutableLauncherLocator(spec)
	if !ok {
		t.Fatal("npx locator not detected")
	}
	lock := launcherLockFixture(spec.Name, "@scope/server@1.2.3")
	applyLauncherResolution(&spec, locator, lock, true)
	if len(spec.LaunchArgs) != 4 || spec.LaunchArgs[1] != "--offline" || spec.LaunchArgs[2] != "@scope/server@1.2.3" {
		t.Fatalf("locked launch args = %q", spec.LaunchArgs)
	}
	if spec.LauncherDigest == "" {
		t.Fatal("launcher digest was not bound into the spec identity")
	}
}

func TestStoredOfflineLauncherKeepsPreflightIdentityArgs(t *testing.T) {
	base := Spec{Name: "search", Command: "npx", Args: []string{"-y", "@scope/server", "--stdio"}}
	locator, ok := mutableLauncherLocator(base)
	if !ok {
		t.Fatal("npx locator not detected")
	}
	lock := launcherLockFixture(base.Name, "@scope/server@1.2.3")
	preflight := base
	applyLauncherResolution(&preflight, locator, lock, false)
	offline := base
	applyLauncherResolution(&offline, locator, lock, true)
	if got, want := identityLaunchArgs(offline), identityLaunchArgs(preflight); !slices.Equal(got, want) {
		t.Fatalf("offline identity args = %q, preflight = %q", got, want)
	}
}

func launcherLockFixture(server, version string) mcptrust.LauncherLock {
	return mcptrust.LauncherLock{Server: server, Workspace: "workspace", Locator: "locator", ResolvedVersion: version, ContentSHA256: "content"}
}
