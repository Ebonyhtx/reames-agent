package pluginpkg

import (
	"encoding/json"
	"testing"
)

func TestManifestV1Fields(t *testing.T) {
	m := Manifest{
		Name:             "superpowers",
		Version:          "1.0.0",
		TrustLevel:       "verified",
		Permissions:      []string{"file.read", "file.write", "exec.shell"},
		MinReamesVersion: "1.8.0",
		Integrity: &ManifestIntegrity{
			SHA256: "abc123def",
			Signatures: []ManifestSignature{
				{Algorithm: "minisign", KeyID: "RWQq...", Signature: "base64..."},
			},
		},
	}
	if err := ValidateManifest(&m); err != nil {
		t.Fatal(err)
	}
	if !m.HasPermission("file.write") {
		t.Fatal("should have file.write permission")
	}
	if m.HasPermission("network.any") {
		t.Fatal("should not have network.any")
	}
}

func TestValidateManifestRejectsInvalidName(t *testing.T) {
	m := Manifest{Name: "", Version: "1.0"}
	if err := ValidateManifest(&m); err == nil {
		t.Fatal("should reject empty name")
	}
}

func TestValidateManifestRequiresVersion(t *testing.T) {
	m := Manifest{Name: "test-plugin"}
	if err := ValidateManifest(&m); err == nil {
		t.Fatal("should require version")
	}
}

func TestValidateManifestRejectsBadTrustLevel(t *testing.T) {
	m := Manifest{Name: "test-plugin", Version: "1.0", TrustLevel: "untrusted"}
	if err := ValidateManifest(&m); err == nil {
		t.Fatal("should reject unknown trust level")
	}
}

func TestValidateManifestAcceptsNilTrustLevel(t *testing.T) {
	m := Manifest{Name: "test-plugin", Version: "1.0"}
	if err := ValidateManifest(&m); err != nil {
		t.Fatalf("empty trust level should be accepted: %v", err)
	}
}

func TestUnknownPermissions(t *testing.T) {
	m := Manifest{
		Name:        "test",
		Version:     "1.0",
		Permissions: []string{"file.read", "unknown.scope", "network.any"},
	}
	unknown := UnknownPermissions(&m)
	if len(unknown) != 1 || unknown[0] != "unknown.scope" {
		t.Fatalf("unknown permissions = %v, want [unknown.scope]", unknown)
	}
}

func TestManifestJSONRoundTripWithNewFields(t *testing.T) {
	m := Manifest{
		Name:             "ci-tools",
		Version:          "2.1.0",
		Description:      "CI integration helpers",
		TrustLevel:       "community",
		Permissions:      []string{"file.read", "network.local"},
		MinReamesVersion: "1.9.0",
		MaxReamesVersion: "2.5.0",
		Integrity: &ManifestIntegrity{
			SHA256:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Provenance: "https://example.com/provenance.json",
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var restored Manifest
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.TrustLevel != "community" {
		t.Fatalf("TrustLevel = %q", restored.TrustLevel)
	}
	if len(restored.Permissions) != 2 {
		t.Fatalf("Permissions = %v", restored.Permissions)
	}
	if restored.MinReamesVersion != "1.9.0" {
		t.Fatalf("MinReamesVersion = %q", restored.MinReamesVersion)
	}
	if restored.Integrity == nil || restored.Integrity.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal("Integrity round-trip failed")
	}
}

func TestManifestV0BackwardCompat(t *testing.T) {
	// Old manifests without the new fields should still parse.
	v0 := []byte(`{"name":"old-plugin","version":"1.0","skills":["lint"]}`)
	var m Manifest
	if err := json.Unmarshal(v0, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "old-plugin" {
		t.Fatalf("Name = %q", m.Name)
	}
	if m.TrustLevel != "" {
		t.Fatalf("TrustLevel should be empty for v0 manifest, got %q", m.TrustLevel)
	}
	if len(m.Permissions) != 0 {
		t.Fatalf("Permissions should be nil for v0 manifest")
	}
}
