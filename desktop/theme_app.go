package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reames-agent/internal/config"
	"reames-agent/internal/themepack"
)

const themeAssetURLPrefix = "/__reames_agent_theme_asset/"

type themeRuntime struct {
	mu      sync.Mutex
	home    string
	store   *themepack.Store
	pending map[string]pendingThemeImport
}

type pendingThemeImport struct {
	candidate themepack.Candidate
	expiresAt time.Time
}

type ThemeSceneView struct {
	Image           themepack.AssetRef `json:"image"`
	ImageURL        string             `json:"imageUrl"`
	FocusX          float64            `json:"focusX"`
	FocusY          float64            `json:"focusY"`
	SafeArea        string             `json:"safeArea"`
	Opacity         float64            `json:"opacity"`
	OverlayStrength float64            `json:"overlayStrength"`
}

type ThemeScenesView struct {
	Home      *ThemeSceneView `json:"home,omitempty"`
	Workspace *ThemeSceneView `json:"workspace,omitempty"`
}

type ThemePackView struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Version          string                      `json:"version,omitempty"`
	Author           string                      `json:"author,omitempty"`
	Description      string                      `json:"description,omitempty"`
	License          string                      `json:"license,omitempty"`
	Provenance       themepack.Provenance        `json:"provenance"`
	BaseStyle        string                      `json:"baseStyle"`
	Tokens           themepack.Tokens            `json:"tokens"`
	Recipes          themepack.Recipes           `json:"recipes"`
	Scenes           ThemeScenesView             `json:"scenes"`
	Kind             string                      `json:"kind"` // base|official|user
	Applied          bool                        `json:"applied"`
	Preview          bool                        `json:"preview"`
	PackageDigest    string                      `json:"packageDigest,omitempty"`
	ContrastWarnings []themepack.ContrastWarning `json:"contrastWarnings"`
}

type ThemeActiveView struct {
	ThemeMode      string         `json:"themeMode"`
	BaseStyle      string         `json:"baseStyle"`
	EffectiveStyle string         `json:"effectiveStyle"`
	AppliedThemeID string         `json:"appliedThemeId,omitempty"`
	Pack           *ThemePackView `json:"pack,omitempty"`
	Warning        string         `json:"warning,omitempty"`
	SafeMode       bool           `json:"safeMode"`
}

type ThemeExperienceView struct {
	ThemeMode      string          `json:"themeMode"`
	BaseStyle      string          `json:"baseStyle"`
	EffectiveStyle string          `json:"effectiveStyle"`
	AppliedThemeID string          `json:"appliedThemeId,omitempty"`
	PreviewThemeID string          `json:"previewThemeId,omitempty"`
	EffectiveID    string          `json:"effectiveId,omitempty"`
	Packs          []ThemePackView `json:"packs"`
	Warnings       []string        `json:"warnings"`
	SafeMode       bool            `json:"safeMode"`
}

type ThemeImportResult struct {
	Pack         ThemePackView `json:"pack"`
	NeedsReplace bool          `json:"needsReplace,omitempty"`
	PendingID    string        `json:"pendingId,omitempty"`
	Canceled     bool          `json:"canceled,omitempty"`
}

func newThemeRuntime() *themeRuntime {
	return &themeRuntime{pending: map[string]pendingThemeImport{}}
}

func (a *App) themeRuntime() *themeRuntime {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.themes == nil {
		a.themes = newThemeRuntime()
	}
	return a.themes
}

func (themes *themeRuntime) currentStore() (*themepack.Store, error) {
	themes.mu.Lock()
	defer themes.mu.Unlock()
	home := strings.TrimSpace(config.ReamesAgentHomeDir())
	if home == "" {
		return nil, fmt.Errorf("Reames Agent home is unavailable")
	}
	home = filepath.Clean(home)
	if themes.store == nil || themes.home != home {
		themes.home = home
		themes.store = themepack.NewStore(home)
		themes.pending = map[string]pendingThemeImport{}
	}
	return themes.store, nil
}

func themeSafeMode() bool { return config.SafeModeRequested() }

func desktopThemePreferences(safeMode bool) (themeMode, baseStyle string) {
	if safeMode {
		return "auto", "graphite"
	}
	cfg := config.LoadForEditWithoutCredentials(config.UserConfigPath())
	themeMode = cfg.DesktopTheme()
	baseStyle = cfg.DesktopThemeStyle()
	if !themepack.IsBaseStyle(baseStyle) {
		baseStyle = "graphite"
	}
	return themeMode, baseStyle
}

// ActiveThemePack restores only the committed pack needed during startup. It
// does not enumerate the lazy Appearance Gallery.
func (a *App) ActiveThemePack() (ThemeActiveView, error) {
	safeMode := themeSafeMode()
	themeMode, baseStyle := desktopThemePreferences(safeMode)
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeActiveView{}, err
	}
	state, pack, warning, err := store.Active(safeMode)
	if err != nil {
		return ThemeActiveView{}, err
	}
	view := ThemeActiveView{
		ThemeMode: themeMode, BaseStyle: baseStyle, EffectiveStyle: baseStyle,
		AppliedThemeID: state.AppliedThemeID, Warning: warning, SafeMode: safeMode,
	}
	if safeMode {
		view.AppliedThemeID = ""
		view.EffectiveStyle = "graphite"
		return view, nil
	}
	if pack != nil {
		packView := themePackView(*pack, true, false)
		view.Pack = &packView
		view.EffectiveStyle = pack.Manifest.BaseStyle
	}
	return view, nil
}

// ThemeExperience lazily enumerates the complete base/user theme library.
func (a *App) ThemeExperience() (ThemeExperienceView, error) {
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeExperienceView{}, err
	}
	safeMode := themeSafeMode()
	snapshot, err := store.Snapshot(safeMode)
	if err != nil {
		return ThemeExperienceView{}, err
	}
	return makeThemeExperience(snapshot, safeMode), nil
}

func makeThemeExperience(snapshot themepack.Snapshot, safeMode bool) ThemeExperienceView {
	themeMode, baseStyle := desktopThemePreferences(safeMode)
	view := ThemeExperienceView{
		ThemeMode: themeMode, BaseStyle: baseStyle, EffectiveStyle: baseStyle,
		AppliedThemeID: snapshot.AppliedThemeID, PreviewThemeID: snapshot.PreviewThemeID,
		EffectiveID: snapshot.EffectiveID, Packs: []ThemePackView{}, Warnings: snapshot.Warnings,
		SafeMode: safeMode,
	}
	for _, id := range themepack.BaseStyleIDs() {
		view.Packs = append(view.Packs, ThemePackView{
			ID: id, Name: baseThemeName(id), BaseStyle: id, Kind: "base",
			Tokens:  themepack.Tokens{Light: map[string]string{}, Dark: map[string]string{}},
			Recipes: themepack.Recipes{Density: "comfortable", Corners: "soft"},
			Scenes:  ThemeScenesView{}, Applied: !safeMode && snapshot.AppliedThemeID == "" && id == baseStyle,
			Preview:          !safeMode && snapshot.PreviewThemeID == "" && snapshot.AppliedThemeID == "" && id == baseStyle,
			ContrastWarnings: []themepack.ContrastWarning{},
		})
	}
	if safeMode {
		view.BaseStyle = "graphite"
		view.EffectiveStyle = "graphite"
		view.AppliedThemeID = ""
		view.PreviewThemeID = ""
		view.EffectiveID = ""
		for index := range view.Packs {
			view.Packs[index].Applied = view.Packs[index].ID == "graphite"
			view.Packs[index].Preview = false
		}
		return view
	}
	for _, pack := range themepack.OfficialPacks() {
		view.Packs = append(view.Packs, themePackView(pack, pack.Manifest.ID == snapshot.AppliedThemeID, pack.Manifest.ID == snapshot.PreviewThemeID))
		if pack.Manifest.ID == snapshot.EffectiveID {
			view.EffectiveStyle = pack.Manifest.BaseStyle
		}
	}
	for _, pack := range snapshot.Packs {
		view.Packs = append(view.Packs, themePackView(pack, pack.Manifest.ID == snapshot.AppliedThemeID, pack.Manifest.ID == snapshot.PreviewThemeID))
		if pack.Manifest.ID == snapshot.EffectiveID {
			view.EffectiveStyle = pack.Manifest.BaseStyle
		}
	}
	return view
}

func themePackView(pack themepack.InstalledPack, applied, preview bool) ThemePackView {
	manifest := pack.Manifest
	kind := "user"
	if themepack.IsOfficialID(manifest.ID) {
		kind = "official"
	}
	return ThemePackView{
		ID: manifest.ID, Name: manifest.Name, Version: manifest.Version, Author: manifest.Author,
		Description: manifest.Description, License: manifest.License, Provenance: manifest.Provenance,
		BaseStyle: manifest.BaseStyle, Tokens: manifest.Tokens, Recipes: manifest.Recipes,
		Scenes: themeScenesView(manifest.Scenes), Kind: kind, Applied: applied, Preview: preview,
		PackageDigest: pack.PackageDigest, ContrastWarnings: themepack.ContrastWarnings(manifest),
	}
}

func candidateThemePackView(candidate themepack.Candidate) ThemePackView {
	return themePackView(themepack.InstalledPack{
		Manifest: candidate.Manifest, PackageDigest: candidate.PackageDigest,
		ImportedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, false, false)
}

func themeScenesView(scenes themepack.Scenes) ThemeScenesView {
	convert := func(scene *themepack.Scene) *ThemeSceneView {
		if scene == nil {
			return nil
		}
		return &ThemeSceneView{
			Image: scene.Image, ImageURL: themeAssetURLPrefix + scene.Image.SHA256,
			FocusX: scene.FocusX, FocusY: scene.FocusY, SafeArea: scene.SafeArea,
			Opacity: scene.Opacity, OverlayStrength: scene.OverlayStrength,
		}
	}
	return ThemeScenesView{Home: convert(scenes.Home), Workspace: convert(scenes.Workspace)}
}

func baseThemeName(id string) string {
	switch id {
	case "graphite":
		return "Graphite"
	case "aurora":
		return "Aurora"
	case "slate":
		return "Slate"
	case "carbon":
		return "Carbon"
	case "nocturne":
		return "Nocturne"
	case "amber":
		return "Amber"
	default:
		return id
	}
}

func (a *App) PreviewThemePack(id string) (ThemeExperienceView, error) {
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeExperienceView{}, err
	}
	safeMode := themeSafeMode()
	snapshot, err := store.Preview(id, safeMode)
	if err != nil {
		return ThemeExperienceView{}, err
	}
	return makeThemeExperience(snapshot, safeMode), nil
}

func (a *App) CancelThemePreview() (ThemeExperienceView, error) {
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeExperienceView{}, err
	}
	safeMode := themeSafeMode()
	snapshot, err := store.CancelPreview(safeMode)
	if err != nil {
		return ThemeExperienceView{}, err
	}
	return makeThemeExperience(snapshot, safeMode), nil
}

func (a *App) ApplyThemePack(id string) (ThemeExperienceView, error) {
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeExperienceView{}, err
	}
	safeMode := themeSafeMode()
	snapshot, err := store.Apply(id, safeMode)
	if err != nil {
		return ThemeExperienceView{}, err
	}
	return makeThemeExperience(snapshot, safeMode), nil
}

func (a *App) DeleteThemePack(id string) (ThemeExperienceView, error) {
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeExperienceView{}, err
	}
	safeMode := themeSafeMode()
	snapshot, err := store.Delete(id, safeMode)
	if err != nil {
		return ThemeExperienceView{}, err
	}
	return makeThemeExperience(snapshot, safeMode), nil
}

// ImportThemePack opens a native picker and verifies the selected archive. A
// replacement requires a second explicit, opaque-token confirmation.
func (a *App) ImportThemePack() (ThemeImportResult, error) {
	if themeSafeMode() {
		return ThemeImportResult{}, errors.New("theme import is disabled in Safe Mode")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Reames Theme Pack",
		Filters: []runtime.FileFilter{{
			DisplayName: "Reames Theme Pack (*.reames-theme)",
			Pattern:     "*" + themepack.ArchiveExtension,
		}},
	})
	if err != nil {
		return ThemeImportResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return ThemeImportResult{Canceled: true}, nil
	}
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeImportResult{}, err
	}
	candidate, err := store.Inspect(path)
	if err != nil {
		return ThemeImportResult{}, err
	}
	exists, err := store.Has(candidate.Manifest.ID)
	if err != nil {
		return ThemeImportResult{}, err
	}
	if exists {
		pendingID, err := a.savePendingThemeImport(candidate)
		if err != nil {
			return ThemeImportResult{}, err
		}
		return ThemeImportResult{Pack: candidateThemePackView(candidate), NeedsReplace: true, PendingID: pendingID}, nil
	}
	record, err := store.Install(candidate, false)
	if err != nil {
		return ThemeImportResult{}, err
	}
	return ThemeImportResult{Pack: themePackView(record, false, false)}, nil
}

func (a *App) ConfirmThemePackImport(pendingID string) (ThemeImportResult, error) {
	if themeSafeMode() {
		return ThemeImportResult{}, errors.New("theme import is disabled in Safe Mode")
	}
	candidate, err := a.takePendingThemeImport(pendingID)
	if err != nil {
		return ThemeImportResult{}, err
	}
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return ThemeImportResult{}, err
	}
	record, err := store.Install(candidate, true)
	if err != nil {
		return ThemeImportResult{}, err
	}
	return ThemeImportResult{Pack: themePackView(record, false, false)}, nil
}

func (a *App) CancelThemePackImport(pendingID string) {
	themes := a.themeRuntime()
	themes.mu.Lock()
	delete(themes.pending, strings.TrimSpace(pendingID))
	themes.mu.Unlock()
}

func (a *App) savePendingThemeImport(candidate themepack.Candidate) (string, error) {
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes[:])
	themes := a.themeRuntime()
	themes.mu.Lock()
	defer themes.mu.Unlock()
	now := time.Now()
	for id, pending := range themes.pending {
		if !now.Before(pending.expiresAt) {
			delete(themes.pending, id)
		}
	}
	themes.pending[token] = pendingThemeImport{candidate: candidate, expiresAt: now.Add(10 * time.Minute)}
	return token, nil
}

func (a *App) takePendingThemeImport(id string) (themepack.Candidate, error) {
	themes := a.themeRuntime()
	themes.mu.Lock()
	defer themes.mu.Unlock()
	id = strings.TrimSpace(id)
	pending, ok := themes.pending[id]
	delete(themes.pending, id)
	if !ok || time.Now().After(pending.expiresAt) {
		return themepack.Candidate{}, errors.New("theme import confirmation expired; choose the package again")
	}
	return pending.candidate, nil
}

func (a *App) themeAsset(digest string) (themepack.AssetContent, error) {
	store, err := a.themeRuntime().currentStore()
	if err != nil {
		return themepack.AssetContent{}, err
	}
	return store.Asset(strings.TrimSpace(digest), themeSafeMode())
}
