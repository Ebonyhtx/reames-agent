package themepack

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	officialDawnID     = "reames-dawn"
	officialWorkshopID = "reames-workshop"

	dawnAssetName     = "reames-dawn-horizon.jpg"
	workshopAssetName = "reames-night-workshop.jpg"
	dawnAssetDigest   = "91f060ed4e34cb5511a490187b9cfa1cd0dd7a255a1af542452cd25be1a8b899"
	workshopDigest    = "740439c941d931a9d2b064aef280f461f68e06c8df9ac5991c7b0f19d88ee6bf"
)

// officialAssetFS holds original Reames artwork in the Go binary. Official
// packs never depend on the user theme store and cannot contain executable
// content, arbitrary CSS, markup, fonts, or remote resources.
//
//go:embed assets/*.jpg
var officialAssetFS embed.FS

var officialIDSet = map[string]struct{}{
	officialDawnID:     {},
	officialWorkshopID: {},
}

type officialCatalogData struct {
	packs  map[string]InstalledPack
	assets map[string]AssetContent
	ids    []string
}

var builtInOfficialCatalog = mustBuildOfficialCatalog()

// IsOfficialID reports whether id belongs to Reames Agent's immutable bundled
// catalog. User archives cannot import, replace, or delete these identifiers.
func IsOfficialID(id string) bool {
	_, ok := officialIDSet[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

// OfficialPackIDs returns the stable Gallery order for immutable Reames packs.
func OfficialPackIDs() []string {
	return append([]string(nil), builtInOfficialCatalog.ids...)
}

// OfficialPacks returns deep copies so callers cannot mutate the process-wide
// verified catalog.
func OfficialPacks() []InstalledPack {
	packs := make([]InstalledPack, 0, len(builtInOfficialCatalog.ids))
	for _, id := range builtInOfficialCatalog.ids {
		packs = append(packs, cloneInstalledPack(builtInOfficialCatalog.packs[id]))
	}
	return packs
}

func officialPack(id string) (InstalledPack, bool) {
	pack, ok := builtInOfficialCatalog.packs[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return InstalledPack{}, false
	}
	return cloneInstalledPack(pack), true
}

func officialAsset(digest string) (AssetContent, bool) {
	asset, ok := builtInOfficialCatalog.assets[strings.ToLower(strings.TrimSpace(digest))]
	if !ok {
		return AssetContent{}, false
	}
	asset.Data = append([]byte(nil), asset.Data...)
	return asset, true
}

func mustBuildOfficialCatalog() officialCatalogData {
	assets := make(map[string]AssetContent, 2)
	loadAsset := func(name, wantDigest string) {
		data, err := officialAssetFS.ReadFile("assets/" + name)
		if err != nil {
			panic(fmt.Sprintf("read embedded official theme asset %s: %v", name, err))
		}
		hash := sha256.Sum256(data)
		gotDigest := hex.EncodeToString(hash[:])
		if gotDigest != wantDigest {
			panic(fmt.Sprintf("official theme asset %s digest = %s, want %s", name, gotDigest, wantDigest))
		}
		info, err := validateImage(data, name)
		if err != nil {
			panic(fmt.Sprintf("validate embedded official theme asset %s: %v", name, err))
		}
		assets[wantDigest] = AssetContent{
			Data: data, MIME: info.MIME, Name: name,
			ModTime: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		}
	}
	loadAsset(dawnAssetName, dawnAssetDigest)
	loadAsset(workshopAssetName, workshopDigest)

	manifests := []struct {
		manifest  Manifest
		published string
	}{
		{
			manifest: Manifest{
				SchemaVersion: SchemaVersion,
				ID:            officialDawnID,
				Name:          "Reames Dawn",
				Version:       "1.0.0",
				Author:        "Reames Project",
				Description:   "A calm misty horizon with restrained graphite and amber tones.",
				License:       "MIT",
				Provenance: Provenance{
					Kind: "original", Source: "Original Reames Agent artwork generated on 2026-07-18",
					GeneratedWith: "OpenAI image generation via Codex imagegen",
				},
				BaseStyle: "slate",
				Tokens: Tokens{
					Light: map[string]string{
						"bg": "#f4f1eb", "bgSoft": "#e9edf0", "bgElev": "#ffffff",
						"panel": "#f8f7f3", "sidebar": "#e6ebef", "chat": "#f7f4ee",
						"workspace": "#e8edf1", "workspaceFiles": "#dde5ea",
						"border": "#b8c3cb", "borderSoft": "#d3dbe0",
						"fg": "#1f2a33", "fgDim": "#53616c", "fgFaint": "#75818a",
						"accent": "#9a5a2b", "accentFg": "#ffffff", "ok": "#2d7453",
						"warn": "#9a5a2b", "err": "#a33c3c",
					},
					Dark: map[string]string{
						"bg": "#111820", "bgSoft": "#18232c", "bgElev": "#202d37",
						"panel": "#17212a", "sidebar": "#101820", "chat": "#151e26",
						"workspace": "#111a22", "workspaceFiles": "#18232c",
						"border": "#33434f", "borderSoft": "#283741",
						"fg": "#e7edf1", "fgDim": "#b5c0c8", "fgFaint": "#85939e",
						"accent": "#e3a15f", "accentFg": "#1e140a", "ok": "#68c492",
						"warn": "#e3a15f", "err": "#ef7777",
					},
				},
				Recipes: Recipes{Density: "comfortable", Corners: "soft"},
				Scenes: Scenes{Home: &Scene{
					Image:  AssetRef{File: dawnAssetName, SHA256: dawnAssetDigest},
					FocusX: 0.5, FocusY: 0.52, SafeArea: "center", Opacity: 0.34, OverlayStrength: 0.58,
				}},
			},
			published: "2026-07-18T00:00:26.549Z",
		},
		{
			manifest: Manifest{
				SchemaVersion: SchemaVersion,
				ID:            officialWorkshopID,
				Name:          "Reames Workshop",
				Version:       "1.0.0",
				Author:        "Reames Project",
				Description:   "A focused technical night landscape with restrained teal and copper detail.",
				License:       "MIT",
				Provenance: Provenance{
					Kind: "original", Source: "Original Reames Agent artwork generated on 2026-07-18",
					GeneratedWith: "OpenAI image generation via Codex imagegen",
				},
				BaseStyle: "nocturne",
				Tokens: Tokens{
					Light: map[string]string{
						"bg": "#edf1f3", "bgSoft": "#e3e9ec", "bgElev": "#ffffff",
						"panel": "#f5f7f8", "sidebar": "#e0e7ea", "chat": "#f3f6f7",
						"workspace": "#dce5e8", "workspaceFiles": "#d3dee2",
						"border": "#afbec5", "borderSoft": "#ccd6da",
						"fg": "#172731", "fgDim": "#4d616d", "fgFaint": "#72838c",
						"accent": "#08717a", "accentFg": "#ffffff", "ok": "#2d7453",
						"warn": "#9a5a2b", "err": "#a33c3c",
					},
					Dark: map[string]string{
						"bg": "#07111c", "bgSoft": "#0c1926", "bgElev": "#122333",
						"panel": "#0b1723", "sidebar": "#06101a", "chat": "#091520",
						"workspace": "#07131f", "workspaceFiles": "#0d1b28",
						"border": "#263c4c", "borderSoft": "#1b2d3b",
						"fg": "#e4edf2", "fgDim": "#adbdc7", "fgFaint": "#7f929e",
						"accent": "#58b7bd", "accentFg": "#061214", "ok": "#68c492",
						"warn": "#d99659", "err": "#ef7777",
					},
				},
				Recipes: Recipes{Density: "compact", Corners: "square"},
				Scenes: Scenes{Workspace: &Scene{
					Image:  AssetRef{File: workshopAssetName, SHA256: workshopDigest},
					FocusX: 0.5, FocusY: 0.45, SafeArea: "center", Opacity: 0.3, OverlayStrength: 0.72,
				}},
			},
			published: "2026-07-18T00:01:10.118Z",
		},
	}

	packs := make(map[string]InstalledPack, len(manifests))
	ids := make([]string, 0, len(manifests))
	for _, item := range manifests {
		manifest := item.manifest
		if err := validateManifest(&manifest, true); err != nil {
			panic(fmt.Sprintf("validate official theme %s: %v", manifest.ID, err))
		}
		canonical, err := canonicalOfficialManifest(manifest)
		if err != nil {
			panic(fmt.Sprintf("canonicalize official theme %s: %v", manifest.ID, err))
		}
		pack := InstalledPack{
			Manifest: manifest, PackageDigest: canonicalPackageDigest(canonical, manifestAssetRefs(manifest)),
			ImportedAt: item.published,
		}
		packs[manifest.ID] = pack
		ids = append(ids, manifest.ID)
	}
	sort.Strings(ids)
	return officialCatalogData{packs: packs, assets: assets, ids: ids}
}

func cloneInstalledPack(pack InstalledPack) InstalledPack {
	pack.Manifest = cloneManifest(pack.Manifest)
	return pack
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Tokens.Light = cloneTokenMap(manifest.Tokens.Light)
	manifest.Tokens.Dark = cloneTokenMap(manifest.Tokens.Dark)
	if manifest.Scenes.Home != nil {
		home := *manifest.Scenes.Home
		manifest.Scenes.Home = &home
	}
	if manifest.Scenes.Workspace != nil {
		workspace := *manifest.Scenes.Workspace
		manifest.Scenes.Workspace = &workspace
	}
	return manifest
}

func cloneTokenMap(tokens map[string]string) map[string]string {
	clone := make(map[string]string, len(tokens))
	for key, value := range tokens {
		clone[key] = value
	}
	return clone
}
