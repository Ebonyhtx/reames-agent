package homebackup

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateVerifyRestoreSplitRootsAndExcludeKnownSecrets(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "source-home")
	state := filepath.Join(base, "source-state")
	mustWrite(t, filepath.Join(home, "config.toml"), "model = \"fixture\"\n")
	mustWrite(t, filepath.Join(home, ".env"), "FIXTURE_SECRET=must-not-be-archived\n")
	mustWrite(t, filepath.Join(home, "credentials.enc"), "machine-bound-secret")
	mustWrite(t, filepath.Join(home, "cache", "probe.json"), "derived")
	mustWrite(t, filepath.Join(state, "sessions", "one.jsonl"), "{\"role\":\"user\",\"content\":\"hello\"}\n")
	mustWrite(t, filepath.Join(state, "weixin", "accounts", "account.json"), "fixture-channel-secret")
	mustWrite(t, filepath.Join(state, "bot", "pairing.json"), "fixture-pairing-code")
	mustWrite(t, filepath.Join(state, "metrics-pending.json"), "ephemeral metrics")
	mustWrite(t, filepath.Join(state, "crash-pending.json"), "ephemeral crash payload")
	if err := os.MkdirAll(filepath.Join(state, "memory", "empty"), 0o700); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(base, "backup.zip")
	created, err := Create(CreateOptions{
		Roots:            []Root{{ID: "state", Path: state}, {ID: "home", Path: home}},
		Destination:      archive,
		CreatedByVersion: "v1.2.3-test",
		Now:              func() time.Time { return time.Date(2026, 7, 14, 1, 2, 3, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Files != 2 || created.Manifest.Secrets != "known-stores-excluded" {
		t.Fatalf("created summary = %+v", created)
	}
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 && os.PathSeparator != '\\' {
		t.Fatalf("archive mode = %#o, want owner-only", info.Mode().Perm())
	}
	verified, err := Verify(archive)
	if err != nil {
		t.Fatal(err)
	}
	if verified.ArchiveSHA256 == "" || verified.Files != 2 {
		t.Fatalf("verified summary = %+v", verified)
	}
	for _, forbidden := range []string{".env", "credentials.enc", "weixin/accounts", "bot/pairing.json", "cache/probe.json", "metrics-pending.json", "crash-pending.json"} {
		if archiveHasEntryContaining(t, archive, forbidden) {
			t.Fatalf("archive contains excluded entry %q", forbidden)
		}
	}

	restoredHome := filepath.Join(base, "restored-home")
	restoredState := filepath.Join(base, "restored-state")
	if _, err := Restore(RestoreOptions{
		Archive: archive,
		Targets: map[string]string{"home": restoredHome, "state": restoredState},
	}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(restoredHome, "config.toml"), "model = \"fixture\"\n")
	assertFile(t, filepath.Join(restoredState, "sessions", "one.jsonl"), "{\"role\":\"user\",\"content\":\"hello\"}\n")
	if info, err := os.Stat(filepath.Join(restoredState, "memory", "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty directory was not restored: info=%v err=%v", info, err)
	}
	for _, forbidden := range []string{
		filepath.Join(restoredHome, ".env"),
		filepath.Join(restoredHome, "credentials.enc"),
		filepath.Join(restoredState, "weixin", "accounts"),
	} {
		if _, err := os.Stat(forbidden); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("excluded path restored: %s err=%v", forbidden, err)
		}
	}
}

func TestCreatePublishAndSyncFailuresDoNotLeaveAmbiguousArchive(t *testing.T) {
	t.Run("destination appears before publish", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		mustWrite(t, filepath.Join(home, "config.toml"), "safe")
		destination := filepath.Join(base, "backup.zip")
		previous := publishBackupPath
		publishBackupPath = func(_, target string) error {
			if err := os.WriteFile(target, []byte("racing-owner"), 0o600); err != nil {
				t.Fatal(err)
			}
			return os.ErrExist
		}
		t.Cleanup(func() { publishBackupPath = previous })
		if _, err := Create(CreateOptions{Roots: []Root{{ID: "home", Path: home}}, Destination: destination}); err == nil {
			t.Fatal("Create succeeded after destination race")
		}
		assertFile(t, destination, "racing-owner")
	})

	t.Run("parent sync failure", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		mustWrite(t, filepath.Join(home, "config.toml"), "safe")
		destination := filepath.Join(base, "backup.zip")
		previous := syncCreateParent
		calls := 0
		syncCreateParent = func(string) error {
			calls++
			if calls == 1 {
				return errors.New("injected parent sync failure")
			}
			return nil
		}
		t.Cleanup(func() { syncCreateParent = previous })
		if _, err := Create(CreateOptions{Roots: []Root{{ID: "home", Path: home}}, Destination: destination}); err == nil || !strings.Contains(err.Error(), "injected parent sync") {
			t.Fatalf("Create sync error = %v", err)
		}
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ambiguous archive survived sync failure: %v", err)
		}
	})
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	mustWrite(t, filepath.Join(home, "config.toml"), "original")
	original := filepath.Join(base, "original.zip")
	if _, err := Create(CreateOptions{Roots: []Root{{ID: "home", Path: home}}, Destination: original}); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(base, "tampered.zip")
	rewriteArchiveEntry(t, original, tampered, "data/home/config.toml", []byte("tampered"))
	if _, err := Verify(tampered); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("Verify tampered error = %v", err)
	}
}

func TestVerifyRejectsTraversalAndCaseCollision(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	baseManifest := Manifest{
		Format: Format, SchemaVersion: SchemaVersion, CreatedAt: now,
		Secrets: "known-stores-excluded", Roots: []ManifestRoot{{ID: "home"}},
	}

	t.Run("zip slip", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "slip.zip")
		writeRawArchive(t, archive, baseManifest, map[string][]byte{"data/home/../escape": {}})
		if _, err := Verify(archive); err == nil || !strings.Contains(err.Error(), "unsafe archive entry") {
			t.Fatalf("Verify traversal error = %v", err)
		}
	})

	t.Run("case collision", func(t *testing.T) {
		manifest := baseManifest
		emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		manifest.Entries = []Entry{
			{Root: "home", Path: "Config", Type: "file", SHA256: emptyHash, ModifiedAt: now},
			{Root: "home", Path: "config", Type: "file", SHA256: emptyHash, ModifiedAt: now},
		}
		archive := filepath.Join(t.TempDir(), "collision.zip")
		writeRawArchive(t, archive, manifest, map[string][]byte{
			"data/home/Config": {},
			"data/home/config": {},
		})
		if _, err := Verify(archive); err == nil || !strings.Contains(err.Error(), "case-collid") {
			t.Fatalf("Verify collision error = %v", err)
		}
	})

	t.Run("unicode normalization collision", func(t *testing.T) {
		manifest := baseManifest
		emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		manifest.Entries = []Entry{
			{Root: "home", Path: "A\u030A.txt", Type: "file", SHA256: emptyHash, ModifiedAt: now},
			{Root: "home", Path: "\u00C5.txt", Type: "file", SHA256: emptyHash, ModifiedAt: now},
		}
		archive := filepath.Join(t.TempDir(), "unicode-collision.zip")
		writeRawArchive(t, archive, manifest, map[string][]byte{
			"data/home/A\u030A.txt": {},
			"data/home/\u00C5.txt":  {},
		})
		if _, err := Verify(archive); err == nil || !strings.Contains(err.Error(), "collid") {
			t.Fatalf("Verify Unicode collision error = %v", err)
		}
	})
}

func TestManifestRejectsTrailingJSON(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	manifest := Manifest{
		Format: Format, SchemaVersion: SchemaVersion, CreatedAt: now,
		Secrets: "known-stores-excluded", Roots: []ManifestRoot{{ID: "home"}},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestData = append(manifestData, []byte("\n{}")...)
	archive := filepath.Join(t.TempDir(), "trailing-manifest.zip")
	writeRawArchiveWithManifest(t, archive, manifestData, nil)
	if _, err := ReadManifest(archive); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("ReadManifest trailing error = %v", err)
	}
	if _, err := Verify(archive); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("Verify trailing error = %v", err)
	}
}

func TestStableSourceRejectsReplacedFileIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	mustWrite(t, path, "same-content")
	expected, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "replacement.txt")
	mustWrite(t, replacement, "same-content")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(expected, current) {
		t.Skip("filesystem reused the source identity for the prepared replacement")
	}
	f, err := openStableSource(path, expected)
	if f != nil {
		_ = f.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "identity") && !strings.Contains(err.Error(), "no longer names") {
		t.Fatalf("openStableSource replacement error = %v", err)
	}
}

func TestCreateRejectsLeaseAndSymlink(t *testing.T) {
	t.Run("lease", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		mustWrite(t, filepath.Join(home, "sessions", "one.jsonl.lease.json"), "{}")
		_, err := Create(CreateOptions{Roots: []Root{{ID: "home", Path: home}}, Destination: filepath.Join(base, "backup.zip")})
		if err == nil || !strings.Contains(err.Error(), "session lease") {
			t.Fatalf("Create lease error = %v", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		mustWrite(t, filepath.Join(home, "config.toml"), "safe")
		if err := os.Symlink(filepath.Join(home, "config.toml"), filepath.Join(home, "config-link")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		_, err := Create(CreateOptions{Roots: []Root{{ID: "home", Path: home}}, Destination: filepath.Join(base, "backup.zip")})
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("Create symlink error = %v", err)
		}
	})
}

func TestRestoreRefusesExistingTargetAndRollsBackSplitCommit(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	state := filepath.Join(base, "state")
	mustWrite(t, filepath.Join(home, "config.toml"), "home")
	mustWrite(t, filepath.Join(state, "sessions", "one.jsonl"), "state")
	archive := filepath.Join(base, "backup.zip")
	if _, err := Create(CreateOptions{
		Roots:       []Root{{ID: "home", Path: home}, {ID: "state", Path: state}},
		Destination: archive,
	}); err != nil {
		t.Fatal(err)
	}

	existing := filepath.Join(base, "existing")
	if err := os.MkdirAll(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(RestoreOptions{Archive: archive, Targets: map[string]string{"home": existing, "state": filepath.Join(base, "unused")}}); err == nil || !strings.Contains(err.Error(), "must not exist") {
		t.Fatalf("Restore existing target error = %v", err)
	}

	targetHome := filepath.Join(base, "target-home")
	targetState := filepath.Join(base, "target-state")
	previousRename := renameRestorePath
	renameRestorePath = func(oldPath, newPath string) error {
		if filepath.Clean(newPath) == filepath.Clean(targetState) {
			return errors.New("injected second-root publish failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { renameRestorePath = previousRename })
	_, err := Restore(RestoreOptions{
		Archive: archive,
		Targets: map[string]string{"home": targetHome, "state": targetState},
	})
	if err == nil || !strings.Contains(err.Error(), "injected second-root") {
		t.Fatalf("Restore injected error = %v", err)
	}
	for _, target := range []string{targetHome, targetState} {
		if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("partial restore target survived rollback: %s err=%v", target, statErr)
		}
	}
}

func TestRestoreParentSyncFailureRollsBackAllRoots(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	state := filepath.Join(base, "state")
	mustWrite(t, filepath.Join(home, "config.toml"), "home")
	mustWrite(t, filepath.Join(state, "sessions", "one.jsonl"), "state")
	archive := filepath.Join(base, "backup.zip")
	if _, err := Create(CreateOptions{
		Roots:       []Root{{ID: "home", Path: home}, {ID: "state", Path: state}},
		Destination: archive,
	}); err != nil {
		t.Fatal(err)
	}
	targetHome := filepath.Join(base, "target-home")
	targetState := filepath.Join(base, "target-state")
	previous := syncRestoreParent
	syncRestoreParent = func(string) error { return errors.New("injected restore sync failure") }
	t.Cleanup(func() { syncRestoreParent = previous })
	_, err := Restore(RestoreOptions{
		Archive: archive,
		Targets: map[string]string{"home": targetHome, "state": targetState},
	})
	if err == nil || !strings.Contains(err.Error(), "injected restore sync") {
		t.Fatalf("Restore sync error = %v", err)
	}
	for _, target := range []string{targetHome, targetState} {
		if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("partial restore survived sync rollback: %s err=%v", target, statErr)
		}
	}
}

func TestRestoreRollsBackWhenLaterTargetAppearsBeforeCommit(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	state := filepath.Join(base, "state")
	mustWrite(t, filepath.Join(home, "config.toml"), "home")
	mustWrite(t, filepath.Join(state, "sessions", "one.jsonl"), "state")
	archive := filepath.Join(base, "backup.zip")
	if _, err := Create(CreateOptions{
		Roots:       []Root{{ID: "home", Path: home}, {ID: "state", Path: state}},
		Destination: archive,
	}); err != nil {
		t.Fatal(err)
	}
	targetHome := filepath.Join(base, "target-home")
	targetState := filepath.Join(base, "target-state")
	previousRename := renameRestorePath
	createdBlocker := false
	renameRestorePath = func(oldPath, newPath string) error {
		if filepath.Clean(newPath) == filepath.Clean(targetHome) {
			if err := os.WriteFile(targetState, []byte("racing owner"), 0o600); err != nil {
				t.Fatal(err)
			}
			createdBlocker = true
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { renameRestorePath = previousRename })
	_, err := Restore(RestoreOptions{
		Archive: archive,
		Targets: map[string]string{"home": targetHome, "state": targetState},
	})
	if err == nil || !strings.Contains(err.Error(), "appeared during extraction") || !createdBlocker {
		t.Fatalf("Restore target race err=%v blocker=%v", err, createdBlocker)
	}
	if _, statErr := os.Stat(targetHome); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("first committed root survived later target race: %v", statErr)
	}
	assertFile(t, targetState, "racing owner")
}

func TestRestoreFileModeClampsGroupAndWorldPermissions(t *testing.T) {
	if got := restoreFileMode(0o777); got != 0o700 {
		t.Fatalf("restoreFileMode(0777) = %#o, want 0700", got)
	}
	if got := restoreFileMode(0o644); got != 0o600 {
		t.Fatalf("restoreFileMode(0644) = %#o, want 0600", got)
	}
}

func TestPortablePathRejectsWindowsDeviceNamesWithMultipleExtensions(t *testing.T) {
	for _, path := range []string{"CON.foo.bar", "dir/com1.log.txt", "Aux.JSON"} {
		if err := validateRelativePath(path); err == nil {
			t.Fatalf("validateRelativePath(%q) accepted Windows device name", path)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func archiveHasEntryContaining(t *testing.T, archive, needle string) bool {
	t.Helper()
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, file := range zr.File {
		if strings.Contains(file.Name, needle) {
			return true
		}
	}
	return false
}

func rewriteArchiveEntry(t *testing.T, source, destination, name string, replacement []byte) {
	t.Helper()
	zr, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for _, file := range zr.File {
		header := file.FileHeader
		w, err := zw.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		if file.Name == name {
			if _, err := w.Write(replacement); err != nil {
				t.Fatal(err)
			}
			continue
		}
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(w, r); err != nil {
			r.Close()
			t.Fatal(err)
		}
		r.Close()
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeRawArchive(t *testing.T, archive string, manifest Manifest, payload map[string][]byte) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeRawArchiveWithManifest(t, archive, data, payload)
}

func writeRawArchiveWithManifest(t *testing.T, archive string, manifestData []byte, payload map[string][]byte) {
	t.Helper()
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range payload {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create(manifestName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(w, bytes.NewReader(manifestData)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
