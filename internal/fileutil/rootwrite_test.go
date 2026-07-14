package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicWriteRootFileCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := AtomicWriteRootFile(root, filepath.Join("nested", "file.txt"), []byte("first"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteRootFile(root, filepath.Join("nested", "file.txt"), []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "nested", "file.txt"))
	if err != nil || string(b) != "second" {
		t.Fatalf("content = %q, err = %v", b, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "nested", "file.txt")); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestAtomicWriteRootFileRejectsEscapeAndSymlinkOutside(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := AtomicWriteRootFile(root, filepath.Join("..", "escape.txt"), []byte("no"), 0o644); err == nil {
		t.Fatal("parent escape was accepted")
	}
	link := filepath.Join(dir, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := AtomicWriteRootFile(root, filepath.Join("outside", "escape.txt"), []byte("no"), 0o644); err == nil {
		t.Fatal("symlink escape was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file exists or stat failed: %v", err)
	}
}

func TestAtomicWriteRootFileCleansTemporaryFileOnReplaceFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := AtomicWriteRootFile(root, "target", []byte("cannot replace a directory"), 0o644); err == nil {
		t.Fatal("expected directory replacement failure")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".atomic-") {
			t.Fatalf("temporary file was not cleaned: %s", entry.Name())
		}
	}
}
