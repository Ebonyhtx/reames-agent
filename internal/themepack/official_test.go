package themepack

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestOfficialCatalogAssetDigestProvenanceAndContrast(t *testing.T) {
	if got := strings.Join(OfficialPackIDs(), ","); got != "reames-dawn,reames-workshop" {
		t.Fatalf("OfficialPackIDs = %q", got)
	}
	packs := OfficialPacks()
	if len(packs) != 2 {
		t.Fatalf("official pack count = %d", len(packs))
	}
	for _, pack := range packs {
		if pack.Manifest.Provenance.Kind != "original" || !strings.Contains(pack.Manifest.Provenance.GeneratedWith, "Codex imagegen") {
			t.Fatalf("official provenance = %+v", pack.Manifest.Provenance)
		}
		if pack.Manifest.License != "MIT" || len(ContrastWarnings(pack.Manifest)) != 0 {
			t.Fatalf("official license/warnings for %s: %q %+v", pack.Manifest.ID, pack.Manifest.License, ContrastWarnings(pack.Manifest))
		}
		canonical, err := canonicalOfficialManifest(pack.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		if got := canonicalPackageDigest(canonical, manifestAssetRefs(pack.Manifest)); got != pack.PackageDigest {
			t.Fatalf("official package %s digest = %s, want %s", pack.Manifest.ID, pack.PackageDigest, got)
		}
		for _, ref := range manifestAssetRefs(pack.Manifest) {
			asset, ok := officialAsset(ref.SHA256)
			if !ok {
				t.Fatalf("official asset %s is missing", ref.SHA256)
			}
			hash := sha256.Sum256(asset.Data)
			if got := hex.EncodeToString(hash[:]); got != ref.SHA256 {
				t.Fatalf("official asset %s digest = %s", asset.Name, got)
			}
			if asset.MIME != "image/jpeg" || asset.Name != ref.File {
				t.Fatalf("official asset metadata = %+v", asset)
			}
		}
	}

	// Catalog accessors must not expose mutable package maps or scene pointers.
	packs[0].Manifest.Tokens.Dark["accent"] = "#000000"
	if fresh := OfficialPacks()[0].Manifest.Tokens.Dark["accent"]; fresh == "#000000" {
		t.Fatal("OfficialPacks exposed mutable catalog state")
	}
}

func TestStoreOfficialPreviewApplyRelaunchAndPartition(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	snapshot, err := store.Snapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Packs) != 0 {
		t.Fatalf("durable user partition contains official packs: %+v", snapshot.Packs)
	}

	snapshot, err = store.Preview(officialDawnID, false)
	if err != nil || snapshot.PreviewThemeID != officialDawnID || snapshot.EffectiveID != officialDawnID {
		t.Fatalf("official preview=%+v err=%v", snapshot, err)
	}
	if _, err := store.Apply(officialWorkshopID, false); err != nil {
		t.Fatal(err)
	}

	relaunched := NewStore(home)
	state, pack, warning, err := relaunched.Active(false)
	if err != nil || warning != "" || pack == nil || pack.Manifest.ID != officialWorkshopID || state.AppliedThemeID != officialWorkshopID {
		t.Fatalf("official relaunch state=%+v pack=%+v warning=%q err=%v", state, pack, warning, err)
	}
	if _, err := relaunched.Delete(officialWorkshopID, false); err == nil {
		t.Fatal("official theme deletion should fail")
	}
	if exists, err := relaunched.Has(officialWorkshopID); err != nil || !exists {
		t.Fatalf("Has(official)=%v err=%v", exists, err)
	}
}

func TestOfficialAssetsAreSuppressedInSafeModeWithoutStoreIO(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	ref := OfficialPacks()[0].Manifest.Scenes.Home.Image
	asset, err := store.Asset(ref.SHA256, false)
	if err != nil || len(asset.Data) == 0 {
		t.Fatalf("official asset err=%v bytes=%d", err, len(asset.Data))
	}
	if _, err := os.Stat(store.Root()); !os.IsNotExist(err) {
		t.Fatalf("serving an embedded official asset touched user store: %v", err)
	}
	if _, err := store.Asset(ref.SHA256, true); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Safe Mode official asset error = %v", err)
	}
}
