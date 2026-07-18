package themepack

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func candidateFixture(t *testing.T, id, accent string) Candidate {
	t.Helper()
	manifest := validManifest()
	manifest.ID = id
	manifest.Name = id
	manifest.Tokens.Dark["accent"] = accent
	raw := themeArchive(t, manifest)
	candidate, err := inspectArchiveBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(t.TempDir())
	store.now = func() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) }
	return store
}

func TestStoreInstallPreviewApplyAndRelaunch(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	first := candidateFixture(t, "z-theme", "#abcdef")
	second := candidateFixture(t, "a-theme", "#123456")
	if _, err := store.Install(first, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(second, false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Preview("z-theme", false)
	if err != nil || snapshot.PreviewThemeID != "z-theme" || snapshot.AppliedThemeID != "" {
		t.Fatalf("preview snapshot=%+v err=%v", snapshot, err)
	}
	if got := snapshot.Packs[0].Manifest.ID; got != "a-theme" {
		t.Fatalf("deterministic first pack = %q", got)
	}
	if _, err := store.Apply("z-theme", false); err != nil {
		t.Fatal(err)
	}

	relaunched := NewStore(home)
	snapshot, err = relaunched.Snapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AppliedThemeID != "z-theme" || snapshot.PreviewThemeID != "" || snapshot.EffectiveID != "z-theme" {
		t.Fatalf("relaunch snapshot = %+v", snapshot)
	}
}

func TestStoreConflictReplaceAndDeleteActive(t *testing.T) {
	store := testStore(t)
	old := candidateFixture(t, "replace-me", "#111111")
	newer := candidateFixture(t, "replace-me", "#222222")
	if _, err := store.Install(old, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(newer, false); !errors.Is(err, ErrPackExists) {
		t.Fatalf("conflict error = %v", err)
	}
	if _, err := store.Install(newer, true); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := store.Apply("replace-me", false)
	if snapshot.AppliedThemeID != "replace-me" {
		t.Fatalf("apply snapshot = %+v", snapshot)
	}
	snapshot, err := store.Delete("replace-me", false)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AppliedThemeID != "" || len(snapshot.Packs) != 0 {
		t.Fatalf("delete snapshot = %+v", snapshot)
	}
}

func TestStoreRecoversInterruptedInstall(t *testing.T) {
	store := testStore(t)
	old := candidateFixture(t, "durable", "#111111")
	newer := candidateFixture(t, "durable", "#222222")
	if _, err := store.Install(old, false); err != nil {
		t.Fatal(err)
	}
	store.failpoint = func(stage string) error {
		if stage == "after-prepare" {
			return errors.New("power loss")
		}
		return nil
	}
	if _, err := store.Install(newer, true); err == nil {
		t.Fatal("expected injected install failure")
	}
	relaunched := NewStore(filepath.Dir(store.Root()))
	snapshot, err := relaunched.Snapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Packs) != 1 || snapshot.Packs[0].PackageDigest != old.PackageDigest {
		t.Fatalf("recovered packs = %+v", snapshot.Packs)
	}
}

func TestStoreRollsForwardInterruptedDelete(t *testing.T) {
	store := testStore(t)
	candidate := candidateFixture(t, "delete-me", "#123456")
	if _, err := store.Install(candidate, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply("delete-me", false); err != nil {
		t.Fatal(err)
	}
	store.failpoint = func(stage string) error {
		if stage == "after-delete-state" {
			return errors.New("power loss")
		}
		return nil
	}
	if _, err := store.Delete("delete-me", false); err == nil {
		t.Fatal("expected injected delete failure")
	}
	relaunched := NewStore(filepath.Dir(store.Root()))
	snapshot, err := relaunched.Snapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AppliedThemeID != "" || len(snapshot.Packs) != 0 {
		t.Fatalf("delete recovery snapshot = %+v", snapshot)
	}
}

func TestStoreInterruptedReplacementMatrixPreservesCoherentActiveState(t *testing.T) {
	tests := []struct {
		stage   string
		wantNew bool
	}{
		{stage: "after-assets", wantNew: false},
		{stage: "after-prepare", wantNew: false},
		{stage: "after-pack-write", wantNew: true},
		{stage: "after-install-state", wantNew: true},
	}
	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			home := t.TempDir()
			store := NewStore(home)
			old := candidateFixture(t, "matrix-theme", "#111111")
			newer := candidateFixture(t, "matrix-theme", "#222222")
			if _, err := store.Install(old, false); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Apply(old.Manifest.ID, false); err != nil {
				t.Fatal(err)
			}
			store.failpoint = func(stage string) error {
				if stage == tt.stage {
					return errors.New("power loss")
				}
				return nil
			}
			if _, err := store.Install(newer, true); err == nil {
				t.Fatal("expected injected replacement failure")
			}

			relaunched := NewStore(home)
			snapshot, err := relaunched.Snapshot(false)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.AppliedThemeID != old.Manifest.ID || len(snapshot.Packs) != 1 || len(snapshot.Warnings) != 0 {
				t.Fatalf("recovered replacement snapshot = %+v", snapshot)
			}
			wantDigest := old.PackageDigest
			if tt.wantNew {
				wantDigest = newer.PackageDigest
			}
			if snapshot.Packs[0].PackageDigest != wantDigest {
				t.Fatalf("recovered digest = %s, want %s", snapshot.Packs[0].PackageDigest, wantDigest)
			}
			state, pack, warning, err := relaunched.Active(false)
			if err != nil || warning != "" || pack == nil || state.AppliedPackageDigest != wantDigest || pack.PackageDigest != wantDigest {
				t.Fatalf("recovered active state=%+v pack=%+v warning=%q err=%v", state, pack, warning, err)
			}
		})
	}
}

func TestStoreInterruptedDeleteMatrixAlwaysRollsForward(t *testing.T) {
	for _, stage := range []string{"after-prepare", "after-delete-state", "after-delete-rename"} {
		t.Run(stage, func(t *testing.T) {
			home := t.TempDir()
			store := NewStore(home)
			candidate := candidateFixture(t, "delete-matrix", "#123456")
			if _, err := store.Install(candidate, false); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Apply(candidate.Manifest.ID, false); err != nil {
				t.Fatal(err)
			}
			store.failpoint = func(got string) error {
				if got == stage {
					return errors.New("power loss")
				}
				return nil
			}
			if _, err := store.Delete(candidate.Manifest.ID, false); err == nil {
				t.Fatal("expected injected delete failure")
			}

			relaunched := NewStore(home)
			snapshot, err := relaunched.Snapshot(false)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.AppliedThemeID != "" || len(snapshot.Packs) != 0 || len(snapshot.Warnings) != 0 {
				t.Fatalf("delete recovery snapshot = %+v", snapshot)
			}
		})
	}
}

func TestStoreSafeModeDoesNotTouchFilesystem(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	snapshot, err := store.Snapshot(true)
	if err != nil || !snapshot.SafeMode || snapshot.Packs == nil {
		t.Fatalf("safe snapshot=%+v err=%v", snapshot, err)
	}
	if _, err := os.Stat(store.Root()); !os.IsNotExist(err) {
		t.Fatalf("Safe Mode touched store root: %v", err)
	}
	if _, err := store.Preview("anything", true); err == nil {
		t.Fatal("Safe Mode preview should fail")
	}
}

func TestStoreRejectsTamperedContentAddressedAsset(t *testing.T) {
	store := testStore(t)
	png := tinyPNG()
	manifest := manifestWithPNG(t, "scene.png", png)
	candidate, err := inspectArchiveBytes(themeArchive(t, manifest, zipFixtureFile{name: "scene.png", data: png}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(candidate, false); err != nil {
		t.Fatal(err)
	}
	digest := manifest.Scenes.Home.Image.SHA256
	assetDir := filepath.Join(store.Root(), "assets", "sha256")
	entries, _ := os.ReadDir(assetDir)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), digest) {
		t.Fatalf("asset entries = %+v", entries)
	}
	if err := os.WriteFile(filepath.Join(assetDir, entries[0].Name()), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Packs) != 0 || len(snapshot.Warnings) == 0 {
		t.Fatalf("tampered snapshot = %+v", snapshot)
	}
	if _, err := store.Asset(digest, false); err == nil {
		t.Fatal("tampered asset should not be served")
	}
}
