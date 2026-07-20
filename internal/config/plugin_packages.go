package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"reames-agent/internal/pluginpkg"
)

// mergeInstalledPluginPackages overlays enabled plugin package capabilities onto
// the in-memory config. It never writes config.toml: plugin package state lives
// in <Reames Agent home>/plugin-packages.json so uninstall/disable can remove the
// entire bundle without editing user-authored config.
func mergeInstalledPluginPackages(cfg *Config, root string) []string {
	if cfg == nil {
		return nil
	}
	reamesAgentHome := ReamesAgentHomeDir()
	if strings.TrimSpace(reamesAgentHome) == "" {
		return nil
	}
	installed, warnings := pluginpkg.LoadInstalled(reamesAgentHome)
	sort.SliceStable(installed, func(i, j int) bool {
		return installed[i].Installed.Name < installed[j].Installed.Name
	})
	for _, item := range installed {
		pkg := item.Package
		for _, warning := range item.Warnings {
			warnings = append(warnings, fmt.Sprintf("%s: %s", item.Installed.Name, warning))
		}
		for _, skillRoot := range pkg.SkillRoots() {
			if !stringSliceContainsPath(cfg.Skills.Paths, skillRoot) {
				cfg.Skills.Paths = append(cfg.Skills.Paths, skillRoot)
			}
			cfg.addPluginSkillOwner(skillRoot, item.Installed.Name)
		}
		for name, srv := range pkg.Manifest.MCPServers {
			if pluginNameExists(cfg.Plugins, name) {
				warnings = append(warnings, fmt.Sprintf("%s: plugin MCP server %q skipped because config already defines that name", item.Installed.Name, name))
				continue
			}
			entry := PluginEntry{
				Name:         name,
				Type:         srv.Type,
				Command:      pluginPackageCommand(pkg.Root, srv.Command),
				Args:         append([]string(nil), srv.Args...),
				Env:          pluginPackageEnv(reamesAgentHome, item.Installed, pkg.Root, srv.Env),
				URL:          strings.TrimSpace(srv.URL),
				Headers:      cloneStringMap(srv.Headers),
				AutoStart:    srv.AutoStart,
				Tier:         srv.Tier,
				packageOwner: item.Installed.Name,
				packageRoot:  pkg.Root,
				packageState: pluginpkg.RuntimeStateDir(reamesAgentHome, item.Installed.Name),
				packageHome:  reamesAgentHome,
				configSource: "plugin_package:" + item.Installed.Name,
			}
			cfg.Plugins = append(cfg.Plugins, entry)
		}
	}
	return warnings
}

func (c *Config) addPluginSkillOwner(root, owner string) {
	if c == nil {
		return
	}
	root = CanonicalSkillPath(root)
	owner = strings.TrimSpace(owner)
	if root == "" || owner == "" {
		return
	}
	if c.pluginPackageSkillOwners == nil {
		c.pluginPackageSkillOwners = map[string][]string{}
	}
	for _, existing := range c.pluginPackageSkillOwners[root] {
		if existing == owner {
			return
		}
	}
	c.pluginPackageSkillOwners[root] = append(c.pluginPackageSkillOwners[root], owner)
	sort.Strings(c.pluginPackageSkillOwners[root])
}

// PluginPackageSkillOwners returns installed package owners keyed by canonical
// skill-root path. The defensive copy keeps runtime provenance host-owned.
func (c *Config) PluginPackageSkillOwners() map[string][]string {
	if c == nil || len(c.pluginPackageSkillOwners) == 0 {
		return nil
	}
	out := make(map[string][]string, len(c.pluginPackageSkillOwners))
	for root, owners := range c.pluginPackageSkillOwners {
		out[root] = append([]string(nil), owners...)
	}
	return out
}

func pluginPackageCommand(root, command string) string {
	command = strings.TrimSpace(command)
	if command == "" || filepath.IsAbs(command) {
		return command
	}
	return filepath.Join(root, filepath.FromSlash(command))
}

func pluginPackageEnv(reamesAgentHome string, installed pluginpkg.InstalledPlugin, root string, env map[string]string) map[string]string {
	out := cloneStringMap(env)
	if out == nil {
		out = map[string]string{}
	}
	out["REAMES_AGENT_PLUGIN_ROOT"] = root
	out["REAMES_AGENT_PLUGIN_NAME"] = installed.Name
	out["REAMES_AGENT_PLUGIN_STATE"] = pluginpkg.RuntimeStateDir(reamesAgentHome, installed.Name)
	if installed.Version != "" {
		out["REAMES_AGENT_PLUGIN_VERSION"] = installed.Version
	}
	return out
}

func pluginNameExists(entries []PluginEntry, name string) bool {
	for _, p := range entries {
		if p.Name == name {
			return true
		}
	}
	return false
}

func stringSliceContainsPath(paths []string, path string) bool {
	canon := CanonicalSkillPath(path)
	for _, existing := range paths {
		if CanonicalSkillPath(ExpandVars(existing)) == canon {
			return true
		}
	}
	return false
}
