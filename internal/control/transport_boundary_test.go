package control

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// legacyTransportRuntimeImports is the migration baseline, not an endorsement.
// Keep it exact: removing an entry is progress, while adding one requires an
// explicit architecture decision in this test and the development plan.
var legacyTransportRuntimeImports = map[string][]string{
	"desktop/app.go":           {"reames-agent/internal/agent"},
	"desktop/main.go":          {"reames-agent/internal/provider/anthropic", "reames-agent/internal/provider/openai", "reames-agent/internal/tool/builtin"},
	"desktop/tabs.go":          {"reames-agent/internal/agent"},
	"internal/cli/chat_tui.go": {"reames-agent/internal/agent", "reames-agent/internal/provider"},
	"internal/cli/cli.go":      {"reames-agent/internal/agent", "reames-agent/internal/provider", "reames-agent/internal/provider/openai"},
	"internal/cli/review.go":   {"reames-agent/internal/agent", "reames-agent/internal/tool", "reames-agent/internal/tool/builtin"},
}

func TestTransportRuntimeImportRatchet(t *testing.T) {
	root := repositoryRoot(t)
	targets := []string{"desktop", "internal/acp", "internal/bot", "internal/botruntime", "internal/cli", "internal/serve"}
	actual := make(map[string]map[string]bool)

	for _, target := range targets {
		base := filepath.Join(root, filepath.FromSlash(target))
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "build", "frontend", "node_modules", "vendor":
					if path != base {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}

			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("relative path for %s: %w", path, err)
			}
			rel = filepath.ToSlash(rel)
			for _, spec := range parsed.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return fmt.Errorf("unquote import in %s: %w", rel, err)
				}
				if isRuntimeInternalImport(importPath) {
					if actual[rel] == nil {
						actual[rel] = make(map[string]bool)
					}
					actual[rel][importPath] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan transport root %s: %v", target, err)
		}
	}

	expected := make(map[string]map[string]bool, len(legacyTransportRuntimeImports))
	for file, imports := range legacyTransportRuntimeImports {
		expected[file] = make(map[string]bool, len(imports))
		for _, importPath := range imports {
			expected[file][importPath] = true
		}
	}

	var unexpected, removed []string
	for file, imports := range actual {
		for importPath := range imports {
			if !expected[file][importPath] {
				unexpected = append(unexpected, file+" -> "+importPath)
			}
		}
	}
	for file, imports := range expected {
		for importPath := range imports {
			if !actual[file][importPath] {
				removed = append(removed, file+" -> "+importPath)
			}
		}
	}
	sort.Strings(unexpected)
	sort.Strings(removed)
	if len(unexpected) > 0 {
		t.Errorf("transport packages added direct runtime imports; route behavior through control/eventwire instead:\n%s", strings.Join(unexpected, "\n"))
	}
	if len(removed) > 0 {
		t.Errorf("transport runtime imports were removed; shrink legacyTransportRuntimeImports to preserve the ratchet:\n%s", strings.Join(removed, "\n"))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func isRuntimeInternalImport(importPath string) bool {
	for _, prefix := range []string{
		"reames-agent/internal/agent",
		"reames-agent/internal/provider",
		"reames-agent/internal/tool",
	} {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}
