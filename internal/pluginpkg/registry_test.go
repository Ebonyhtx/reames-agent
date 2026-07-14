package pluginpkg

import (
	"testing"
)

func TestSearchEmpty(t *testing.T) {
	idx := &RegistryIndex{Plugins: []RegistryEntry{
		{Name: "code-review", Description: "PR review", Category: "dev"},
		{Name: "hookify", Description: "Create hooks", Category: "tools"},
	}}
	r := Search(idx, "")
	if len(r) != 2 {
		t.Fatalf("expected 2, got %d", len(r))
	}
}

func TestSearchMatch(t *testing.T) {
	idx := &RegistryIndex{Plugins: []RegistryEntry{
		{Name: "code-review", Description: "PR review", Category: "dev"},
		{Name: "hookify", Description: "Create hooks from patterns", Category: "tools"},
		{Name: "deploy", Description: "CI/CD deployment", Category: "ops"},
	}}
	r := Search(idx, "hook")
	if len(r) != 1 || r[0].Name != "hookify" {
		t.Fatalf("expected hookify, got %+v", r)
	}
}

func TestSearchNoMatch(t *testing.T) {
	idx := &RegistryIndex{Plugins: []RegistryEntry{
		{Name: "code-review", Description: "PR review", Category: "dev"},
	}}
	r := Search(idx, "nonexistent")
	if len(r) != 0 {
		t.Fatalf("expected 0, got %d", len(r))
	}
}

func TestByCategory(t *testing.T) {
	idx := &RegistryIndex{Plugins: []RegistryEntry{
		{Name: "a", Category: "dev"},
		{Name: "b", Category: "dev"},
		{Name: "c", Category: "tools"},
		{Name: "d"},
	}}
	m := ByCategory(idx)
	if len(m) != 3 {
		t.Fatalf("expected 3 categories, got %d: %v", len(m), m)
	}
	if len(m["dev"]) != 2 {
		t.Fatalf("expected 2 dev plugins")
	}
	if len(m["other"]) != 1 {
		t.Fatalf("expected 1 other plugin, got %d", len(m["other"]))
	}
}

func TestFetchRegistryRequiresExplicitURL(t *testing.T) {
	if _, err := FetchRegistry("  ", nil); err == nil {
		t.Fatal("FetchRegistry without an explicit URL should fail")
	}
}
