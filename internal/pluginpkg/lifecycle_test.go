package pluginpkg

import (
	"os"
	"testing"
)

func mustUpsertPlugin(t *testing.T, home string, plugin InstalledPlugin) {
	t.Helper()
	if err := Upsert(home, plugin); err != nil {
		t.Fatal(err)
	}
}

func TestPluginLifecycle_Install(t *testing.T) {
	home := t.TempDir()
	p := InstalledPlugin{Name: "superpowers", Version: "1.0.0", Root: "p/superpowers", Enabled: true}
	mustUpsertPlugin(t, home, p)
	st, _ := LoadState(home)
	if len(st.Plugins) != 1 || st.Plugins[0].Name != "superpowers" || st.Plugins[0].Version != "1.0.0" || !st.Plugins[0].Enabled {
		t.Fatalf("install failed: %+v", st.Plugins)
	}
}

func TestPluginLifecycle_Update(t *testing.T) {
	home := t.TempDir()
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "ci-tools", Version: "1.0", Root: "p/ci-tools", Enabled: true})
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "ci-tools", Version: "2.0", Root: "p/ci-tools-v2", Enabled: true})
	st, _ := LoadState(home)
	if len(st.Plugins) != 1 || st.Plugins[0].Version != "2.0" {
		t.Fatalf("update failed: %+v", st.Plugins)
	}
}

func TestPluginLifecycle_Disable(t *testing.T) {
	home := t.TempDir()
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "test-plugin", Version: "1.0", Root: "p/test", Enabled: true})
	if err := SetEnabled(home, "test-plugin", false); err != nil {
		t.Fatal(err)
	}
	st, _ := LoadState(home)
	if st.Plugins[0].Enabled {
		t.Fatal("should be disabled")
	}
}

func TestPluginLifecycle_ReEnable(t *testing.T) {
	home := t.TempDir()
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "test-plugin", Version: "1.0", Root: "p/test", Enabled: true})
	if err := SetEnabled(home, "test-plugin", false); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(home, "test-plugin", true); err != nil {
		t.Fatal(err)
	}
	st, _ := LoadState(home)
	if !st.Plugins[0].Enabled {
		t.Fatal("should be re-enabled")
	}
}

func TestPluginLifecycle_Remove(t *testing.T) {
	home := t.TempDir()
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "to-remove", Version: "1.0", Root: "p/remove", Enabled: true})
	removed, found, _ := Remove(home, "to-remove")
	if !found || removed.Name != "to-remove" {
		t.Fatal("remove failed")
	}
	st, _ := LoadState(home)
	if len(st.Plugins) != 0 {
		t.Fatalf("plugins after remove = %d", len(st.Plugins))
	}
	_, found, _ = Remove(home, "nonexistent")
	if found {
		t.Fatal("should not find nonexistent")
	}
}

func TestPluginLifecycle_MultiplePlugins(t *testing.T) {
	home := t.TempDir()
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "plugin-a", Version: "1.0", Root: "p/a", Enabled: true})
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "plugin-b", Version: "2.0", Root: "p/b", Enabled: true})
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "plugin-c", Version: "1.0", Root: "p/c", Enabled: false})
	st, _ := LoadState(home)
	if len(st.Plugins) != 3 {
		t.Fatalf("plugins = %d", len(st.Plugins))
	}
	found := false
	for _, p := range st.Plugins {
		if p.Name == "plugin-c" && !p.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatal("plugin-c should be present and disabled")
	}
}

func TestPluginLifecycle_InvalidName(t *testing.T) {
	home := t.TempDir()
	if err := Upsert(home, InstalledPlugin{Name: ""}); err == nil {
		t.Fatal("should reject empty name")
	}
	if err := Upsert(home, InstalledPlugin{Name: "bad/name"}); err == nil {
		t.Fatal("should reject name with slash")
	}
}

func TestPluginLifecycle_StatePersistsAcrossLoads(t *testing.T) {
	home := t.TempDir()
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "persist", Version: "1.0", Root: "p/p", Enabled: true})
	st1, _ := LoadState(home)
	st2, _ := LoadState(home)
	if st1.Plugins[0].Name != st2.Plugins[0].Name {
		t.Fatal("state should be stable")
	}
}

func TestPluginLifecycle_StateFileCreatedOnFirstUse(t *testing.T) {
	home := t.TempDir()
	if _, err := os.Stat(StatePath(home)); err == nil {
		t.Fatal("should not exist before Upsert")
	}
	mustUpsertPlugin(t, home, InstalledPlugin{Name: "first", Version: "1.0", Root: "p/first", Enabled: true})
	if _, err := os.Stat(StatePath(home)); err != nil {
		t.Fatal("should exist after Upsert")
	}
}

func TestPluginLifecycle_SetEnabledUnknown(t *testing.T) {
	if err := SetEnabled(t.TempDir(), "nonexistent", true); err == nil {
		t.Fatal("should error")
	}
}

func TestPluginLifecycle_ConcurrentUpsert(t *testing.T) {
	home := t.TempDir()
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			done <- Upsert(home, InstalledPlugin{Name: "concurrent", Version: "1.0", Root: "p/c", Enabled: true})
		}()
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	st, _ := LoadState(home)
	if len(st.Plugins) != 1 {
		t.Fatalf("concurrent Upserts: got %d", len(st.Plugins))
	}
}
