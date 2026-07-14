package builtin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootedTargetRejectsStableSymlinkEscape(t *testing.T) {
	rootDir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(rootDir, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	target, err := openRootedWriteTarget(realRoots([]string{rootDir}), SessionDataGuard{}, filepath.Join(link, "escape.txt"))
	if err == nil {
		target.Close()
		t.Fatal("symlink escape was accepted")
	}
}

func TestRootedTargetPinsResolvedPathAcrossSymlinkReplacement(t *testing.T) {
	rootDir := t.TempDir()
	inside := filepath.Join(rootDir, "inside")
	outside := t.TempDir()
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootDir, "link")
	if err := os.Symlink(inside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	target, err := openRootedWriteTarget(realRoots([]string{rootDir}), SessionDataGuard{}, filepath.Join(link, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := target.writeBytes([]byte("inside only"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(inside, "file.txt")); err != nil || string(got) != "inside only" {
		t.Fatalf("inside content = %q, err = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file exists or stat failed: %v", err)
	}
}
