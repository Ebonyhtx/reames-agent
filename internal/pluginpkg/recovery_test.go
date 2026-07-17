package pluginpkg

import "testing"

func TestDisableAllIsAtomicAndIdempotent(t *testing.T) {
	home := t.TempDir()
	state := State{Version: StateSchemaVersion, Plugins: []InstalledPlugin{
		{Name: "alpha", Root: t.TempDir(), Enabled: true},
		{Name: "beta", Root: t.TempDir(), Enabled: false},
		{Name: "gamma", Root: t.TempDir(), Enabled: true},
	}}
	if err := SaveState(home, state); err != nil {
		t.Fatal(err)
	}
	disabled, err := DisableAll(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 2 || disabled[0] != "alpha" || disabled[1] != "gamma" {
		t.Fatalf("disabled = %v", disabled)
	}
	loaded, err := LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, plugin := range loaded.Plugins {
		if plugin.Enabled {
			t.Fatalf("plugin remained enabled: %+v", plugin)
		}
	}
	disabled, err = DisableAll(home)
	if err != nil || len(disabled) != 0 {
		t.Fatalf("second disable = %v, %v", disabled, err)
	}
}
