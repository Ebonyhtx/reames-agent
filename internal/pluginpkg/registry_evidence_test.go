package pluginpkg

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleUpdateAndRollbackPreserveRegistryEvidence(t *testing.T) {
	home := t.TempDir()
	source := newNativePluginFixture(t, "registry-evidence", "1.0.0", []string{PermissionSkillsLoad})

	install := func(version string, replace bool, rootVersion int64) InstalledPlugin {
		t.Helper()
		result, err := Install(home, InstallRequest{
			Name:                "registry-evidence",
			Source:              "registry:registry-evidence",
			SourceRoot:          source,
			SourceKind:          "tuf-registry",
			SourceRevision:      strings.Repeat(version[:1], 40),
			TrustStatus:         "tuf-registry-signed",
			RegistryName:        "fixture-registry-" + version,
			RegistryMetadataURL: "https://registry.example/metadata/" + version,
			RegistryRootVersion: rootVersion,
			RegistryRootDigest:  "sha256-root-" + version,
			RegistryEntryDigest: "sha256-entry-" + version,
			ProvenanceStatus:    "tuf-attestation-target-integrity-verified-" + version,
			AttestationDigest:   "sha256-attestation-" + version,
			Replace:             replace,
		})
		if err != nil {
			t.Fatalf("install %s: %v", version, err)
		}
		return result.Installed
	}
	assertEvidence := func(label string, installed InstalledPlugin, version string, rootVersion int64) {
		t.Helper()
		if installed.Source != "registry:registry-evidence" ||
			installed.SourceKind != "tuf-registry" ||
			installed.SourceRevision != strings.Repeat(version[:1], 40) ||
			installed.TrustStatus != "tuf-registry-signed" ||
			installed.RegistryName != "fixture-registry-"+version ||
			installed.RegistryMetadataURL != "https://registry.example/metadata/"+version ||
			installed.RegistryRootVersion != rootVersion ||
			installed.RegistryRootDigest != "sha256-root-"+version ||
			installed.RegistryEntryDigest != "sha256-entry-"+version ||
			installed.ProvenanceStatus != "tuf-attestation-target-integrity-verified-"+version ||
			installed.AttestationDigest != "sha256-attestation-"+version {
			t.Fatalf("%s evidence = %+v", label, installed)
		}
	}
	assertReleaseEvidence := func(label string, release *PluginRelease, version string, rootVersion int64) {
		t.Helper()
		if release == nil {
			t.Fatalf("%s release is nil", label)
		}
		assertEvidence(label, installedFromRelease("registry-evidence", *release), version, rootVersion)
	}

	first := install("1.0.0", false, 7)
	assertEvidence("first install", first, "1.0.0", 7)

	writeNativePluginManifest(t, filepath.Join(source, NativeManifest), "registry-evidence", "2.0.0", []string{PermissionSkillsLoad})
	second := install("2.0.0", true, 9)
	assertEvidence("updated release", second, "2.0.0", 9)
	assertReleaseEvidence("rollback release after update", second.Previous, "1.0.0", 7)

	restored, _, err := Rollback(home, "registry-evidence")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertEvidence("restored release", restored, "1.0.0", 7)
	assertReleaseEvidence("forward release after rollback", restored.Previous, "2.0.0", 9)

	persisted, ok, err := FindInstalled(home, "registry-evidence")
	if err != nil || !ok {
		t.Fatalf("persisted release: ok=%t err=%v", ok, err)
	}
	assertEvidence("persisted restored release", persisted, "1.0.0", 7)
	assertReleaseEvidence("persisted forward release", persisted.Previous, "2.0.0", 9)
}
