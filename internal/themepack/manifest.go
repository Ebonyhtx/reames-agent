// Package themepack implements Reames Agent's controlled, non-executable
// desktop theme package format and its local content-addressed store.
package themepack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	SchemaVersion       = 1
	ArchiveExtension    = ".reames-theme"
	ManifestName        = "theme.json"
	MaxArchiveBytes     = 36 << 20
	MaxManifestBytes    = 1 << 20
	MaxImageBytes       = 16 << 20
	MaxExpandedBytes    = 33 << 20
	MaxArchiveEntries   = 3
	MaxImageEdge        = 8192
	MaxImagePixels      = 24_000_000
	MaxCompressionRatio = 200
	MaxIDLength         = 64
	MaxNameLength       = 80
	MaxTextLength       = 240
	MaxDescription      = 512
)

var (
	idPattern      = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,62}[a-z0-9])?$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[+-][0-9A-Za-z.-]+)?$`)
	colorPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}(?:[0-9a-fA-F]{2})?$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	imagePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,120}\.(?i:png|jpe?g|webp)$`)
)

var baseStyles = map[string]struct{}{
	"graphite": {}, "aurora": {}, "slate": {}, "carbon": {}, "nocturne": {}, "amber": {},
}

// TokenCSSVariables is the complete package-controlled semantic-token surface.
// Its stable keys are shared with the frontend and public schema.
var TokenCSSVariables = map[string]string{
	"bg": "--bg", "bgSoft": "--bg-soft", "bgElev": "--bg-elev",
	"panel": "--panel", "sidebar": "--sidebar-bg", "chat": "--chat-bg",
	"workspace": "--workspace-preview-bg", "workspaceFiles": "--workspace-files-bg",
	"border": "--border", "borderSoft": "--border-soft",
	"fg": "--fg", "fgDim": "--fg-dim", "fgFaint": "--fg-faint",
	"accent": "--accent", "accentFg": "--accent-fg",
	"ok": "--ok", "warn": "--warn", "err": "--err",
}

// Manifest is the strict theme.json contract. It cannot carry scripts, markup,
// fonts, remote resources, arbitrary CSS, or unrecognized fields.
type Manifest struct {
	SchemaVersion int        `json:"schemaVersion"`
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Version       string     `json:"version"`
	Author        string     `json:"author,omitempty"`
	Description   string     `json:"description,omitempty"`
	License       string     `json:"license"`
	Provenance    Provenance `json:"provenance"`
	BaseStyle     string     `json:"baseStyle"`
	Tokens        Tokens     `json:"tokens"`
	Recipes       Recipes    `json:"recipes"`
	Scenes        Scenes     `json:"scenes,omitempty"`
}

type Provenance struct {
	Kind          string `json:"kind"` // original|licensed
	Source        string `json:"source"`
	SourceURL     string `json:"sourceUrl,omitempty"`
	GeneratedWith string `json:"generatedWith,omitempty"`
}

type Tokens struct {
	Light map[string]string `json:"light"`
	Dark  map[string]string `json:"dark"`
}

type Recipes struct {
	Density string `json:"density"` // compact|comfortable
	Corners string `json:"corners"` // square|soft|round
}

type Scenes struct {
	Home      *Scene `json:"home,omitempty"`
	Workspace *Scene `json:"workspace,omitempty"`
}

type Scene struct {
	Image           AssetRef `json:"image"`
	FocusX          float64  `json:"focusX"`
	FocusY          float64  `json:"focusY"`
	SafeArea        string   `json:"safeArea"` // left|center|right
	Opacity         float64  `json:"opacity"`
	OverlayStrength float64  `json:"overlayStrength"`
}

type AssetRef struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// DecodeManifest parses one strict manifest, rejecting unknown fields at every
// nesting level and trailing JSON values.
func DecodeManifest(data []byte) (Manifest, error) {
	if len(data) == 0 {
		return Manifest{}, fmt.Errorf("theme manifest is empty")
	}
	if len(data) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("theme manifest exceeds %d bytes", MaxManifestBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var manifest Manifest
	if err := dec.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode theme manifest: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return Manifest{}, fmt.Errorf("theme manifest contains trailing JSON")
	}
	if err := ValidateManifest(&manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ValidateManifest normalizes bounded textual fields and validates the complete
// semantic and resource allow-list.
func ValidateManifest(manifest *Manifest) error {
	return validateManifest(manifest, false)
}

func validateManifest(manifest *Manifest, allowOfficialID bool) error {
	if manifest == nil {
		return fmt.Errorf("theme manifest is nil")
	}
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported theme schemaVersion %d (want %d)", manifest.SchemaVersion, SchemaVersion)
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	if len(manifest.ID) > MaxIDLength || !idPattern.MatchString(manifest.ID) {
		return fmt.Errorf("invalid theme id %q", manifest.ID)
	}
	if IsBaseStyle(manifest.ID) {
		return fmt.Errorf("theme id %q is reserved for a base style", manifest.ID)
	}
	if !allowOfficialID && IsOfficialID(manifest.ID) {
		return fmt.Errorf("theme id %q is reserved for an official Reames theme", manifest.ID)
	}
	if isWindowsDeviceName(manifest.ID) {
		return fmt.Errorf("theme id %q is reserved by Windows", manifest.ID)
	}
	var err error
	if manifest.Name, err = requiredText("name", manifest.Name, MaxNameLength); err != nil {
		return err
	}
	manifest.Version = strings.TrimSpace(manifest.Version)
	if len(manifest.Version) > 64 || !versionPattern.MatchString(manifest.Version) {
		return fmt.Errorf("theme version %q must be semantic version text", manifest.Version)
	}
	if manifest.Author, err = optionalText("author", manifest.Author, MaxTextLength); err != nil {
		return err
	}
	if manifest.Description, err = optionalText("description", manifest.Description, MaxDescription); err != nil {
		return err
	}
	if manifest.License, err = requiredText("license", manifest.License, 120); err != nil {
		return err
	}
	if err := validateProvenance(&manifest.Provenance); err != nil {
		return err
	}
	manifest.BaseStyle = strings.ToLower(strings.TrimSpace(manifest.BaseStyle))
	if !IsBaseStyle(manifest.BaseStyle) {
		return fmt.Errorf("invalid baseStyle %q", manifest.BaseStyle)
	}
	if err := validateTokenMap(manifest.Tokens.Light, "tokens.light"); err != nil {
		return err
	}
	if err := validateTokenMap(manifest.Tokens.Dark, "tokens.dark"); err != nil {
		return err
	}
	manifest.Recipes.Density = strings.ToLower(strings.TrimSpace(manifest.Recipes.Density))
	switch manifest.Recipes.Density {
	case "compact", "comfortable":
	default:
		return fmt.Errorf("invalid recipes.density %q", manifest.Recipes.Density)
	}
	manifest.Recipes.Corners = strings.ToLower(strings.TrimSpace(manifest.Recipes.Corners))
	switch manifest.Recipes.Corners {
	case "square", "soft", "round":
	default:
		return fmt.Errorf("invalid recipes.corners %q", manifest.Recipes.Corners)
	}
	seenAssets := map[string]string{}
	if err := validateScene(manifest.Scenes.Home, "scenes.home", seenAssets); err != nil {
		return err
	}
	if err := validateScene(manifest.Scenes.Workspace, "scenes.workspace", seenAssets); err != nil {
		return err
	}
	if len(manifest.Tokens.Light)+len(manifest.Tokens.Dark) == 0 && len(seenAssets) == 0 {
		return fmt.Errorf("theme must override at least one semantic token or scene")
	}
	return nil
}

func validateProvenance(provenance *Provenance) error {
	if provenance == nil {
		return fmt.Errorf("provenance is required")
	}
	provenance.Kind = strings.ToLower(strings.TrimSpace(provenance.Kind))
	switch provenance.Kind {
	case "original", "licensed":
	default:
		return fmt.Errorf("provenance.kind must be original|licensed")
	}
	var err error
	if provenance.Source, err = requiredText("provenance.source", provenance.Source, MaxTextLength); err != nil {
		return err
	}
	if provenance.GeneratedWith, err = optionalText("provenance.generatedWith", provenance.GeneratedWith, MaxTextLength); err != nil {
		return err
	}
	provenance.SourceURL = strings.TrimSpace(provenance.SourceURL)
	if provenance.SourceURL != "" {
		parsed, err := url.Parse(provenance.SourceURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("provenance.sourceUrl must be an absolute HTTPS URL without credentials")
		}
	}
	return nil
}

func validateTokenMap(tokens map[string]string, path string) error {
	for key, value := range tokens {
		if _, ok := TokenCSSVariables[key]; !ok {
			return fmt.Errorf("%s contains unknown semantic token %q", path, key)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if !colorPattern.MatchString(value) {
			return fmt.Errorf("%s.%s must be #RRGGBB or #RRGGBBAA", path, key)
		}
		tokens[key] = value
	}
	return nil
}

func validateScene(scene *Scene, path string, seen map[string]string) error {
	if scene == nil {
		return nil
	}
	scene.Image.File = strings.TrimSpace(scene.Image.File)
	if !imagePattern.MatchString(scene.Image.File) || filepath.Base(scene.Image.File) != scene.Image.File || strings.ContainsAny(scene.Image.File, `/\:`) {
		return fmt.Errorf("%s.image.file must be a root-level PNG, JPEG, or WebP file name", path)
	}
	if isWindowsDeviceName(scene.Image.File) {
		return fmt.Errorf("%s.image.file %q is reserved by Windows", path, scene.Image.File)
	}
	scene.Image.SHA256 = strings.ToLower(strings.TrimSpace(scene.Image.SHA256))
	if !digestPattern.MatchString(scene.Image.SHA256) {
		return fmt.Errorf("%s.image.sha256 must be a full lowercase SHA-256 digest", path)
	}
	key := strings.ToLower(scene.Image.File)
	if previous, exists := seen[key]; exists {
		return fmt.Errorf("%s image name duplicates %s under case-folding", path, previous)
	}
	seen[key] = path
	if scene.FocusX < 0 || scene.FocusX > 1 || scene.FocusY < 0 || scene.FocusY > 1 {
		return fmt.Errorf("%s focus coordinates must be between 0 and 1", path)
	}
	scene.SafeArea = strings.ToLower(strings.TrimSpace(scene.SafeArea))
	switch scene.SafeArea {
	case "left", "center", "right":
	default:
		return fmt.Errorf("%s.safeArea must be left|center|right", path)
	}
	if scene.Opacity < 0 || scene.Opacity > 1 || scene.OverlayStrength < 0 || scene.OverlayStrength > 1 {
		return fmt.Errorf("%s opacity values must be between 0 and 1", path)
	}
	return nil
}

func requiredText(path, value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", path)
	}
	return boundedText(path, value, max)
}

func optionalText(path, value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return boundedText(path, value, max)
}

func boundedText(path, value string, max int) (string, error) {
	if len(value) > max {
		return "", fmt.Errorf("%s exceeds %d bytes", path, max)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%s contains control characters", path)
		}
	}
	return value, nil
}

// IsBaseStyle reports whether id is one of the six built-in visual directions.
func IsBaseStyle(id string) bool {
	_, ok := baseStyles[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

// BaseStyleIDs returns the stable UI order for built-in styles.
func BaseStyleIDs() []string {
	return []string{"graphite", "aurora", "slate", "carbon", "nocturne", "amber"}
}

func manifestAssetRefs(manifest Manifest) []AssetRef {
	var refs []AssetRef
	if manifest.Scenes.Home != nil {
		refs = append(refs, manifest.Scenes.Home.Image)
	}
	if manifest.Scenes.Workspace != nil {
		refs = append(refs, manifest.Scenes.Workspace.Image)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].File < refs[j].File })
	return refs
}

func isWindowsDeviceName(name string) bool {
	base := strings.TrimRight(strings.TrimSpace(filepath.Base(name)), ". ")
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	return false
}
