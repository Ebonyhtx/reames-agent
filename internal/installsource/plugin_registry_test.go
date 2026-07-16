package installsource

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/pluginpkg"
	"reames-agent/internal/pluginregistry"
	"reames-agent/internal/tool"
)

func TestSignedRegistryPluginPlanApplyAndPersistEvidence(t *testing.T) {
	reamesHome := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", reamesHome)
	source := filepath.Join(t.TempDir(), "registry-demo")
	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "description": "signed registry fixture",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	writeFile(t, filepath.Join(source, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\nRun demo.\n")
	entry := registryInstallEntry(validRegistrySourceDigest("b"))
	resolver := &fakePluginRegistryResolver{entry: entry}
	restore := stubRegistryClone(t, source)
	defer restore()

	tl := NewTool(Options{PluginRegistry: resolver})
	planned := execInstall(t, tl, map[string]any{"source": "registry:registry-demo", "kind": "plugin"})
	if !planned.OK || planned.Status != "planned" || planned.PlanID == "" || len(planned.Actions) != 1 {
		t.Fatalf("registry plan = %+v", planned)
	}
	action := planned.Actions[0]
	if action.SourceKind != pluginregistry.SourceKind || action.TrustStatus != pluginregistry.TrustStatus || action.RegistryName != "fixture-registry" || action.RegistryRootVersion != 7 || action.RegistryRootDigest == "" || action.RegistryEntryDigest == "" || action.ProvenanceStatus != "tuf-attestation-target-integrity-verified" || action.AttestationDigest == "" {
		t.Fatalf("registry evidence missing from plan: %+v", action)
	}
	applyArgs := mustJSON(map[string]any{
		"source": "registry:registry-demo", "kind": "plugin", "apply": true, "planId": planned.PlanID,
	})
	previewer, ok := tl.(tool.ApprovalPreviewer)
	if !ok {
		t.Fatal("install_source does not expose structured approval")
	}
	approval, required, err := previewer.PreviewApproval(context.Background(), applyArgs)
	if err != nil || !required || len(approval.Actions) != 1 || approval.Actions[0].RegistryEntryDigest != entry.ReleaseEvidenceSHA256 {
		t.Fatalf("registry structured approval = %+v required=%t err=%v", approval, required, err)
	}
	applied := execInstall(t, tl, map[string]any{
		"source": "registry:registry-demo", "kind": "plugin", "apply": true, "planId": planned.PlanID,
	})
	if !applied.OK || applied.Status != "done" {
		t.Fatalf("registry apply = %+v", applied)
	}
	installed, ok, err := pluginpkg.FindInstalled(reamesHome, "registry-demo")
	if err != nil || !ok {
		t.Fatalf("FindInstalled ok=%t err=%v", ok, err)
	}
	if installed.Source != "registry:registry-demo" || installed.SourceKind != pluginregistry.SourceKind || installed.TrustStatus != pluginregistry.TrustStatus || installed.SourceRevision != entry.Revision || installed.RegistryName != entry.RegistryName || installed.RegistryMetadataURL != entry.RegistryMetadataURL || installed.RegistryRootVersion != entry.RootVersion || installed.RegistryRootDigest != entry.BootstrapRootSHA256 || installed.RegistryEntryDigest != entry.ReleaseEvidenceSHA256 || installed.ProvenanceStatus != entry.ProvenanceStatus || installed.AttestationDigest != entry.AttestationSHA256 {
		t.Fatalf("persisted registry evidence = %+v", installed)
	}
}

func TestSignedRegistryPluginRejectsManifestAndDigestMismatch(t *testing.T) {
	source := filepath.Join(t.TempDir(), "registry-demo")
	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	writeFile(t, filepath.Join(source, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\nRun.\n")
	restore := stubRegistryClone(t, source)
	defer restore()

	entry := registryInstallEntry(validRegistrySourceDigest("b"))
	tl := NewTool(Options{PluginRegistry: &fakePluginRegistryResolver{entry: entry}})
	entry.Version = "9.9.9"
	tl = NewTool(Options{PluginRegistry: &fakePluginRegistryResolver{entry: entry}})
	if _, err := tl.Execute(context.Background(), mustJSON(map[string]any{"source": "registry:registry-demo", "kind": "plugin"})); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("signed version mismatch err = %v", err)
	}

	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "other-name",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	entry = registryInstallEntry(validRegistrySourceDigest("b"))
	tl = NewTool(Options{PluginRegistry: &fakePluginRegistryResolver{entry: entry}})
	if _, err := tl.Execute(context.Background(), mustJSON(map[string]any{"source": "registry:registry-demo", "kind": "plugin"})); err == nil || !strings.Contains(err.Error(), "names plugin") {
		t.Fatalf("signed name mismatch err = %v", err)
	}

	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["hooks.execute"]
}`)
	entry = registryInstallEntry(validRegistrySourceDigest("b"))
	tl = NewTool(Options{PluginRegistry: &fakePluginRegistryResolver{entry: entry}})
	if _, err := tl.Execute(context.Background(), mustJSON(map[string]any{"source": "registry:registry-demo", "kind": "plugin"})); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("signed permission mismatch err = %v", err)
	}

	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	entry = registryInstallEntry(validRegistrySourceDigest("b"))
	entry.ReleaseEvidenceSHA256 = ""
	tl = NewTool(Options{PluginRegistry: &fakePluginRegistryResolver{entry: entry}})
	if _, err := tl.Execute(context.Background(), mustJSON(map[string]any{"source": "registry:registry-demo", "kind": "plugin"})); err == nil || !strings.Contains(err.Error(), "evidence digest") {
		t.Fatalf("missing release evidence digest err = %v", err)
	}
}

func TestSignedRegistryPluginRejectsCanonicalSourceDigestMismatch(t *testing.T) {
	source := filepath.Join(t.TempDir(), "registry-demo")
	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	writeFile(t, filepath.Join(source, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\nRun.\n")
	entry := registryInstallEntry(validRegistrySourceDigest("0"))
	restore := stubRegistryCloneWithDigest(t, source, validRegistrySourceDigest("f"))
	defer restore()
	tl := NewTool(Options{PluginRegistry: &fakePluginRegistryResolver{entry: entry}})
	if _, err := tl.Execute(context.Background(), mustJSON(map[string]any{"source": "registry:registry-demo", "kind": "plugin"})); err == nil || !strings.Contains(err.Error(), "source digest verification") {
		t.Fatalf("signed source digest mismatch err = %v", err)
	}
}

func TestSignedRegistryMetadataChangeInvalidatesApprovedPlan(t *testing.T) {
	source := filepath.Join(t.TempDir(), "registry-demo")
	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	writeFile(t, filepath.Join(source, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\nRun.\n")
	resolver := &fakePluginRegistryResolver{entry: registryInstallEntry(validRegistrySourceDigest("b"))}
	restore := stubRegistryClone(t, source)
	defer restore()
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())

	tl := NewTool(Options{PluginRegistry: resolver})
	planned := execInstall(t, tl, map[string]any{"source": "registry:registry-demo", "kind": "plugin"})
	resolver.entry.RootVersion++
	_, err := tl.Execute(context.Background(), mustJSON(map[string]any{
		"source": "registry:registry-demo", "kind": "plugin", "apply": true, "planId": planned.PlanID,
	}))
	if err == nil || !strings.Contains(err.Error(), "planId mismatch") {
		t.Fatalf("changed TUF evidence apply err = %v", err)
	}
}

func TestSignedRegistryEntryDigestChangesPlanID(t *testing.T) {
	source := filepath.Join(t.TempDir(), "registry-demo")
	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	writeFile(t, filepath.Join(source, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\nRun.\n")
	resolver := &fakePluginRegistryResolver{entry: registryInstallEntry(validRegistrySourceDigest("b"))}
	restore := stubRegistryClone(t, source)
	defer restore()
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())

	tl := NewTool(Options{PluginRegistry: resolver})
	first := execInstall(t, tl, map[string]any{"source": "registry:registry-demo", "kind": "plugin"})
	resolver.entry.ReleaseEvidenceSHA256 = "sha256:" + strings.Repeat("e", 64)
	second := execInstall(t, tl, map[string]any{"source": "registry:registry-demo", "kind": "plugin"})
	if first.PlanID == second.PlanID {
		t.Fatalf("planId did not bind registry entry digest: %s", first.PlanID)
	}
	_, err := tl.Execute(context.Background(), mustJSON(map[string]any{
		"source": "registry:registry-demo", "kind": "plugin", "apply": true, "planId": first.PlanID,
	}))
	if err == nil || !strings.Contains(err.Error(), "planId mismatch") {
		t.Fatalf("changed registry entry evidence apply err = %v", err)
	}
}

func TestSignedRegistryEntryDigestChangeDuringApplyIsRejected(t *testing.T) {
	source := filepath.Join(t.TempDir(), "registry-demo")
	writeFile(t, filepath.Join(source, pluginpkg.NativeManifest), `{
  "schemaVersion": 1,
  "name": "registry-demo",
  "version": "1.2.3",
  "skills": ["skills"],
  "permissions": ["skills.load"]
}`)
	writeFile(t, filepath.Join(source, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: demo\n---\nRun.\n")
	initial := registryInstallEntry(validRegistrySourceDigest("b"))
	changed := initial
	changed.ReleaseEvidenceSHA256 = "sha256:" + strings.Repeat("e", 64)
	resolver := &stagedPluginRegistryResolver{initial: initial, changed: changed, changeAfter: 2}
	restore := stubRegistryClone(t, source)
	defer restore()
	t.Setenv("REAMES_AGENT_HOME", t.TempDir())

	tl := NewTool(Options{PluginRegistry: resolver})
	planned := execInstall(t, tl, map[string]any{"source": "registry:registry-demo", "kind": "plugin"})
	applied := execInstall(t, tl, map[string]any{
		"source": "registry:registry-demo", "kind": "plugin", "apply": true, "planId": planned.PlanID,
	})
	if applied.OK || applied.Status != "failed" || len(applied.Actions) != 1 || !strings.Contains(applied.Actions[0].Error, "registry provenance changed after planning") {
		t.Fatalf("mid-apply registry entry change response = %+v", applied)
	}
}

func TestSignedRegistrySourceFailsClosedWithoutConfiguration(t *testing.T) {
	tl := NewTool(Options{})
	_, err := tl.Execute(context.Background(), mustJSON(map[string]any{"source": "registry:demo", "kind": "plugin"}))
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured registry err = %v", err)
	}
}

type fakePluginRegistryResolver struct {
	entry pluginregistry.Entry
}

func (f *fakePluginRegistryResolver) Resolve(_ context.Context, name string) (pluginregistry.Entry, error) {
	if name != f.entry.Name {
		return pluginregistry.Entry{}, context.Canceled
	}
	return f.entry, nil
}

type stagedPluginRegistryResolver struct {
	initial     pluginregistry.Entry
	changed     pluginregistry.Entry
	changeAfter int
	calls       int
}

func (r *stagedPluginRegistryResolver) Resolve(_ context.Context, name string) (pluginregistry.Entry, error) {
	if name != r.initial.Name {
		return pluginregistry.Entry{}, context.Canceled
	}
	r.calls++
	if r.calls > r.changeAfter {
		return r.changed, nil
	}
	return r.initial, nil
}

func registryInstallEntry(digest string) pluginregistry.Entry {
	return pluginregistry.Entry{
		Name: "registry-demo", Version: "1.2.3", Source: "https://github.com/example/registry-demo",
		Revision: strings.Repeat("a", 40), Digest: digest, Permissions: []string{pluginpkg.PermissionSkillsLoad},
		RegistryName: "fixture-registry", RegistryMetadataURL: "https://registry.example/metadata",
		BootstrapRootSHA256: "sha256:" + strings.Repeat("b", 64), RootVersion: 7,
		ReleaseEvidenceSHA256: "sha256:" + strings.Repeat("d", 64),
		ProvenanceStatus:      "tuf-attestation-target-integrity-verified", AttestationSHA256: "sha256:" + strings.Repeat("c", 64),
	}
}

func stubRegistryClone(t *testing.T, source string) func() {
	return stubRegistryCloneWithDigest(t, source, "")
}

func stubRegistryCloneWithDigest(t *testing.T, source, sourceDigest string) func() {
	t.Helper()
	previous := cloneRegistryPluginSource
	cloneRegistryPluginSource = func(_ context.Context, entry pluginregistry.Entry) (preparedPluginSource, func(), error) {
		entryCopy := entry
		verifiedDigest := sourceDigest
		if verifiedDigest == "" {
			verifiedDigest = entry.Digest
		}
		return preparedPluginSource{
			Root: source, Kind: pluginregistry.SourceKind, Revision: entry.Revision,
			Trust: pluginregistry.TrustStatus, Registry: &entryCopy, RegistrySourceDigest: verifiedDigest,
		}, func() {}, nil
	}
	return func() { cloneRegistryPluginSource = previous }
}

func validRegistrySourceDigest(hexDigit string) string {
	return pluginregistry.GitTreeDigestPrefix + strings.Repeat(hexDigit, 64)
}

func mustJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}
