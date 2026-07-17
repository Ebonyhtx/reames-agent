package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/mcptrust"
	"reames-agent/internal/tool"
)

func identityBoundHelperSpec(t *testing.T, manager *mcptrust.Manager) Spec {
	t.Helper()
	return Spec{
		Name: "identity-bound", Command: os.Args[0],
		Args:              []string{"-test.run=TestHelperProcess", "--"},
		Env:               map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
		ReadOnlyToolNames: map[string]bool{"echo": true},
		TrustManager:      manager, ConfigSource: "configured",
	}
}

func TestLegacyReadOnlyOverrideMigratesOnceToIdentityBoundReceipt(t *testing.T) {
	manager := mcptrust.NewManager(filepath.Join(t.TempDir(), mcptrust.StateFilename), t.TempDir())
	spec := identityBoundHelperSpec(t, manager)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host, tools, err := Start(ctx, []Spec{spec}, StartPolicy{PerPluginTimeout: 5 * time.Second, Concurrency: 1, AbortOnError: true})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	var echo tool.Tool
	for _, candidate := range tools {
		if candidate.Name() == "mcp__identity-bound__echo" {
			echo = candidate
		}
	}
	if echo == nil || !echo.ReadOnly() {
		t.Fatalf("legacy reader was not imported into an identity-bound receipt: %T %+v", echo, host.Servers())
	}
	state, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Receipts) != 1 || state.Receipts[0].Source != mcptrust.SourceLegacyImport || !state.Receipts[0].Tools[0].TrustedReader && !state.Receipts[0].Tools[1].TrustedReader {
		t.Fatalf("legacy receipt = %+v", state.Receipts)
	}

	changed := spec
	changed.Args = append(append([]string(nil), spec.Args...), "changed-launch-semantics")
	if _, _, err := Start(ctx, []Spec{changed}, StartPolicy{PerPluginTimeout: 5 * time.Second, Concurrency: 1, AbortOnError: true}); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("changed executable identity start error = %v", err)
	}
}

func TestCachedTrustedReaderIsBlockedWhenLiveCapabilityLosesTrust(t *testing.T) {
	shared := &lazySpawn{
		spec: Spec{Name: "srv"}, state: spawnReady,
		real: map[string]tool.Tool{},
	}
	lazy := &lazyTool{
		shared: shared, name: "mcp__srv__read", rawName: "read", hasCache: true,
		declaredReadOnly: true, readOnly: true, readOnlyTrusted: true,
	}
	real := &remoteTool{
		client: &Client{name: "srv"}, name: lazy.name, rawName: "read",
		declaredReadOnly: false, readOnly: false, readOnlyTrusted: false,
	}
	shared.real[lazy.name] = real
	_, err := lazy.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "blocked before dispatch") {
		t.Fatalf("cached capability drift error = %v", err)
	}
	if lazy.ReadOnly() {
		t.Fatal("lazy adapter retained reader authority after live drift")
	}
}

func TestCachedWriterIsBlockedWhenLiveToolBecomesDestructive(t *testing.T) {
	shared := &lazySpawn{
		spec: Spec{Name: "srv"}, state: spawnReady,
		real: map[string]tool.Tool{},
	}
	lazy := &lazyTool{
		shared: shared, name: "mcp__srv__write", rawName: "write", hasCache: true,
	}
	real := &remoteTool{
		name: lazy.name, rawName: "write", destructive: true,
	}
	shared.real[lazy.name] = real
	_, err := lazy.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "changed tool \"write\" to destructive") {
		t.Fatalf("cached writer destructive drift error = %v", err)
	}
	if !lazy.MCPDestructiveHint() {
		t.Fatal("lazy adapter did not retain the live destructive posture for the next approval")
	}
}

func TestSpecFingerprintRedactsCredentialURLValues(t *testing.T) {
	one := Spec{Name: "remote", Type: "http", URL: "https://user:secret@example.com/mcp?api_key=one&workspace=a"}
	two := one
	two.URL = "https://other:rotated@example.com:443/mcp?workspace=a&api_key=two"
	if SpecFingerprint(one) != SpecFingerprint(two) {
		t.Fatal("credential rotation changed MCP schema cache identity")
	}
	two.URL = "https://other:rotated@example.com:443/mcp?workspace=b&api_key=two"
	if SpecFingerprint(one) == SpecFingerprint(two) {
		t.Fatal("non-credential resource scope did not change MCP schema cache identity")
	}
}

func TestPersistentTrustRejectsInsecureRemoteTransport(t *testing.T) {
	err := validatePersistentTransportTrust(Spec{Name: "remote", Type: "http", URL: "http://example.com/mcp"})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("persistent HTTP trust error = %v", err)
	}
}

func TestDirectWorkspaceTrustRequiresContentLockEvenForExactLauncherVersion(t *testing.T) {
	manager := mcptrust.NewManager(filepath.Join(t.TempDir(), mcptrust.StateFilename), t.TempDir())
	capabilities := []mcptrust.Capability{{RawName: "read", ModelName: "mcp__srv__read", ReadOnly: true}}
	client := &Client{
		name: "srv", identity: "identity", capabilities: capabilities,
		spec: Spec{Name: "srv", Command: "npx", Args: []string{"@scope/server@1.2.3"}, TrustManager: manager, ConfigSource: "configured"},
	}
	host := &Host{clients: []*Client{client}}
	err := host.SetReaderTrust("srv", mcptrust.ScopeWorkspace, []string{"read"})
	if err == nil || !strings.Contains(err.Error(), "identity-bound trust preflight") {
		t.Fatalf("exact launcher direct workspace trust error = %v", err)
	}
}

func TestExplicitWorkspaceTrustReplacesOlderSessionSelection(t *testing.T) {
	manager := mcptrust.NewManager(filepath.Join(t.TempDir(), mcptrust.StateFilename), t.TempDir())
	capabilities := []mcptrust.Capability{{RawName: "read", ModelName: "mcp__srv__read", ReadOnly: true}}
	if err := manager.TrustReaders(mcptrust.ScopeSession, mcptrust.SourceUser, "srv", "configured", "identity", capabilities, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	client := &Client{
		name: "srv", identity: "identity", capabilities: capabilities,
		spec:      Spec{Name: "srv", Command: os.Args[0], TrustManager: manager, ConfigSource: "configured"},
		trustEval: mcptrust.Evaluation{State: mcptrust.TrustSession, TrustedReaders: map[string]bool{"read": true}},
	}
	host := &Host{clients: []*Client{client}}
	if err := host.SetReaderTrust("srv", mcptrust.ScopeWorkspace, nil); err != nil {
		t.Fatal(err)
	}
	selected, scope, err := manager.SelectedReadersWithScope("srv", "configured")
	if err != nil {
		t.Fatal(err)
	}
	if scope != mcptrust.ScopeWorkspace || len(selected) != 0 {
		t.Fatalf("selected readers after workspace revocation = %v scope=%q", selected, scope)
	}
	if client.trustEval.State != mcptrust.TrustWorkspace || len(client.trustEval.TrustedReaders) != 0 {
		t.Fatalf("client trust after workspace revocation = %+v", client.trustEval)
	}
}

func TestSpecIdentityBindsInterpreterScriptContent(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "server.js")
	if err := os.WriteFile(script, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := Spec{Name: "script", Command: os.Args[0], Args: []string{script}}
	one, err := specIdentityFingerprint(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	two, err := specIdentityFingerprint(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("changing an interpreter script did not change MCP identity")
	}
}

func TestHasStoredReaderSelectionUsesReceiptInsteadOfRawAliasAfterMigration(t *testing.T) {
	manager := mcptrust.NewManager(filepath.Join(t.TempDir(), mcptrust.StateFilename), t.TempDir())
	spec := Spec{Name: "srv", ConfigSource: "configured", TrustManager: manager, ReadOnlyToolNames: map[string]bool{"read": true}}
	if !HasStoredReaderSelection(spec) {
		t.Fatal("unmigrated legacy reader alias should allow one connection preflight")
	}
	capabilities := []mcptrust.Capability{{RawName: "read", ModelName: "mcp__srv__read", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true}}
	if err := manager.TrustReaders(mcptrust.ScopeWorkspace, mcptrust.SourceUser, "srv", "configured", "identity", capabilities, nil); err != nil {
		t.Fatal(err)
	}
	if HasStoredReaderSelection(spec) {
		t.Fatal("raw alias overrode an explicit receipt with no selected readers")
	}
	if err := manager.TrustReaders(mcptrust.ScopeWorkspace, mcptrust.SourceUser, "srv", "configured", "identity", capabilities, []string{"read"}); err != nil {
		t.Fatal(err)
	}
	if !HasStoredReaderSelection(spec) {
		t.Fatal("identity-bound selected reader was not visible to on-demand resolution")
	}
}

func TestEligibleSelectedReadersDropsRemovedWriterAndDestructiveTools(t *testing.T) {
	capabilities := []mcptrust.Capability{
		{RawName: "read", ReadOnly: true},
		{RawName: "write"},
		{RawName: "delete", ReadOnly: true, Destructive: true},
	}
	got := eligibleSelectedReaders(capabilities, []string{"missing", "delete", "write", "read", "read"})
	if len(got) != 1 || got[0] != "read" {
		t.Fatalf("eligible selected readers = %v, want [read]", got)
	}
}
