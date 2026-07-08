package builtin

import (
	"testing"
)

func TestApplyPatchSchema(t *testing.T) {
	ap := applyPatch{}
	if ap.Name() != "apply_patch" {
		t.Fatalf("name: %s", ap.Name())
	}
	if ap.ReadOnly() {
		t.Fatal("apply_patch should not be read-only")
	}
}

func TestParseUnifiedDiffEmpty(t *testing.T) {
	_, err := parseUnifiedDiff("")
	if err == nil {
		t.Fatal("expected error for empty diff")
	}
}

func TestParseUnifiedDiffNoHeader(t *testing.T) {
	_, err := parseUnifiedDiff("just some text\nno diff here\n")
	if err == nil {
		t.Fatal("expected error for no diff headers")
	}
}

func TestParseUnifiedDiffValid(t *testing.T) {
	diff := `--- a/hello.txt
+++ b/hello.txt
@@ -1,1 +1,1 @@
-Hello
+World
`
	files, err := parseUnifiedDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "hello.txt" {
		t.Fatalf("path: %s", files[0].Path)
	}
	if files[0].Added != 1 || files[0].Removed != 1 {
		t.Fatalf("added=%d removed=%d", files[0].Added, files[0].Removed)
	}
}
