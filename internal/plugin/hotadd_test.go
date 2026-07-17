package plugin

import (
	"context"
	"sync"
	"testing"
	"time"

	"reames-agent/internal/tool"
)

// TestHostAddRemove exercises the hot add/remove path behind `/mcp add` and
// `/mcp remove`: a server connects live into an existing host, its namespaced
// tools surface, a duplicate name is rejected, and removal disconnects it and
// reports the tool prefix to unregister.
func TestHostAddRemove(t *testing.T) {
	srv := mcpHTTPServer(t, false)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h := NewHost()
	defer h.Close()

	spec := Spec{Name: "h", Type: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer secret"}}
	tools, err := h.Add(ctx, spec)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(tools) != 1 || tools[0].Name() != "mcp__h__greet" {
		t.Fatalf("tools = %v, want [mcp__h__greet]", names(tools))
	}
	if got := h.Servers(); len(got) != 1 || got[0].Name != "h" || got[0].Tools != 1 {
		t.Fatalf("Servers() = %+v, want one server 'h' with 1 tool", got)
	}

	// A second add under the same name is rejected (no duplicate connection).
	if _, err := h.Add(ctx, spec); err == nil {
		t.Error("Add of an already-connected name should error")
	}

	prefix, found := h.Remove("h")
	if !found || prefix != "mcp__h__" {
		t.Fatalf("Remove = (%q, %v), want (\"mcp__h__\", true)", prefix, found)
	}
	if len(h.Servers()) != 0 {
		t.Errorf("server should be gone after Remove, got %+v", h.Servers())
	}
	if _, found := h.Remove("h"); found {
		t.Error("removing an absent server should report not found")
	}
}

func TestHostReconnectRefreshesEveryAttachedRegistry(t *testing.T) {
	srv := mcpHTTPServer(t, false)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host := NewHost()
	defer host.Close()
	active := tool.NewRegistry()
	sibling := tool.NewRegistry()
	host.AttachRegistry(active)
	host.AttachRegistry(sibling)
	defer host.DetachRegistry(active)
	defer host.DetachRegistry(sibling)

	spec := Spec{Name: "h", Type: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer secret"}}
	if _, err := host.Add(ctx, spec); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	for label, registry := range map[string]*tool.Registry{"active": active, "sibling": sibling} {
		if _, ok := registry.Get("mcp__h__greet"); !ok {
			t.Fatalf("%s registry did not receive initial shared-host tools", label)
		}
	}
	if _, ok := host.Remove("h"); !ok {
		t.Fatal("Remove(h) reported no connected server")
	}
	for label, registry := range map[string]*tool.Registry{"active": active, "sibling": sibling} {
		if _, ok := registry.Get("mcp__h__greet"); ok {
			t.Fatalf("%s registry retained a stale adapter across shared-host removal", label)
		}
	}
	if _, err := host.Add(ctx, spec); err != nil {
		t.Fatalf("second Add: %v", err)
	}
	for label, registry := range map[string]*tool.Registry{"active": active, "sibling": sibling} {
		if _, ok := registry.Get("mcp__h__greet"); !ok {
			t.Fatalf("%s registry was not refreshed after shared-host reconnect", label)
		}
	}
	host.Close()
	for label, registry := range map[string]*tool.Registry{"active": active, "sibling": sibling} {
		if _, ok := registry.Get("mcp__h__greet"); ok {
			t.Fatalf("%s registry retained a stale adapter after shared-host close", label)
		}
	}
}

func TestHostAddConnectedRejectsLateDuplicate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h := NewHost()
	defer h.Close()

	spec := helperSpec()
	if _, err := h.addConnected(ctx, spec); err != nil {
		t.Fatalf("first addConnected: %v", err)
	}
	if _, err := h.addConnected(ctx, spec); !IsServerAlreadyConnected(err) {
		t.Fatalf("second addConnected error = %v, want ErrServerAlreadyConnected", err)
	}
	if got := h.ServerNames(); len(got) != 1 || got[0] != spec.Name {
		t.Fatalf("ServerNames() = %v, want exactly one %q", got, spec.Name)
	}
}

func TestHostAddConcurrentSameServerReusesSingleClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h := NewHost()
	defer h.Close()

	spec := helperSpec()
	spec.Env["GO_WANT_HELPER_INIT_MS"] = "100"

	const callers = 5
	var wg sync.WaitGroup
	errs := make([]error, callers)
	counts := make([]int, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			tools, err := h.Add(ctx, spec)
			errs[i] = err
			counts[i] = len(tools)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d Add: %v", i, err)
		}
		if counts[i] != 2 {
			t.Fatalf("caller %d got %d tools, want 2", i, counts[i])
		}
	}
	if got := h.ServerNames(); len(got) != 1 || got[0] != spec.Name {
		t.Fatalf("ServerNames() = %v, want exactly one %q", got, spec.Name)
	}
}
