package themepack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: 1,
		ID:            "test-theme",
		Name:          "Reames Dawn",
		Version:       "1.0.0",
		Author:        "Reames Project",
		Description:   "A controlled test theme",
		License:       "CC-BY-4.0",
		Provenance: Provenance{
			Kind:   "original",
			Source: "Reames test fixture",
		},
		BaseStyle: "graphite",
		Tokens: Tokens{
			Light: map[string]string{"accent": "#315f8cff"},
			Dark:  map[string]string{"accent": "#88baf0"},
		},
		Recipes: Recipes{Density: "comfortable", Corners: "soft"},
	}
}

func TestDecodeManifestRejectsUnknownNestedFieldsAndTrailingJSON(t *testing.T) {
	manifest := validManifest()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"density":"comfortable"`, `"density":"comfortable","script":"alert(1)"`, 1)
	if _, err := DecodeManifest([]byte(bad)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown nested field error = %v", err)
	}
	if _, err := DecodeManifest(append(raw, []byte(` {}`)...)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing JSON error = %v", err)
	}
}

func TestValidateManifestSemanticAllowlist(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"reserved base id", func(m *Manifest) { m.ID = "graphite" }, "reserved"},
		{"reserved official id", func(m *Manifest) { m.ID = "reames-dawn" }, "official"},
		{"windows device id", func(m *Manifest) { m.ID = "con" }, "Windows"},
		{"unknown token", func(m *Manifest) { m.Tokens.Dark["fontFamily"] = "#ffffff" }, "unknown semantic token"},
		{"css expression", func(m *Manifest) { m.Tokens.Light["accent"] = "url(https://example.com/x)" }, "#RRGGBB"},
		{"remote provenance credentials", func(m *Manifest) { m.Provenance.SourceURL = "https://user:pass@example.com/theme" }, "without credentials"},
		{"empty theme", func(m *Manifest) { m.Tokens = Tokens{Light: map[string]string{}, Dark: map[string]string{}} }, "at least one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validManifest()
			tt.mutate(&manifest)
			if err := ValidateManifest(&manifest); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateManifest error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPublicSchemaMatchesGoAllowlists(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "theme-pack.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	defs := schema["$defs"].(map[string]any)
	tokenProperties := defs["tokenMap"].(map[string]any)["properties"].(map[string]any)
	goTokens := make([]string, 0, len(TokenCSSVariables))
	for token := range TokenCSSVariables {
		goTokens = append(goTokens, token)
	}
	schemaTokens := make([]string, 0, len(tokenProperties))
	for token := range tokenProperties {
		schemaTokens = append(schemaTokens, token)
	}
	sort.Strings(goTokens)
	sort.Strings(schemaTokens)
	if strings.Join(goTokens, ",") != strings.Join(schemaTokens, ",") {
		t.Fatalf("schema tokens = %v, Go tokens = %v", schemaTokens, goTokens)
	}

	properties := schema["properties"].(map[string]any)
	idRule := properties["id"].(map[string]any)
	notRule := idRule["not"].(map[string]any)
	reservedValues := notRule["enum"].([]any)
	reserved := make([]string, 0, len(reservedValues))
	for _, value := range reservedValues {
		reserved = append(reserved, value.(string))
	}
	wantReserved := append(BaseStyleIDs(), OfficialPackIDs()...)
	sort.Strings(reserved)
	sort.Strings(wantReserved)
	if strings.Join(reserved, ",") != strings.Join(wantReserved, ",") {
		t.Fatalf("schema reserved ids = %v, Go reserved ids = %v", reserved, wantReserved)
	}
}

func TestValidateManifestRejectsCaseFoldedSceneNames(t *testing.T) {
	manifest := validManifest()
	manifest.Scenes = Scenes{
		Home: &Scene{
			Image:    AssetRef{File: "Scene.PNG", SHA256: strings.Repeat("a", 64)},
			SafeArea: "center", FocusX: .5, FocusY: .5, Opacity: 1, OverlayStrength: .5,
		},
		Workspace: &Scene{
			Image:    AssetRef{File: "scene.png", SHA256: strings.Repeat("b", 64)},
			SafeArea: "center", FocusX: .5, FocusY: .5, Opacity: .3, OverlayStrength: .7,
		},
	}
	if err := ValidateManifest(&manifest); err == nil || !strings.Contains(err.Error(), "case-folding") {
		t.Fatalf("ValidateManifest error = %v", err)
	}
}

func TestBaseStyleIDsStable(t *testing.T) {
	want := "graphite,aurora,slate,carbon,nocturne,amber"
	if got := strings.Join(BaseStyleIDs(), ","); got != want {
		t.Fatalf("BaseStyleIDs = %q, want %q", got, want)
	}
	if len(TokenCSSVariables) != 18 {
		t.Fatalf("semantic token count = %d, want 18", len(TokenCSSVariables))
	}
}

func TestContrastWarningsSurfaceUnsafePairs(t *testing.T) {
	manifest := validManifest()
	manifest.Tokens.Light = map[string]string{
		"fg": "#ffffff", "bg": "#ffffff", "accent": "#ffffff", "accentFg": "#ffffff",
	}
	warnings := ContrastWarnings(manifest)
	if len(warnings) < 2 {
		t.Fatalf("ContrastWarnings = %+v, want unsafe light pairs", warnings)
	}
	for _, warning := range warnings {
		if warning.Ratio >= warning.Minimum || warning.Suggest == "" {
			t.Fatalf("invalid warning = %+v", warning)
		}
	}
}
