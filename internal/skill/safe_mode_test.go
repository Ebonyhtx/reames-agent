package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisableDiscoveryReturnsEmptyStore(t *testing.T) {
	project := t.TempDir()
	dir := filepath.Join(project, ".reames-agent", "skills", "unsafe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: unsafe\n---\nbody"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(Options{ProjectRoot: project, DisableDiscovery: true})
	if roots := store.Roots(); len(roots) != 0 {
		t.Fatalf("roots = %+v, want none", roots)
	}
	if skills := store.List(); len(skills) != 0 {
		t.Fatalf("skills = %+v, want none", skills)
	}
	if _, ok := store.Read("unsafe"); ok {
		t.Fatal("disabled store read a skill")
	}
}
