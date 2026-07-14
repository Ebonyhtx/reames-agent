package pluginpkg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeManifestV1PermissionContract(t *testing.T) {
	tests := []struct {
		name      string
		manifest  string
		wantError string
	}{
		{
			name:     "valid",
			manifest: `{"schemaVersion":1,"name":"native","version":"1.2.3","skills":["skills"],"permissions":["skills.load"]}`,
		},
		{
			name:      "missing permission",
			manifest:  `{"schemaVersion":1,"name":"native","version":"1.2.3","skills":["skills"],"permissions":[]}`,
			wantError: "do not match required permissions",
		},
		{
			name:      "unknown permission",
			manifest:  `{"schemaVersion":1,"name":"native","version":"1.2.3","skills":["skills"],"permissions":["skills.load","host.root"]}`,
			wantError: "unknown plugin permission",
		},
		{
			name:      "invalid version",
			manifest:  `{"schemaVersion":1,"name":"native","version":"latest","skills":["skills"],"permissions":["skills.load"]}`,
			wantError: "requires a semantic version",
		},
		{
			name:      "future schema",
			manifest:  `{"schemaVersion":2,"name":"native","version":"1.2.3","skills":["skills"],"permissions":["skills.load"]}`,
			wantError: "unsupported native plugin schemaVersion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, NativeManifest), []byte(tt.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
				t.Fatal(err)
			}
			pkg, _, err := ParseDir(root)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("ParseDir error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if pkg.Manifest.SchemaVersion != NativeSchemaVersion || pkg.Manifest.PermissionSource != PermissionSourceDeclared || !sameStrings(pkg.Manifest.Permissions, []string{PermissionSkillsLoad}) {
				t.Fatalf("manifest = %+v", pkg.Manifest)
			}
		})
	}
}

func TestNativeManifestLegacyIsInferredWithWarning(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, NativeManifest), []byte(`{"name":"legacy","skills":["skills"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg, warnings, err := ParseDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.PermissionSource != PermissionSourceLegacy || !sameStrings(pkg.Manifest.Permissions, []string{PermissionSkillsLoad}) {
		t.Fatalf("legacy manifest = %+v", pkg.Manifest)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "schemaVersion=1") {
		t.Fatalf("legacy warnings = %v", warnings)
	}
}

func TestLegacyNativeManifestFilenameIsAcceptedWithWarning(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, LegacyNativeManifest), []byte(`{"schemaVersion":1,"name":"legacy-name","version":"1.0.0","permissions":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, warnings, err := ParseDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.ManifestKind != "reames-agent-legacy" || ManifestPath(pkg.ManifestKind) != LegacyNativeManifest {
		t.Fatalf("legacy manifest identity = kind %q path %q", pkg.ManifestKind, ManifestPath(pkg.ManifestKind))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "deprecated") || !strings.Contains(warnings[0], NativeManifest) {
		t.Fatalf("legacy filename warnings = %v", warnings)
	}
}

func TestCorruptNativeManifestDoesNotFallBackToCompatibilityManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, NativeManifest), []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(root, CodexManifest)
	if err := os.MkdirAll(filepath.Dir(codex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codex, []byte(`{"name":"fallback","version":"1.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseDir(root); err == nil || !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("ParseDir error = %v, want corrupt native manifest error", err)
	}
}

func TestContentDigestRejectsNestedSymlink(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "redirect.txt")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ContentDigest(root); err == nil || !strings.Contains(err.Error(), "contains symlink") {
		t.Fatalf("ContentDigest error = %v, want symlink rejection", err)
	}
}
