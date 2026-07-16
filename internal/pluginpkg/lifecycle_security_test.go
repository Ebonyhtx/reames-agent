package pluginpkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleEnableBindsDigestAndPermissions(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "bound", "1.0.0", []string{PermissionSkillsLoad})
	result := installFixture(t, home, source, "bound", false)

	if err := Enable(home, EnableRequest{
		Name: "bound", ExpectedDigest: result.Installed.Digest + "-stale", GrantedPermissions: result.Installed.Permissions,
	}); err == nil {
		t.Fatal("enable should reject a stale digest")
	}
	if err := Enable(home, EnableRequest{
		Name: "bound", ExpectedDigest: result.Installed.Digest, GrantedPermissions: nil,
	}); err == nil {
		t.Fatal("enable should reject an incomplete permission grant")
	}
	if err := Enable(home, EnableRequest{
		Name: "bound", ExpectedDigest: result.Installed.Digest, GrantedPermissions: result.Installed.Permissions,
	}); err != nil {
		t.Fatalf("enable exact approved package: %v", err)
	}

	manifest := filepath.Join(source, NativeManifest)
	writeNativePluginManifest(t, manifest, "bound", "1.0.1", []string{PermissionSkillsLoad})
	if err := SetEnabled(home, "bound", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := Enable(home, EnableRequest{
		Name: "bound", ExpectedDigest: result.Installed.Digest, GrantedPermissions: result.Installed.Permissions,
	}); err != nil {
		// A copied generation is immutable and must remain unaffected by source
		// changes after install.
		t.Fatalf("source mutation affected copied generation: %v", err)
	}
}

func TestLifecycleMutableLinkRequiresReapprovalAfterChange(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "linked", "1.0.0", []string{PermissionSkillsLoad})
	result, err := Install(home, InstallRequest{
		Name: "linked", Source: source, SourceRoot: source, Mode: InstallModeLink,
	})
	if err != nil {
		t.Fatalf("install link: %v", err)
	}
	writeNativePluginManifest(t, filepath.Join(source, NativeManifest), "linked", "2.0.0", []string{PermissionSkillsLoad})
	if err := Enable(home, EnableRequest{
		Name: "linked", ExpectedDigest: result.Installed.Digest, GrantedPermissions: result.Installed.Permissions,
	}); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("mutable link enable error = %v, want digest mismatch", err)
	}
	installed, ok, err := FindInstalled(home, "linked")
	if err != nil || !ok || installed.Enabled {
		t.Fatalf("linked state after rejected enable = %+v ok=%t err=%v", installed, ok, err)
	}
}

func TestLifecycleMutableLinkUpdateDoesNotCreateFalseRollback(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "linked-update", "1.0.0", []string{PermissionSkillsLoad})
	first, err := Install(home, InstallRequest{
		Name: "linked-update", Source: source, SourceRoot: source, Mode: InstallModeLink,
	})
	if err != nil {
		t.Fatalf("install link v1: %v", err)
	}
	writeNativePluginManifest(t, filepath.Join(source, NativeManifest), "linked-update", "2.0.0", []string{PermissionSkillsLoad})
	secondDigest, err := ContentDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Install(home, InstallRequest{
		Name: "linked-update", Source: source, SourceRoot: source, Mode: InstallModeLink,
		ExpectedDigest: secondDigest, Replace: true,
	})
	if err != nil {
		t.Fatalf("install link v2: %v", err)
	}
	if first.Installed.Digest == second.Installed.Digest || second.Installed.Previous != nil {
		t.Fatalf("mutable link update = %+v, want changed digest without false rollback", second.Installed)
	}
	if _, _, err := Rollback(home, "linked-update"); err == nil {
		t.Fatal("mutable link update should not advertise a restorable previous generation")
	}
}

func TestLifecycleLegacyEnabledStateIsBlockedFromRuntime(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "legacy-runtime", "1.0.0", []string{PermissionSkillsLoad})
	if err := Upsert(home, InstalledPlugin{
		Name: "legacy-runtime", Root: source, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, warnings := LoadInstalled(home)
	if len(loaded) != 0 {
		t.Fatalf("legacy plugin loaded into runtime: %+v", loaded)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "legacy installation is blocked") {
		t.Fatalf("legacy warnings = %v", warnings)
	}
}

func TestLifecyclePublishFailureLeavesNoActivePlugin(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "publish-fail", "1.0.0", []string{PermissionSkillsLoad})
	original := publishPluginGeneration
	publishPluginGeneration = func(string, string, string) error { return errors.New("injected publish failure") }
	t.Cleanup(func() { publishPluginGeneration = original })

	if _, err := Install(home, InstallRequest{Name: "publish-fail", Source: source, SourceRoot: source}); err == nil {
		t.Fatal("install should fail")
	}
	if installed, ok, err := FindInstalled(home, "publish-fail"); err != nil || ok {
		t.Fatalf("active plugin after publish failure = %+v ok=%t err=%v", installed, ok, err)
	}
	entries, err := os.ReadDir(filepath.Join(InstallRoot(home, "publish-fail"), "versions"))
	if err != nil {
		t.Fatalf("read versions: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("published generations after failure = %d, want 0", len(entries))
	}
}

func TestLifecyclePublishSyncFailureCleansPublishedOrphan(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "sync-fail", "1.0.0", []string{PermissionSkillsLoad})
	original := publishPluginGeneration
	publishPluginGeneration = func(home, staged, destination string) error {
		if err := original(home, staged, destination); err != nil {
			return err
		}
		return errors.New("injected directory sync failure")
	}
	t.Cleanup(func() { publishPluginGeneration = original })

	if _, err := Install(home, InstallRequest{Name: "sync-fail", Source: source, SourceRoot: source}); err == nil {
		t.Fatal("install should fail")
	}
	if _, ok, err := FindInstalled(home, "sync-fail"); err != nil || ok {
		t.Fatalf("active plugin after sync failure: ok=%t err=%v", ok, err)
	}
	entries, err := os.ReadDir(filepath.Join(InstallRoot(home, "sync-fail"), "versions"))
	if err != nil {
		t.Fatalf("read versions: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("orphan generations after sync failure = %d", len(entries))
	}
}

func TestLifecycleRejectsStagingPathReplacementBeforePublish(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "stage-race", "1.0.0", []string{PermissionSkillsLoad})
	original := beforePluginPublish
	beforePluginPublish = func(_, stage string) error {
		moved := stage + ".moved"
		if err := os.Rename(stage, moved); err != nil {
			return err
		}
		return os.Mkdir(stage, 0o700)
	}
	t.Cleanup(func() { beforePluginPublish = original })

	if _, err := Install(home, InstallRequest{Name: "stage-race", Source: source, SourceRoot: source}); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("install error = %v, want staging identity rejection", err)
	}
	if _, ok, err := FindInstalled(home, "stage-race"); err != nil || ok {
		t.Fatalf("active plugin after staging replacement: ok=%t err=%v", ok, err)
	}
	entries, err := os.ReadDir(filepath.Join(InstallRoot(home, "stage-race"), "versions"))
	if err != nil {
		t.Fatalf("read versions: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("published generations after staging replacement = %d", len(entries))
	}
}

func TestLifecycleCopyFailureLeavesNoGeneration(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "copy-fail", "1.0.0", []string{PermissionSkillsLoad})
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(source, "redirect.txt")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := Install(home, InstallRequest{Name: "copy-fail", Source: source, SourceRoot: source}); err == nil {
		t.Fatal("install should reject source symlink")
	}
	if _, ok, err := FindInstalled(home, "copy-fail"); err != nil || ok {
		t.Fatalf("active plugin after copy failure: ok=%t err=%v", ok, err)
	}
	entries, err := os.ReadDir(filepath.Join(InstallRoot(home, "copy-fail"), "versions"))
	if err != nil {
		t.Fatalf("read versions: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("generations after copy failure = %d", len(entries))
	}
}

func TestLifecycleTamperSkipsEnabledPluginAtRuntime(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "tampered", "1.0.0", []string{PermissionSkillsLoad})
	installed := installFixture(t, home, source, "tampered", false).Installed
	if err := Enable(home, EnableRequest{
		Name: installed.Name, ExpectedDigest: installed.Digest, GrantedPermissions: installed.Permissions,
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	root := ResolveRoot(home, installed.Root)
	if err := os.WriteFile(filepath.Join(root, "skills", "tampered", "SKILL.md"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, warnings := LoadInstalled(home)
	if len(loaded) != 0 {
		t.Fatalf("tampered plugin loaded: %+v", loaded)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "digest mismatch") {
		t.Fatalf("tamper warnings = %v", warnings)
	}
}

func TestLifecycleVerificationRejectsContentChangingMidRead(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "verify-race", "1.0.0", []string{PermissionSkillsLoad})
	installed := installFixture(t, home, source, "verify-race", false).Installed
	target := filepath.Join(ResolveRoot(home, installed.Root), "skills", "verify-race", "SKILL.md")
	original := digestInstalledContent
	calls := 0
	digestInstalledContent = func(root string) (string, error) {
		calls++
		digest, err := original(root)
		if calls == 1 && err == nil {
			if writeErr := os.WriteFile(target, []byte("changed during verification\n"), 0o644); writeErr != nil {
				return "", writeErr
			}
		}
		return digest, err
	}
	t.Cleanup(func() { digestInstalledContent = original })
	if _, err := VerifyInstalled(home, "verify-race"); err == nil || !strings.Contains(err.Error(), "changed during verification") {
		t.Fatalf("VerifyInstalled error = %v, want mid-verification change", err)
	}
}

func TestLifecycleStateWriteFailurePreservesPreviousGeneration(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "state-fail", "1.0.0", []string{PermissionSkillsLoad})
	first := installFixture(t, home, source, "state-fail", false)
	if err := Enable(home, EnableRequest{
		Name: "state-fail", ExpectedDigest: first.Installed.Digest, GrantedPermissions: first.Installed.Permissions,
	}); err != nil {
		t.Fatalf("enable v1: %v", err)
	}
	writeNativePluginManifest(t, filepath.Join(source, NativeManifest), "state-fail", "2.0.0", []string{PermissionSkillsLoad})

	original := writeStateFile
	writeStateFile = func(string, []byte, os.FileMode) error { return errors.New("injected state failure") }
	t.Cleanup(func() { writeStateFile = original })
	if _, err := Install(home, InstallRequest{
		Name: "state-fail", Source: source, SourceRoot: source, Replace: true,
	}); err == nil {
		t.Fatal("update should fail")
	}
	writeStateFile = original
	active, ok, err := FindInstalled(home, "state-fail")
	if err != nil || !ok || active.Version != "1.0.0" || !active.Enabled || active.Digest != first.Installed.Digest {
		t.Fatalf("active after failed state write = %+v ok=%t err=%v", active, ok, err)
	}
}

func TestLifecycleRollbackStateWriteFailureKeepsCurrent(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "rollback-fail", "1.0.0", []string{PermissionSkillsLoad})
	first := installFixture(t, home, source, "rollback-fail", false)
	if err := Enable(home, EnableRequest{
		Name: "rollback-fail", ExpectedDigest: first.Installed.Digest, GrantedPermissions: first.Installed.Permissions,
	}); err != nil {
		t.Fatalf("enable v1: %v", err)
	}
	writeNativePluginManifest(t, filepath.Join(source, NativeManifest), "rollback-fail", "2.0.0", []string{PermissionSkillsLoad})
	second := installFixture(t, home, source, "rollback-fail", true)

	original := writeStateFile
	writeStateFile = func(string, []byte, os.FileMode) error { return errors.New("injected state failure") }
	t.Cleanup(func() { writeStateFile = original })
	if _, _, err := Rollback(home, "rollback-fail"); err == nil {
		t.Fatal("rollback should fail")
	}
	writeStateFile = original
	active, ok, err := FindInstalled(home, "rollback-fail")
	if err != nil || !ok || active.Version != "2.0.0" || active.Digest != second.Installed.Digest {
		t.Fatalf("active after failed rollback = %+v ok=%t err=%v", active, ok, err)
	}
}

func TestRuntimeStateSurvivesGenerationChangesAndUninstallRemovesIt(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "runtime-state", "1.0.0", []string{PermissionSkillsLoad})
	first := installFixture(t, home, source, "runtime-state", false)
	if err := Enable(home, EnableRequest{
		Name: first.Installed.Name, ExpectedDigest: first.Installed.Digest, GrantedPermissions: first.Installed.Permissions,
	}); err != nil {
		t.Fatal(err)
	}
	state := RuntimeStateDir(home, "runtime-state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(state, "index.json")
	if err := os.WriteFile(marker, []byte("persistent"), 0o600); err != nil {
		t.Fatal(err)
	}

	writeNativePluginManifest(t, filepath.Join(source, NativeManifest), "runtime-state", "2.0.0", []string{PermissionSkillsLoad})
	installFixture(t, home, source, "runtime-state", true)
	if body, err := os.ReadFile(marker); err != nil || string(body) != "persistent" {
		t.Fatalf("runtime state after update = %q err=%v", body, err)
	}
	if _, _, err := Rollback(home, "runtime-state"); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "persistent" {
		t.Fatalf("runtime state after rollback = %q err=%v", body, err)
	}
	if _, _, found, err := Uninstall(home, "runtime-state"); err != nil || !found {
		t.Fatalf("uninstall found=%t err=%v", found, err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("runtime state survived uninstall: %v", err)
	}
}

func TestLifecycleUninstallStateFailureKeepsContentAndState(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "remove-state-fail", "1.0.0", []string{PermissionSkillsLoad})
	installed := installFixture(t, home, source, "remove-state-fail", false).Installed
	root := ResolveRoot(home, installed.Root)

	original := writeStateFile
	writeStateFile = func(string, []byte, os.FileMode) error { return errors.New("injected state failure") }
	t.Cleanup(func() { writeStateFile = original })
	if _, _, _, err := Uninstall(home, "remove-state-fail"); err == nil {
		t.Fatal("uninstall should fail")
	}
	writeStateFile = original
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("active content removed after state failure: %v", err)
	}
	if _, ok, err := FindInstalled(home, "remove-state-fail"); err != nil || !ok {
		t.Fatalf("state removed after failed uninstall: ok=%t err=%v", ok, err)
	}
}

func TestLifecycleUninstallCleanupFailureLeavesOnlyInactiveContent(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "remove-cleanup-fail", "1.0.0", []string{PermissionSkillsLoad})
	installed := installFixture(t, home, source, "remove-cleanup-fail", false).Installed
	root := ResolveRoot(home, installed.Root)

	original := removeManagedPluginPath
	removeManagedPluginPath = func(string, string) error { return errors.New("injected cleanup failure") }
	t.Cleanup(func() { removeManagedPluginPath = original })
	_, warnings, found, err := Uninstall(home, "remove-cleanup-fail")
	if err != nil || !found || len(warnings) == 0 {
		t.Fatalf("uninstall = found=%t warnings=%v err=%v", found, warnings, err)
	}
	removeManagedPluginPath = original
	if _, ok, err := FindInstalled(home, "remove-cleanup-fail"); err != nil || ok {
		t.Fatalf("plugin remains active after cleanup failure: ok=%t err=%v", ok, err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("expected inactive orphan to remain: %v", err)
	}
}

func TestLifecycleUninstallApprovedRejectsStateChange(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "remove-race", "1.0.0", []string{PermissionSkillsLoad})
	installed := installFixture(t, home, source, "remove-race", false).Installed
	approvedState := InstalledStateToken(installed)
	if err := Enable(home, EnableRequest{
		Name: installed.Name, ExpectedDigest: installed.Digest, GrantedPermissions: installed.Permissions,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := UninstallApproved(home, UninstallRequest{
		Name: installed.Name, ExpectedCurrentState: approvedState, BindCurrentState: true,
	}); err == nil || !strings.Contains(err.Error(), "state changed after approval") {
		t.Fatalf("uninstall stale state error = %v", err)
	}
	current, ok, err := FindInstalled(home, installed.Name)
	if err != nil || !ok || !current.Enabled {
		t.Fatalf("plugin changed after rejected uninstall: current=%+v ok=%t err=%v", current, ok, err)
	}
}

func TestLifecycleUninstallApprovedRejectsConcurrentRemoval(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "remove-missing-race", "1.0.0", []string{PermissionSkillsLoad})
	installed := installFixture(t, home, source, "remove-missing-race", false).Installed
	approvedState := InstalledStateToken(installed)
	if _, _, found, err := Uninstall(home, installed.Name); err != nil || !found {
		t.Fatalf("concurrent uninstall = found=%t err=%v", found, err)
	}
	if _, _, _, err := UninstallApproved(home, UninstallRequest{
		Name: installed.Name, ExpectedCurrentState: approvedState, BindCurrentState: true,
	}); err == nil || !strings.Contains(err.Error(), "no longer installed") {
		t.Fatalf("approved uninstall after concurrent removal error = %v", err)
	}
}

func TestLifecycleRejectsInvalidUninstallNameWithoutExternalDeletion(t *testing.T) {
	home := t.TempDir()
	external := filepath.Join(filepath.Dir(home), "outside-marker.txt")
	if err := os.WriteFile(external, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(external) })
	if _, _, _, err := Uninstall(home, ".."); err == nil {
		t.Fatal("invalid uninstall name should fail")
	}
	if raw, err := os.ReadFile(external); err != nil || string(raw) != "keep" {
		t.Fatalf("external marker changed: %q err=%v", raw, err)
	}
}

func TestLifecycleRejectsManagedDirectorySymlink(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(PluginsDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, InstallRoot(home, "escaped")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	source := newNativePluginFixture(t, "escaped", "1.0.0", []string{PermissionSkillsLoad})
	if _, err := Install(home, InstallRequest{Name: "escaped", Source: source, SourceRoot: source}); err == nil {
		t.Fatal("install through managed directory symlink should fail")
	}
	entries, err := os.ReadDir(external)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("external directory modified through symlink: %v", entries)
	}
}

func TestLoadStateRejectsDuplicateAndInvalidNames(t *testing.T) {
	for _, body := range []string{
		`{"version":2,"plugins":[{"name":"../escape","root":"x","enabled":false}]}`,
		`{"version":2,"plugins":[{"name":"same","root":"a","enabled":false},{"name":"same","root":"b","enabled":false}]}`,
	} {
		home := t.TempDir()
		if err := os.WriteFile(StatePath(home), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadState(home); err == nil {
			t.Fatalf("LoadState accepted invalid state: %s", body)
		}
	}
}

func newNativePluginFixture(t *testing.T, name, version string, permissions []string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	writeNativePluginManifest(t, filepath.Join(root, NativeManifest), name, version, permissions)
	if err := os.MkdirAll(filepath.Join(root, "skills", name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", name, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: fixture\n---\nRun fixture.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeNativePluginManifest(t *testing.T, path, name, version string, permissions []string) {
	t.Helper()
	quoted := make([]string, len(permissions))
	for i, permission := range permissions {
		quoted[i] = fmt.Sprintf("%q", permission)
	}
	body := fmt.Sprintf(`{
  "schemaVersion": 1,
  "name": %q,
  "version": %q,
  "description": "security fixture",
  "skills": ["skills"],
  "permissions": [%s]
}`, name, version, strings.Join(quoted, ","))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func installFixture(t *testing.T, home, source, name string, replace bool) InstallResult {
	t.Helper()
	result, err := Install(home, InstallRequest{
		Name: name, Source: source, SourceRoot: source, Replace: replace,
	})
	if err != nil {
		t.Fatalf("install %s: %v", name, err)
	}
	return result
}
