package installsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"reames-agent/internal/pluginpkg"
)

type preparedPluginSource struct {
	Root     string
	Kind     string
	Revision string
	Trust    string
}

func (t *installSourceTool) localPluginPackageAction(req request, root string) (action, []string, error) {
	pkg, warnings, digest, err := pluginpkg.InspectDir(root)
	if err != nil {
		return action{}, warnings, newErr(ErrManifestMissing, "%v", err)
	}
	trust := pluginpkg.TrustLocalSnapshot
	if modeForPlugin(req.Mode) == pluginpkg.InstallModeLink {
		trust = pluginpkg.TrustMutableLink
	}
	act, err := t.pluginPackageAction(req, pkg, root, preparedPluginSource{
		Root:  root,
		Kind:  pluginpkg.SourceKindLocal,
		Trust: trust,
	}, digest)
	return act, warnings, err
}

func (t *installSourceTool) planGitHubPluginPackage(ctx context.Context, req request) ([]action, []string, error) {
	if modeForPlugin(req.Mode) == pluginpkg.InstallModeLink {
		return nil, nil, newErr(ErrUnsafeLinkTarget, "remote plugin sources cannot use link mode")
	}
	prepared, cleanup, err := t.preparePluginSource(ctx, req.Source, pluginpkg.InstallModeCopy)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()
	pkg, warnings, digest, err := pluginpkg.InspectDir(prepared.Root)
	if err != nil {
		return nil, warnings, newErr(ErrManifestMissing, "%v", err)
	}
	act, err := t.pluginPackageAction(req, pkg, req.Source, prepared, digest)
	if err != nil {
		return nil, warnings, err
	}
	return []action{act}, warnings, nil
}

func (t *installSourceTool) pluginPackageAction(req request, pkg pluginpkg.Package, source string, prepared preparedPluginSource, digest string) (action, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = pkg.Manifest.Name
	}
	if !pluginpkg.IsValidName(name) {
		return action{}, newErr(ErrInvalidManifest, "invalid plugin name %q", name)
	}
	root := ""
	if t.reamesAgentHome != "" {
		root = pluginpkg.InstallRoot(t.reamesAgentHome, name)
	}
	skills, hooks, mcp := pkg.CapabilityCounts()
	a := action{
		Kind:             "plugin",
		Action:           "install_plugin_package",
		Name:             name,
		Source:           source,
		Target:           root,
		Scope:            "global",
		Mode:             modeForPlugin(req.Mode),
		ConfigPath:       pluginpkg.StatePath(t.reamesAgentHome),
		Skills:           append([]string(nil), pkg.Manifest.Skills...),
		SkillCount:       skills,
		HookCount:        hooks,
		ToolCount:        mcp,
		ManifestKind:     pkg.ManifestKind,
		Version:          pkg.Manifest.Version,
		Digest:           digest,
		Permissions:      append([]string(nil), pkg.Manifest.Permissions...),
		PermissionSource: pkg.Manifest.PermissionSource,
		SourceKind:       prepared.Kind,
		SourceRevision:   prepared.Revision,
		TrustStatus:      prepared.Trust,
		RiskLevel:        RiskMedium,
		RiskReasons:      []string{"installs a plugin package disabled until its permissions are explicitly granted"},
	}
	if a.Mode == pluginpkg.InstallModeLink {
		a.RiskLevel = RiskHigh
		a.RiskReasons = append(a.RiskReasons, "links a mutable local directory; content changes require re-enabling before they load")
	}
	if hooks > 0 {
		a.RiskLevel = RiskHigh
		a.RiskReasons = append(a.RiskReasons, "declares hooks that can inject context or execute commands during sessions")
	}
	if mcp > 0 {
		a.RiskLevel = RiskHigh
		a.RiskReasons = append(a.RiskReasons, "declares MCP servers that can spawn processes or connect to remote endpoints")
	}
	if len(a.Permissions) > 0 {
		a.RiskReasons = append(a.RiskReasons, "requests permissions: "+strings.Join(a.Permissions, ", "))
	}
	if prepared.Trust == pluginpkg.TrustGitHubUnsigned {
		a.RiskReasons = append(a.RiskReasons, "GitHub HTTPS transport is recorded but the package has no Reames signature verification")
	}
	if installed, ok, err := pluginpkg.FindInstalled(t.reamesAgentHome, name); err != nil {
		return action{}, err
	} else if ok {
		a.Action = "update_plugin_package"
		a.CurrentVersion = installed.Version
		a.CurrentDigest = installed.Digest
		a.CurrentStateToken = pluginpkg.InstalledStateToken(installed)
		a.AddedPermissions, a.RemovedPermissions = permissionDiff(installed.Permissions, a.Permissions)
		a.WillEnable = installed.Enabled && permissionSetCovers(installed.GrantedPermissions, a.Permissions)
		a.RollbackAvailable = installed.Previous != nil || (installed.InstallMode == pluginpkg.InstallModeCopy && installed.Digest != digest)
		if len(a.AddedPermissions) > 0 {
			a.RiskLevel = RiskHigh
			a.RiskReasons = append(a.RiskReasons, "adds permissions: "+strings.Join(a.AddedPermissions, ", "))
		}
	}
	sort.Strings(a.Skills)
	return a, nil
}

func modeForPlugin(mode string) string {
	if mode == pluginpkg.InstallModeLink {
		return pluginpkg.InstallModeLink
	}
	return pluginpkg.InstallModeCopy
}

func permissionDiff(current, next []string) (added, removed []string) {
	currentSet := map[string]bool{}
	nextSet := map[string]bool{}
	for _, permission := range current {
		currentSet[permission] = true
	}
	for _, permission := range next {
		nextSet[permission] = true
		if !currentSet[permission] {
			added = append(added, permission)
		}
	}
	for _, permission := range current {
		if !nextSet[permission] {
			removed = append(removed, permission)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func permissionSetCovers(granted, required []string) bool {
	set := map[string]bool{}
	for _, permission := range granted {
		set[permission] = true
	}
	for _, permission := range required {
		if !set[permission] {
			return false
		}
	}
	return true
}

func (t *installSourceTool) applyInstallPluginPackage(ctx context.Context, req request, act *action) error {
	if t.reamesAgentHome == "" {
		return newErr(ErrSourceUnreadable, "plugin install requires a Reames Agent home directory")
	}
	if !pluginpkg.IsValidName(act.Name) {
		return newErr(ErrInvalidManifest, "invalid plugin name %q", act.Name)
	}
	var previousMCPServers []string
	if act.CurrentStateToken != "" {
		previous, ok, err := pluginpkg.FindInstalled(t.reamesAgentHome, act.Name)
		if err != nil {
			return err
		}
		if !ok {
			return newErr(ErrApprovalDenied, "plugin state changed after planning: %q is no longer installed", act.Name)
		}
		previousMCPServers, err = pluginMCPServerNames(t.reamesAgentHome, previous)
		if err != nil {
			return err
		}
	}
	prepared, cleanup, err := t.preparePluginSource(ctx, act.Source, act.Mode)
	if err != nil {
		return err
	}
	defer cleanup()
	pkg, warnings, digest, err := pluginpkg.InspectDir(prepared.Root)
	if err != nil {
		return newErr(ErrInvalidManifest, "%v", err)
	}
	act.Warnings = append(act.Warnings, warnings...)
	if pkg.Manifest.Name != act.Name && strings.TrimSpace(req.Name) == "" {
		return newErr(ErrInvalidManifest, "planned plugin name %q but source now reports %q", act.Name, pkg.Manifest.Name)
	}
	if act.Digest != "" && digest != act.Digest {
		return newErr(ErrApprovalDenied, "plugin content changed after planning: got %s, want %s", digest, act.Digest)
	}
	if act.SourceRevision != "" && prepared.Revision != act.SourceRevision {
		return newErr(ErrApprovalDenied, "plugin source revision changed after planning: got %s, want %s", prepared.Revision, act.SourceRevision)
	}
	if act.Mode == pluginpkg.InstallModeLink && !isLinkTargetSafe(prepared.Root, t.home, t.root) {
		return newErr(ErrUnsafeLinkTarget, "plugin source %s is outside %s and %s", prepared.Root, t.root, t.home)
	}
	result, err := pluginpkg.Install(t.reamesAgentHome, pluginpkg.InstallRequest{
		Name:                 act.Name,
		Source:               act.Source,
		SourceRoot:           prepared.Root,
		SourceKind:           prepared.Kind,
		SourceRevision:       prepared.Revision,
		TrustStatus:          prepared.Trust,
		Mode:                 act.Mode,
		ExpectedDigest:       digest,
		ExpectedCurrentState: act.CurrentStateToken,
		BindCurrentState:     true,
		Replace:              req.Replace,
		AllowNameOverride:    strings.TrimSpace(req.Name) != "",
	})
	if err != nil {
		if errors.Is(err, pluginpkg.ErrAlreadyInstalled) {
			return newErr(ErrAlreadyExists, "%v; retry with replace=true to update it", err)
		}
		return err
	}
	installed := result.Installed
	act.Warnings = append(act.Warnings, result.Warnings...)
	act.Target = pluginpkg.ResolveRoot(t.reamesAgentHome, installed.Root)
	act.ManifestKind = installed.ManifestKind
	act.Version = installed.Version
	act.Digest = installed.Digest
	act.Permissions = append([]string(nil), installed.Permissions...)
	act.SourceKind = installed.SourceKind
	act.SourceRevision = installed.SourceRevision
	act.TrustStatus = installed.TrustStatus
	act.WillEnable = installed.Enabled
	act.RollbackAvailable = installed.Previous != nil
	act.SkillCount, act.HookCount, act.ToolCount = pkg.CapabilityCounts()
	if act.CurrentStateToken != "" {
		act.Warnings = append(act.Warnings, t.revokePluginRuntime(act.Name)...)
	}
	if removed := t.revokePluginMCPServers(act.Name, previousMCPServers); removed > 0 {
		act.Warnings = append(act.Warnings, fmt.Sprintf("disconnected %d MCP server(s) from the previous plugin generation; open a new session to load the verified replacement", removed))
	}
	return nil
}

func (t *installSourceTool) preparePluginSource(ctx context.Context, source, mode string) (preparedPluginSource, func(), error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "git:github.com/") {
		source = "https://github.com/" + strings.TrimPrefix(source, "git:github.com/")
	}
	if isURL(source) {
		if mode == pluginpkg.InstallModeLink {
			return preparedPluginSource{}, func() {}, newErr(ErrUnsafeLinkTarget, "remote plugin sources cannot use link mode")
		}
		src, ok := parseGitHubRepoSource(source)
		if !ok {
			return preparedPluginSource{}, func() {}, newErr(ErrUnsupportedKind, "plugin URL %q is not a GitHub repository", source)
		}
		if src.Path != "" {
			clean := filepath.Clean(filepath.FromSlash(src.Path))
			if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return preparedPluginSource{}, func() {}, newErr(ErrInvalidManifest, "plugin repository subpath %q escapes the repository", src.Path)
			}
		}
		tmp, err := os.MkdirTemp("", "reames-agent-plugin-*")
		if err != nil {
			return preparedPluginSource{}, func() {}, err
		}
		cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", src.Owner, src.Repo)
		args := []string{"clone", "--depth=1"}
		if src.Branch != "" {
			args = append(args, "--branch", src.Branch)
		}
		args = append(args, cloneURL, tmp)
		cmd := exec.CommandContext(ctx, "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.RemoveAll(tmp)
			return preparedPluginSource{}, func() {}, newErr(ErrSourceUnreadable, "git clone failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		revisionOut, err := exec.CommandContext(ctx, "git", "-C", tmp, "rev-parse", "HEAD").CombinedOutput()
		if err != nil {
			_ = os.RemoveAll(tmp)
			return preparedPluginSource{}, func() {}, newErr(ErrSourceUnreadable, "resolve cloned plugin revision: %v: %s", err, strings.TrimSpace(string(revisionOut)))
		}
		root := tmp
		if src.Path != "" {
			root = filepath.Join(tmp, filepath.FromSlash(src.Path))
		}
		return preparedPluginSource{
			Root:     root,
			Kind:     pluginpkg.SourceKindGitHub,
			Revision: strings.TrimSpace(string(revisionOut)),
			Trust:    pluginpkg.TrustGitHubUnsigned,
		}, func() { _ = os.RemoveAll(tmp) }, nil
	}
	path := t.resolvePath(source)
	abs, err := filepath.Abs(path)
	if err != nil {
		return preparedPluginSource{}, func() {}, err
	}
	trust := pluginpkg.TrustLocalSnapshot
	if mode == pluginpkg.InstallModeLink {
		trust = pluginpkg.TrustMutableLink
	}
	return preparedPluginSource{Root: abs, Kind: pluginpkg.SourceKindLocal, Trust: trust}, func() {}, nil
}

func (t *installSourceTool) applyRemovePluginPackage(_ request, act *action) error {
	installed, ok, err := pluginpkg.FindInstalled(t.reamesAgentHome, act.Name)
	if err != nil || !ok {
		return err
	}
	serverNames, err := pluginMCPServerNames(t.reamesAgentHome, installed)
	if err != nil {
		return err
	}
	_, warnings, found, err := pluginpkg.UninstallApproved(t.reamesAgentHome, pluginpkg.UninstallRequest{
		Name: act.Name, ExpectedCurrentState: act.CurrentStateToken, BindCurrentState: true,
	})
	act.Warnings = append(act.Warnings, warnings...)
	if err != nil {
		return err
	}
	if !found {
		return newErr(ErrApprovalDenied, "plugin state changed after planning: %q is no longer installed", act.Name)
	}
	act.Warnings = append(act.Warnings, t.revokePluginRuntime(act.Name)...)
	if removed := t.revokePluginMCPServers(act.Name, serverNames); removed > 0 {
		act.Warnings = append(act.Warnings, fmt.Sprintf("disconnected %d MCP server(s) owned by the removed plugin", removed))
	}
	return nil
}

func (t *installSourceTool) applyRollbackPluginPackage(act *action) error {
	current, ok, err := pluginpkg.FindInstalled(t.reamesAgentHome, act.Name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("plugin %q is not installed", act.Name)
	}
	serverNames, err := pluginMCPServerNames(t.reamesAgentHome, current)
	if err != nil {
		return err
	}
	restored, warnings, err := pluginpkg.RollbackApproved(t.reamesAgentHome, pluginpkg.RollbackRequest{
		Name: act.Name, ExpectedCurrentState: act.CurrentStateToken, BindCurrentState: true,
	})
	act.Warnings = append(act.Warnings, warnings...)
	if err != nil {
		return err
	}
	act.Target = pluginpkg.ResolveRoot(t.reamesAgentHome, restored.Root)
	act.Version = restored.Version
	act.Digest = restored.Digest
	act.Permissions = append([]string(nil), restored.Permissions...)
	act.SourceKind = restored.SourceKind
	act.SourceRevision = restored.SourceRevision
	act.TrustStatus = restored.TrustStatus
	act.WillEnable = restored.Enabled
	act.RollbackAvailable = restored.Previous != nil
	act.Warnings = append(act.Warnings, t.revokePluginRuntime(act.Name)...)
	if removed := t.revokePluginMCPServers(act.Name, serverNames); removed > 0 {
		act.Warnings = append(act.Warnings, fmt.Sprintf("disconnected %d MCP server(s) from the replaced plugin generation; open a new session to load the verified rollback", removed))
	}
	return nil
}

func pluginMCPServerNames(reamesAgentHome string, installed pluginpkg.InstalledPlugin) ([]string, error) {
	if installed.MCPServerNamesBound {
		return append([]string(nil), installed.MCPServerNames...), nil
	}
	verified, err := pluginpkg.VerifyInstalled(reamesAgentHome, installed.Name)
	if err != nil {
		return nil, fmt.Errorf("verify plugin MCP ownership before lifecycle mutation: %w", err)
	}
	names := make([]string, 0, len(verified.Package.Manifest.MCPServers))
	for name := range verified.Package.Manifest.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (t *installSourceTool) revokePluginMCPServers(pluginName string, serverNames []string) int {
	if t.onPluginDisconnect == nil {
		return 0
	}
	removed := 0
	for _, serverName := range serverNames {
		if t.onPluginDisconnect(pluginName, serverName) {
			removed++
		}
	}
	return removed
}

func (t *installSourceTool) revokePluginRuntime(pluginName string) []string {
	if t.onPluginRuntimeChange == nil {
		return nil
	}
	return t.onPluginRuntimeChange(pluginName)
}
