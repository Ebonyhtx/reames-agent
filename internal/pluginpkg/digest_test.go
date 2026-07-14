package pluginpkg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestContentDigestRejectsExecutableModeChangeDuringHash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix executable mode bits")
	}
	root := t.TempDir()
	path := filepath.Join(root, "tool")
	if err := os.WriteFile(path, []byte("tool"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := beforeDigestFileOpen
	beforeDigestFileOpen = func(rel string) error {
		if rel == "tool" {
			return os.Chmod(path, 0o755)
		}
		return nil
	}
	t.Cleanup(func() { beforeDigestFileOpen = original })

	if _, err := ContentDigest(root); err == nil || !strings.Contains(err.Error(), "changed while hashing") {
		t.Fatalf("mode-racing digest error = %v", err)
	}
}
