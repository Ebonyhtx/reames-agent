package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reames-agent/internal/themepack"
)

func desktopThemeArchive(t *testing.T, home, id string, withImage bool) (string, themepack.Candidate) {
	t.Helper()
	manifest := themepack.Manifest{
		SchemaVersion: themepack.SchemaVersion,
		ID:            id,
		Name:          "Desktop Theme",
		Version:       "1.0.0",
		Author:        "Reames Project",
		License:       "CC-BY-4.0",
		Provenance:    themepack.Provenance{Kind: "original", Source: "desktop test fixture"},
		BaseStyle:     "aurora",
		Tokens: themepack.Tokens{
			Light: map[string]string{"accent": "#315f8c"},
			Dark:  map[string]string{"accent": "#88baf0"},
		},
		Recipes: themepack.Recipes{Density: "comfortable", Corners: "soft"},
	}
	var imageData []byte
	if withImage {
		var err error
		imageData, err = base64.StdEncoding.DecodeString(desktopTinyPNG)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(imageData)
		manifest.Scenes.Home = &themepack.Scene{
			Image:  themepack.AssetRef{File: "scene.png", SHA256: hex.EncodeToString(digest[:])},
			FocusX: .5, FocusY: .5, SafeArea: "center", Opacity: 1, OverlayStrength: .55,
		}
	}
	path := filepath.Join(home, id+themepack.ArchiveExtension)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	manifestEntry, err := writer.Create(themepack.ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(manifest)
	if _, err := manifestEntry.Write(raw); err != nil {
		t.Fatal(err)
	}
	if withImage {
		imageEntry, err := writer.Create("scene.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := imageEntry.Write(imageData); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	candidate, err := themepack.InspectArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, candidate
}

func TestThemeAppStartupGalleryPreviewAndRelaunch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "")
	_, candidate := desktopThemeArchive(t, home, "desktop-dawn", false)
	store := themepack.NewStore(home)
	if _, err := store.Install(candidate, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(candidate.Manifest.ID, false); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	active, err := app.ActiveThemePack()
	if err != nil {
		t.Fatal(err)
	}
	if active.Pack == nil || active.Pack.ID != "desktop-dawn" || active.EffectiveStyle != "aurora" {
		t.Fatalf("active theme = %+v", active)
	}
	experience, err := app.ThemeExperience()
	if err != nil {
		t.Fatal(err)
	}
	if len(experience.Packs) != 9 || experience.AppliedThemeID != "desktop-dawn" {
		t.Fatalf("experience = %+v", experience)
	}
	if _, err := app.PreviewThemePack("desktop-dawn"); err != nil {
		t.Fatal(err)
	}
	if canceled, err := app.CancelThemePreview(); err != nil || canceled.PreviewThemeID != "" || canceled.AppliedThemeID != "desktop-dawn" {
		t.Fatalf("cancel preview=%+v err=%v", canceled, err)
	}
}

func TestThemeAppOfficialCatalogApplyAndLazyStartupRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "")
	app := NewApp()
	experience, err := app.ThemeExperience()
	if err != nil {
		t.Fatal(err)
	}
	if len(experience.Packs) != 8 {
		t.Fatalf("base + official pack count = %d", len(experience.Packs))
	}
	var official []ThemePackView
	for _, pack := range experience.Packs {
		if pack.Kind == "official" {
			official = append(official, pack)
		}
	}
	if len(official) != 2 || official[0].ID != "reames-dawn" || official[1].ID != "reames-workshop" {
		t.Fatalf("official catalog = %+v", official)
	}
	if _, err := app.PreviewThemePack("reames-dawn"); err != nil {
		t.Fatal(err)
	}
	if applied, err := app.ApplyThemePack("reames-workshop"); err != nil || applied.AppliedThemeID != "reames-workshop" {
		t.Fatalf("official apply=%+v err=%v", applied, err)
	}
	if _, err := app.DeleteThemePack("reames-workshop"); err == nil {
		t.Fatal("official pack deletion should fail")
	}

	// Startup restore reads only state + the selected immutable official pack;
	// an unrelated malformed Gallery record must not delay or break startup.
	packDir := filepath.Join(home, "theme-packs", "packs")
	if err := os.WriteFile(filepath.Join(packDir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	relaunched := NewApp()
	active, err := relaunched.ActiveThemePack()
	if err != nil || active.Pack == nil || active.Pack.Kind != "official" || active.Pack.ID != "reames-workshop" {
		t.Fatalf("lazy startup active=%+v err=%v", active, err)
	}
}

func TestOfficialThemeAssetMiddlewareIsEmbeddedAndSafeModeSuppressed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "")
	pack := themepack.OfficialPacks()[0]
	digest := pack.Manifest.Scenes.Home.Image.SHA256
	app := NewApp()
	handler := app.workspaceMediaMiddleware()(http.NotFoundHandler())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, themeAssetURLPrefix+digest, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/jpeg" || recorder.Body.Len() == 0 {
		t.Fatalf("official asset status=%d headers=%v bytes=%d", recorder.Code, recorder.Header(), recorder.Body.Len())
	}
	if _, err := os.Stat(filepath.Join(home, "theme-packs")); !os.IsNotExist(err) {
		t.Fatalf("embedded official asset touched user store: %v", err)
	}

	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, themeAssetURLPrefix+digest, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("Safe Mode official asset status = %d", recorder.Code)
	}
}

func TestThemeAssetMiddlewareIsDigestAddressedAndSafeModeSuppressed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "")
	_, candidate := desktopThemeArchive(t, home, "desktop-scene", true)
	store := themepack.NewStore(home)
	if _, err := store.Install(candidate, false); err != nil {
		t.Fatal(err)
	}
	digest := candidate.Manifest.Scenes.Home.Image.SHA256
	app := NewApp()
	nextCalled := false
	handler := app.workspaceMediaMiddleware()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, themeAssetURLPrefix+digest, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(recorder.Header().Get("Cache-Control"), "immutable") || nextCalled {
		t.Fatalf("theme response status=%d headers=%v next=%v", recorder.Code, recorder.Header(), nextCalled)
	}
	original, _ := base64.StdEncoding.DecodeString(desktopTinyPNG)
	if !bytes.Equal(recorder.Body.Bytes(), original) {
		t.Fatal("theme middleware returned different content")
	}

	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, themeAssetURLPrefix+digest, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("Safe Mode asset status = %d, want 404", recorder.Code)
	}
	active, err := app.ActiveThemePack()
	if err != nil || !active.SafeMode || active.Pack != nil || active.EffectiveStyle != "graphite" {
		t.Fatalf("Safe Mode active=%+v err=%v", active, err)
	}
}

func TestThemeAppSafeModeRejectsEveryPackMutationBindingBeforeIO(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	t.Setenv("REAMES_AGENT_SAFE_MODE", "1")
	app := NewApp()

	experience, err := app.ThemeExperience()
	if err != nil {
		t.Fatal(err)
	}
	if !experience.SafeMode || experience.EffectiveStyle != "graphite" || len(experience.Packs) != len(themepack.BaseStyleIDs()) {
		t.Fatalf("Safe Mode experience = %+v", experience)
	}
	for name, call := range map[string]func() error{
		"preview": func() error { _, err := app.PreviewThemePack("reames-dawn"); return err },
		"apply":   func() error { _, err := app.ApplyThemePack("reames-dawn"); return err },
		"delete":  func() error { _, err := app.DeleteThemePack("reames-dawn"); return err },
		"import":  func() error { _, err := app.ImportThemePack(); return err },
		"confirm": func() error { _, err := app.ConfirmThemePackImport("opaque-token"); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "Safe Mode") {
			t.Fatalf("Safe Mode %s error = %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "theme-packs")); !os.IsNotExist(err) {
		t.Fatalf("Safe Mode mutation bindings touched theme store: %v", err)
	}
}

func TestThemePendingImportUsesOpaqueSingleUseToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REAMES_AGENT_HOME", home)
	_, candidate := desktopThemeArchive(t, home, "pending-theme", false)
	app := NewApp()
	token, err := app.savePendingThemeImport(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 32 || strings.Contains(token, home) {
		t.Fatalf("pending token = %q", token)
	}
	got, err := app.takePendingThemeImport(token)
	if err != nil || got.PackageDigest != candidate.PackageDigest {
		t.Fatalf("take pending got=%+v err=%v", got, err)
	}
	if _, err := app.takePendingThemeImport(token); err == nil {
		t.Fatal("pending import token must be single use")
	}
}
