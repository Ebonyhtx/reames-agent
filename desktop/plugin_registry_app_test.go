package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"reames-agent/internal/pluginpkg"
	"reames-agent/internal/pluginregistry"
)

func TestDesktopPluginRegistryAPIFailsClosedWhenUnconfigured(t *testing.T) {
	isolateDesktopUserDirs(t)
	app := NewApp()

	entries, err := app.SearchPluginRegistry("demo")
	if err == nil || !strings.Contains(err.Error(), "plugin registry") || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("SearchPluginRegistry entries=%+v err=%v, want unconfigured error", entries, err)
	}
	if entries != nil {
		t.Fatalf("SearchPluginRegistry entries = %+v, want nil on failure", entries)
	}

	entry, err := app.PluginRegistryEntry("demo")
	if err == nil || !strings.Contains(err.Error(), "plugin registry") || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("PluginRegistryEntry entry=%+v err=%v, want unconfigured error", entry, err)
	}
	if !reflect.DeepEqual(entry, PluginRegistryEntryView{}) {
		t.Fatalf("PluginRegistryEntry entry = %+v, want zero value on failure", entry)
	}
}

func TestDesktopPluginViewMapsRegistryEvidence(t *testing.T) {
	home := t.TempDir()
	installed := pluginpkg.InstalledPlugin{
		Name:                "registry-demo",
		Version:             "2.3.4",
		Source:              "registry:registry-demo",
		Root:                filepath.Join("plugins", "registry-demo", "versions", "release"),
		SourceKind:          "tuf-registry",
		SourceRevision:      "0123456789abcdef0123456789abcdef01234567",
		TrustStatus:         "tuf-registry-signed",
		RegistryName:        "official",
		RegistryMetadataURL: "https://registry.example/metadata",
		RegistryRootVersion: 12,
		RegistryRootDigest:  "sha256-root",
		RegistryEntryDigest: "sha256-entry",
		ProvenanceStatus:    "tuf-attestation-target-integrity-verified",
		AttestationDigest:   "sha256-attestation",
		Permissions:         []string{pluginpkg.PermissionSkillsLoad},
		Previous: &pluginpkg.PluginRelease{
			Version:             "1.0.0",
			RegistryEntryDigest: "sha256-entry-previous",
		},
	}

	view := pluginViewFromInstalled(home, installed)
	if view.Name != installed.Name || view.Version != installed.Version ||
		view.Source != installed.Source || view.Root != pluginpkg.ResolveRoot(home, installed.Root) ||
		view.SourceKind != installed.SourceKind || view.SourceRevision != installed.SourceRevision ||
		view.TrustStatus != installed.TrustStatus || view.RegistryName != installed.RegistryName ||
		view.RegistryMetadataURL != installed.RegistryMetadataURL ||
		view.RegistryRootVersion != installed.RegistryRootVersion ||
		view.RegistryRootDigest != installed.RegistryRootDigest ||
		view.RegistryEntryDigest != installed.RegistryEntryDigest ||
		view.ProvenanceStatus != installed.ProvenanceStatus ||
		view.AttestationDigest != installed.AttestationDigest {
		t.Fatalf("plugin view registry evidence = %+v, installed = %+v", view, installed)
	}
	if view.Rollback == nil || view.Rollback.RegistryEntryDigest != installed.Previous.RegistryEntryDigest {
		t.Fatalf("rollback registry evidence = %+v", view.Rollback)
	}
	view.Permissions[0] = pluginpkg.PermissionHooksExecute
	if installed.Permissions[0] != pluginpkg.PermissionSkillsLoad {
		t.Fatal("plugin view permissions alias persisted plugin state")
	}
}

func TestDesktopRegistryEntryViewMapsReleaseEvidenceDigest(t *testing.T) {
	entry := pluginregistry.Entry{
		Name: "demo", Version: "1.2.3", ReleaseEvidenceSHA256: "sha256:entry",
		Permissions: []string{pluginpkg.PermissionSkillsLoad},
	}
	view := pluginRegistryEntryView(entry)
	if view.RegistryEntryDigest != entry.ReleaseEvidenceSHA256 {
		t.Fatalf("registry entry digest = %q, want %q", view.RegistryEntryDigest, entry.ReleaseEvidenceSHA256)
	}
	view.Permissions[0] = pluginpkg.PermissionHooksExecute
	if entry.Permissions[0] != pluginpkg.PermissionSkillsLoad {
		t.Fatal("registry entry view permissions alias source entry")
	}
}

func TestDecodePluginOperationPreservesRegistryEvidence(t *testing.T) {
	raw := `{"ok":true,"status":"planned","applied":false,"planId":"sha256:plan","actions":[{"name":"demo","trustStatus":"tuf-registry-signed","registryName":"official","registryMetadataUrl":"https://registry.example/metadata","registryRootVersion":12,"registryRootDigest":"sha256:root","registryEntryDigest":"sha256:entry","provenanceStatus":"tuf-attestation-target-integrity-verified","attestationDigest":"sha256:attestation"}]}`
	operation, err := decodePluginOperation(raw)
	if err != nil {
		t.Fatalf("decodePluginOperation: %v", err)
	}
	if len(operation.Actions) != 1 {
		t.Fatalf("actions = %+v", operation.Actions)
	}
	action := operation.Actions[0]
	if action.RegistryName != "official" ||
		action.RegistryMetadataURL != "https://registry.example/metadata" ||
		action.RegistryRootVersion != 12 ||
		action.RegistryRootDigest != "sha256:root" ||
		action.RegistryEntryDigest != "sha256:entry" ||
		action.ProvenanceStatus != "tuf-attestation-target-integrity-verified" ||
		action.AttestationDigest != "sha256:attestation" {
		t.Fatalf("registry action evidence = %+v", action)
	}
}
