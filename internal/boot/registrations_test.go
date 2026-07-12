package boot

import (
	"slices"
	"testing"

	"reames-agent/internal/provider"
	"reames-agent/internal/tool"
)

func TestBootRegistersCompileTimeProvidersAndBuiltins(t *testing.T) {
	kinds := provider.Kinds()
	for _, kind := range []string{"anthropic", "openai"} {
		if !slices.Contains(kinds, kind) {
			t.Fatalf("boot provider kinds = %v, missing %q", kinds, kind)
		}
	}
	for _, name := range []string{"bash", "read_file", "write_file"} {
		if _, ok := tool.LookupBuiltin(name); !ok {
			t.Fatalf("boot did not register builtin %q", name)
		}
	}
}
