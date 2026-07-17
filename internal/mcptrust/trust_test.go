package mcptrust

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func testCapabilities() []Capability {
	return []Capability{
		{RawName: "read", ModelName: "mcp__srv__read", InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string","description":"display"}}}`), ReadOnly: true},
		{RawName: "write", ModelName: "mcp__srv__write", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
}

func TestIdentityFingerprintIgnoresCredentialValuesButBindsNamesAndExecutable(t *testing.T) {
	base := Identity{Server: "srv", Transport: "stdio", CommandPath: filepath.Join(t.TempDir(), "server"), CommandSHA256: "abc", EnvKeys: []string{"TOKEN"}}
	one, err := IdentityFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	base.EnvKeys = []string{"OTHER_TOKEN"}
	two, err := IdentityFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("changing a credential key name did not change identity")
	}
	base.EnvKeys = []string{"TOKEN"}
	base.CommandSHA256 = "def"
	three, err := IdentityFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	if one == three {
		t.Fatal("changing executable content did not change identity")
	}
}

func TestCapabilityFingerprintIgnoresDisplayTextButBindsSafety(t *testing.T) {
	cap := testCapabilities()[0]
	one, err := CapabilityFingerprint(cap)
	if err != nil {
		t.Fatal(err)
	}
	cap.InputSchema = json.RawMessage(`{"description":"server copy","type":"object","properties":{"q":{"title":"Query","type":"string"}}}`)
	two, err := CapabilityFingerprint(cap)
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatalf("display-only schema changes altered capability fingerprint: %s != %s", one, two)
	}
	cap.Destructive = true
	three, err := CapabilityFingerprint(cap)
	if err != nil {
		t.Fatal(err)
	}
	if one == three {
		t.Fatal("destructive drift did not alter capability fingerprint")
	}
}

func TestReceiptBindsIdentityAndSelectedToolCapabilities(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), StateFilename), t.TempDir())
	caps := testCapabilities()
	if err := manager.TrustReaders(ScopeWorkspace, SourceUser, "srv", "configured", "identity-1", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	eval, err := manager.Evaluate("srv", "configured", "identity-1", caps)
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustWorkspace || !eval.TrustedReaders["read"] || eval.TrustedReaders["write"] {
		t.Fatalf("initial evaluation = %+v", eval)
	}

	drifted := append([]Capability(nil), caps...)
	drifted[0].Destructive = true
	eval, err = manager.Evaluate("srv", "configured", "identity-1", drifted)
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustChanged || eval.TrustedReaders["read"] || len(eval.ChangedTools) != 1 || eval.ChangedTools[0] != "read" {
		t.Fatalf("capability drift evaluation = %+v", eval)
	}

	eval, err = manager.Evaluate("srv", "configured", "identity-2", caps)
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustChanged || !eval.IdentityChanged || len(eval.TrustedReaders) != 0 {
		t.Fatalf("identity drift evaluation = %+v", eval)
	}
}

func TestRemovedCapabilityIsReportedAsDrift(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), StateFilename), t.TempDir())
	caps := testCapabilities()
	if err := manager.TrustReaders(ScopeWorkspace, SourceUser, "srv", "configured", "identity", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	eval, err := manager.Evaluate("srv", "configured", "identity", caps[1:])
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustChanged || eval.TrustedReaders["read"] || len(eval.ToolChanges) != 1 || eval.ToolChanges[0].Name != "read" || eval.ToolChanges[0].Kind != "removed" {
		t.Fatalf("removed capability evaluation = %+v", eval)
	}
}

func TestSessionReceiptOverridesWorkspaceAndRevokeClearsBoth(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), StateFilename), t.TempDir())
	caps := testCapabilities()
	if err := manager.TrustReaders(ScopeWorkspace, SourceUser, "srv", "configured", "identity", caps, nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.TrustReaders(ScopeSession, SourceUser, "srv", "configured", "identity", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	eval, err := manager.Evaluate("srv", "configured", "identity", caps)
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustSession || !eval.TrustedReaders["read"] {
		t.Fatalf("session evaluation = %+v", eval)
	}
	selected, scope, err := manager.SelectedReadersWithScope("srv", "configured")
	if err != nil || scope != ScopeSession || len(selected) != 1 || selected[0] != "read" {
		t.Fatalf("SelectedReadersWithScope = %v, %q, %v", selected, scope, err)
	}
	if err := manager.Revoke("srv"); err != nil {
		t.Fatal(err)
	}
	eval, err = manager.Evaluate("srv", "configured", "identity", caps)
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustUntrusted || len(eval.TrustedReaders) != 0 {
		t.Fatalf("evaluation after revoke = %+v", eval)
	}
}

func TestLegacyReaderImportWritesReceiptAndRevocationMarkerAtomically(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), StateFilename), t.TempDir())
	caps := testCapabilities()
	if err := manager.ImportLegacyReaders("srv", "configured", "identity", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Receipts) != 1 || state.Receipts[0].Source != SourceLegacyImport || len(state.LegacyImports) != 1 {
		t.Fatalf("legacy import state = %+v", state)
	}
	if err := manager.Revoke("srv"); err != nil {
		t.Fatal(err)
	}
	if imported, err := manager.LegacyImported("srv", "configured"); err != nil || !imported {
		t.Fatalf("legacy marker after receipt revocation = %v, %v", imported, err)
	}
}

func TestClearSessionReceiptAllowsWorkspaceReverifyToTakeEffect(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), StateFilename), t.TempDir())
	caps := testCapabilities()
	if err := manager.TrustReaders(ScopeWorkspace, SourceUser, "srv", "configured", "old", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.TrustReaders(ScopeSession, SourceUser, "srv", "configured", "old", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	manager.ClearSessionReceipt("srv", "configured")
	if err := manager.TrustReaders(ScopeWorkspace, SourceUser, "srv", "configured", "new", caps, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	eval, err := manager.Evaluate("srv", "configured", "new", caps)
	if err != nil {
		t.Fatal(err)
	}
	if eval.State != TrustWorkspace || eval.IdentityChanged || !eval.TrustedReaders["read"] {
		t.Fatalf("workspace reverify remained masked by stale session receipt: %+v", eval)
	}
}

func TestLegacyImportMarkerAndLauncherLockAreWorkspaceScoped(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFilename)
	workspace := t.TempDir()
	manager := NewManager(path, workspace)
	if imported, err := manager.LegacyImported("srv", "configured"); err != nil || imported {
		t.Fatalf("LegacyImported before mark = %v, %v", imported, err)
	}
	if err := manager.MarkLegacyImported("srv", "configured"); err != nil {
		t.Fatal(err)
	}
	if imported, err := manager.LegacyImported("srv", "configured"); err != nil || !imported {
		t.Fatalf("LegacyImported after mark = %v, %v", imported, err)
	}
	lock := LauncherLock{Server: "srv", Locator: "locator", ResolvedVersion: "pkg@1.2.3", ContentSHA256: "digest"}
	if err := manager.PutLauncherLock(lock); err != nil {
		t.Fatal(err)
	}
	got, ok, err := manager.GetLauncherLock("srv", "locator")
	if err != nil || !ok || got.ResolvedVersion != lock.ResolvedVersion || got.Workspace != manager.WorkspaceFingerprint() {
		t.Fatalf("GetLauncherLock = %+v, %v, %v", got, ok, err)
	}
	other := NewManager(path, t.TempDir())
	if _, ok, err := other.GetLauncherLock("srv", "locator"); err != nil || ok {
		t.Fatalf("other workspace launcher lock = %v, %v", ok, err)
	}
}
