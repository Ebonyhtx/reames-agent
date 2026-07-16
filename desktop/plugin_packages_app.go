package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/control"
	"reames-agent/internal/installsource"
	"reames-agent/internal/netclient"
	"reames-agent/internal/pluginpkg"
	"reames-agent/internal/pluginregistry"
)

type PluginView struct {
	Name                string                `json:"name"`
	Version             string                `json:"version,omitempty"`
	Description         string                `json:"description,omitempty"`
	Source              string                `json:"source,omitempty"`
	Root                string                `json:"root"`
	ManifestKind        string                `json:"manifestKind,omitempty"`
	ManifestSchema      int                   `json:"manifestSchema,omitempty"`
	InstallMode         string                `json:"installMode,omitempty"`
	SourceKind          string                `json:"sourceKind,omitempty"`
	SourceRevision      string                `json:"sourceRevision,omitempty"`
	TrustStatus         string                `json:"trustStatus,omitempty"`
	RegistryName        string                `json:"registryName,omitempty"`
	RegistryMetadataURL string                `json:"registryMetadataUrl,omitempty"`
	RegistryRootVersion int64                 `json:"registryRootVersion,omitempty"`
	RegistryRootDigest  string                `json:"registryRootDigest,omitempty"`
	RegistryEntryDigest string                `json:"registryEntryDigest,omitempty"`
	ProvenanceStatus    string                `json:"provenanceStatus,omitempty"`
	AttestationDigest   string                `json:"attestationDigest,omitempty"`
	Digest              string                `json:"digest,omitempty"`
	Permissions         []string              `json:"permissions,omitempty"`
	GrantedPermissions  []string              `json:"grantedPermissions,omitempty"`
	LifecycleSecurity   int                   `json:"lifecycleSecurity,omitempty"`
	Rollback            *PluginRollbackView   `json:"rollback,omitempty"`
	Enabled             bool                  `json:"enabled"`
	Skills              int                   `json:"skills"`
	Hooks               int                   `json:"hooks"`
	MCPServers          int                   `json:"mcpServers"`
	SkillDetails        []PluginSkillView     `json:"skillDetails,omitempty"`
	HookDetails         []PluginHookView      `json:"hookDetails,omitempty"`
	MCPServerDetails    []PluginMCPServerView `json:"mcpServerDetails,omitempty"`
	Warnings            []string              `json:"warnings,omitempty"`
	Error               string                `json:"error,omitempty"`
}

type PluginInstallOptions struct {
	DryRun  bool   `json:"dryRun,omitempty"`
	Link    bool   `json:"link,omitempty"`
	Replace bool   `json:"replace,omitempty"`
	Name    string `json:"name,omitempty"`
	PlanID  string `json:"planId,omitempty"`
}

type PluginRegistryEntryView struct {
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Version             string   `json:"version"`
	Author              string   `json:"author,omitempty"`
	Category            string   `json:"category,omitempty"`
	Source              string   `json:"source"`
	Subpath             string   `json:"subpath,omitempty"`
	Revision            string   `json:"revision"`
	Digest              string   `json:"digest"`
	Permissions         []string `json:"permissions"`
	RegistryName        string   `json:"registryName"`
	RegistryMetadataURL string   `json:"registryMetadataUrl"`
	RegistryRootVersion int64    `json:"registryRootVersion"`
	RegistryRootDigest  string   `json:"registryRootDigest"`
	RegistryEntryDigest string   `json:"registryEntryDigest"`
	ProvenanceStatus    string   `json:"provenanceStatus"`
	AttestationDigest   string   `json:"attestationDigest,omitempty"`
}

type PluginRollbackView struct {
	Version             string   `json:"version,omitempty"`
	Digest              string   `json:"digest,omitempty"`
	TrustStatus         string   `json:"trustStatus,omitempty"`
	RegistryEntryDigest string   `json:"registryEntryDigest,omitempty"`
	Permissions         []string `json:"permissions,omitempty"`
	GrantedPermissions  []string `json:"grantedPermissions,omitempty"`
	Enabled             bool     `json:"enabled"`
}

type PluginOperationKinds struct {
	Skill  int `json:"skill"`
	MCP    int `json:"mcp"`
	Plugin int `json:"plugin"`
}

type PluginOperationAction struct {
	Kind                string   `json:"kind,omitempty"`
	Action              string   `json:"action,omitempty"`
	Status              string   `json:"status,omitempty"`
	RiskLevel           string   `json:"riskLevel,omitempty"`
	RiskReasons         []string `json:"riskReasons,omitempty"`
	Name                string   `json:"name,omitempty"`
	Source              string   `json:"source,omitempty"`
	Target              string   `json:"target,omitempty"`
	Scope               string   `json:"scope,omitempty"`
	Mode                string   `json:"mode,omitempty"`
	ManifestKind        string   `json:"manifestKind,omitempty"`
	Version             string   `json:"version,omitempty"`
	CurrentVersion      string   `json:"currentVersion,omitempty"`
	Digest              string   `json:"digest,omitempty"`
	CurrentDigest       string   `json:"currentDigest,omitempty"`
	Permissions         []string `json:"permissions,omitempty"`
	AddedPermissions    []string `json:"addedPermissions,omitempty"`
	RemovedPermissions  []string `json:"removedPermissions,omitempty"`
	PermissionSource    string   `json:"permissionSource,omitempty"`
	SourceKind          string   `json:"sourceKind,omitempty"`
	SourceRevision      string   `json:"sourceRevision,omitempty"`
	RegistryName        string   `json:"registryName,omitempty"`
	RegistryMetadataURL string   `json:"registryMetadataUrl,omitempty"`
	RegistryRootVersion int64    `json:"registryRootVersion,omitempty"`
	RegistryRootDigest  string   `json:"registryRootDigest,omitempty"`
	RegistryEntryDigest string   `json:"registryEntryDigest,omitempty"`
	ProvenanceStatus    string   `json:"provenanceStatus,omitempty"`
	AttestationDigest   string   `json:"attestationDigest,omitempty"`
	TrustStatus         string   `json:"trustStatus,omitempty"`
	WillEnable          bool     `json:"willEnable"`
	RollbackAvailable   bool     `json:"rollbackAvailable"`
	Warnings            []string `json:"warnings,omitempty"`
	Error               string   `json:"error,omitempty"`
	Next                string   `json:"next,omitempty"`
}

type PluginOperationView struct {
	OK       bool                    `json:"ok"`
	Status   string                  `json:"status"`
	Op       string                  `json:"op"`
	Applied  bool                    `json:"applied"`
	Source   string                  `json:"source,omitempty"`
	Name     string                  `json:"name,omitempty"`
	Kind     string                  `json:"kind,omitempty"`
	Kinds    PluginOperationKinds    `json:"kinds,omitempty"`
	Scope    string                  `json:"scope,omitempty"`
	Mode     string                  `json:"mode,omitempty"`
	PlanID   string                  `json:"planId,omitempty"`
	Actions  []PluginOperationAction `json:"actions"`
	Warnings []string                `json:"warnings,omitempty"`
	Next     string                  `json:"next,omitempty"`
}

type PluginSkillView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Invocation  string `json:"invocation,omitempty"`
	RunAs       string `json:"runAs,omitempty"`
}

type PluginHookView struct {
	Event       string `json:"event"`
	Match       string `json:"match,omitempty"`
	Command     string `json:"command,omitempty"`
	ContextFile string `json:"contextFile,omitempty"`
	Description string `json:"description,omitempty"`
}

type PluginMCPServerView struct {
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"`
	Command   string `json:"command,omitempty"`
	URL       string `json:"url,omitempty"`
}

func (a *App) Plugins() []PluginView {
	home := config.ReamesAgentHomeDir()
	st, err := pluginpkg.LoadState(home)
	if err != nil {
		return []PluginView{{Error: err.Error()}}
	}
	out := make([]PluginView, 0, len(st.Plugins))
	for _, p := range st.Plugins {
		view := pluginViewFromInstalled(home, p)
		verified, verifyErr := pluginpkg.VerifyInstalled(home, p.Name)
		if verifyErr != nil {
			view.Error = verifyErr.Error()
			out = append(out, view)
			continue
		}
		view = pluginViewFromInstalled(home, verified.Installed)
		applyPluginPackageDetails(&view, verified.Package, verified.Warnings)
		out = append(out, view)
	}
	return out
}

// SearchPluginRegistry returns only entries authenticated by the configured
// user-global TUF root. An unconfigured registry is an explicit error rather
// than a fallback to an unsigned marketplace document.
func (a *App) SearchPluginRegistry(query string) ([]PluginRegistryEntryView, error) {
	_, client, err := desktopPluginRegistryDependencies(a.pluginRegistryWorkspaceRoot())
	if err != nil {
		return nil, err
	}
	index, err := client.Refresh(context.Background())
	if err != nil {
		return nil, err
	}
	entries := pluginregistry.Search(index, query)
	out := make([]PluginRegistryEntryView, 0, len(entries))
	for _, entry := range entries {
		out = append(out, pluginRegistryEntryView(entry))
	}
	return out, nil
}

// PluginRegistryEntry resolves one entry and verifies its optional attestation
// target before the Desktop offers it for installation.
func (a *App) PluginRegistryEntry(name string) (PluginRegistryEntryView, error) {
	_, client, err := desktopPluginRegistryDependencies(a.pluginRegistryWorkspaceRoot())
	if err != nil {
		return PluginRegistryEntryView{}, err
	}
	entry, err := client.Resolve(context.Background(), name)
	if err != nil {
		return PluginRegistryEntryView{}, err
	}
	return pluginRegistryEntryView(entry), nil
}

func (a *App) pluginRegistryWorkspaceRoot() string {
	if tab := a.activeTab(); tab != nil {
		a.reconcileTabWithPinnedSessionMeta(tab)
		if root := strings.TrimSpace(tab.WorkspaceRoot); root != "" {
			return root
		}
	}
	return "."
}

func pluginRegistryEntryView(entry pluginregistry.Entry) PluginRegistryEntryView {
	return PluginRegistryEntryView{
		Name: entry.Name, Description: entry.Description, Version: entry.Version, Author: entry.Author,
		Category: entry.Category, Source: entry.Source, Subpath: entry.Subpath, Revision: entry.Revision,
		Digest: entry.Digest, Permissions: append([]string(nil), entry.Permissions...), RegistryName: entry.RegistryName,
		RegistryMetadataURL: entry.RegistryMetadataURL, RegistryRootVersion: entry.RootVersion,
		RegistryRootDigest: entry.BootstrapRootSHA256, ProvenanceStatus: entry.ProvenanceStatus,
		RegistryEntryDigest: entry.ReleaseEvidenceSHA256, AttestationDigest: entry.AttestationSHA256,
	}
}

func pluginViewFromInstalled(home string, installed pluginpkg.InstalledPlugin) PluginView {
	view := PluginView{
		Name:                installed.Name,
		Version:             installed.Version,
		Description:         installed.Description,
		Source:              installed.Source,
		Root:                pluginpkg.ResolveRoot(home, installed.Root),
		ManifestKind:        installed.ManifestKind,
		ManifestSchema:      installed.ManifestSchema,
		InstallMode:         installed.InstallMode,
		SourceKind:          installed.SourceKind,
		SourceRevision:      installed.SourceRevision,
		TrustStatus:         installed.TrustStatus,
		RegistryName:        installed.RegistryName,
		RegistryMetadataURL: installed.RegistryMetadataURL,
		RegistryRootVersion: installed.RegistryRootVersion,
		RegistryRootDigest:  installed.RegistryRootDigest,
		RegistryEntryDigest: installed.RegistryEntryDigest,
		ProvenanceStatus:    installed.ProvenanceStatus,
		AttestationDigest:   installed.AttestationDigest,
		Digest:              installed.Digest,
		Permissions:         append([]string(nil), installed.Permissions...),
		GrantedPermissions:  append([]string(nil), installed.GrantedPermissions...),
		LifecycleSecurity:   installed.LifecycleSecurity,
		Enabled:             installed.Enabled,
	}
	if installed.Previous != nil {
		view.Rollback = &PluginRollbackView{
			Version:             installed.Previous.Version,
			Digest:              installed.Previous.Digest,
			TrustStatus:         installed.Previous.TrustStatus,
			RegistryEntryDigest: installed.Previous.RegistryEntryDigest,
			Permissions:         append([]string(nil), installed.Previous.Permissions...),
			GrantedPermissions:  append([]string(nil), installed.Previous.GrantedPermissions...),
			Enabled:             installed.Previous.Enabled,
		}
	}
	return view
}

func applyPluginPackageDetails(view *PluginView, pkg pluginpkg.Package, warnings []string) {
	view.Skills, view.Hooks, view.MCPServers = pkg.CapabilityCounts()
	view.Warnings = warnings
	inv := pkg.Inventory()
	view.SkillDetails = make([]PluginSkillView, 0, len(inv.Skills))
	for _, sk := range inv.Skills {
		view.SkillDetails = append(view.SkillDetails, PluginSkillView{
			Name:        sk.Name,
			Description: sk.Description,
			Path:        sk.Path,
			Invocation:  sk.Invocation,
			RunAs:       sk.RunAs,
		})
	}
	view.HookDetails = make([]PluginHookView, 0, len(inv.Hooks))
	for _, hook := range inv.Hooks {
		view.HookDetails = append(view.HookDetails, PluginHookView{
			Event:       hook.Event,
			Match:       hook.Match,
			Command:     hook.Command,
			ContextFile: hook.ContextFile,
			Description: hook.Description,
		})
	}
	view.MCPServerDetails = make([]PluginMCPServerView, 0, len(inv.MCPServers))
	for _, server := range inv.MCPServers {
		view.MCPServerDetails = append(view.MCPServerDetails, PluginMCPServerView{
			Name:      server.Name,
			Transport: server.Transport,
			Command:   server.Command,
			URL:       server.URL,
		})
	}
}

func (a *App) PlanPluginInstall(source string, opts PluginInstallOptions) (PluginOperationView, error) {
	opts.DryRun = true
	opts.PlanID = ""
	return a.runPluginInstallSource(source, opts, false)
}

func (a *App) InstallPlugin(source string, opts PluginInstallOptions) (PluginOperationView, error) {
	return a.applyPluginOperation(func() (PluginOperationView, error) {
		return a.runPluginInstallSource(source, opts, true)
	})
}

func (a *App) PlanPluginRemove(name string) (PluginOperationView, error) {
	return a.runPluginLifecycleOperation("uninstall", name, "", false)
}

func (a *App) RemovePlugin(name, planID string) (PluginOperationView, error) {
	return a.applyPluginOperation(func() (PluginOperationView, error) {
		return a.runPluginLifecycleOperation("uninstall", name, planID, true)
	})
}

func (a *App) SetPluginEnabled(name string, enabled bool, expectedDigest string, grantedPermissions []string) error {
	a.pluginRuntimeGate.Lock()
	defer a.pluginRuntimeGate.Unlock()
	a.runtimeRebuildMu.Lock()
	defer a.runtimeRebuildMu.Unlock()
	if err := a.ensurePluginRuntimeMutationAllowed(); err != nil {
		return err
	}
	release, err := a.beginPluginRuntimeMutation()
	if err != nil {
		return err
	}
	defer release()
	name = strings.TrimSpace(name)
	if enabled {
		err = pluginpkg.Enable(config.ReamesAgentHomeDir(), pluginpkg.EnableRequest{
			Name:               name,
			ExpectedDigest:     strings.TrimSpace(expectedDigest),
			GrantedPermissions: append([]string(nil), grantedPermissions...),
		})
	} else {
		err = pluginpkg.SetEnabled(config.ReamesAgentHomeDir(), name, false)
	}
	if err != nil {
		return err
	}
	if !enabled {
		a.supersedePluginStartupBuilds()
		revokePluginRuntimeTargets(a.snapshotPluginRuntimeTargets(), name)
	}
	a.invalidateSkillRootsCache()
	if err := a.rebuildSettingLocked("plugins"); err != nil {
		if _, ok := a.deferredRebuildWarning("plugins", err); ok {
			return nil
		}
		return err
	}
	return nil
}

func (a *App) PlanPluginUpdate(name string) (PluginOperationView, error) {
	return a.runPluginUpdate(name, "", false)
}

func (a *App) UpdatePlugin(name, planID string) (PluginOperationView, error) {
	return a.applyPluginOperation(func() (PluginOperationView, error) {
		return a.runPluginUpdate(name, planID, true)
	})
}

func (a *App) PlanPluginRollback(name string) (PluginOperationView, error) {
	return a.runPluginLifecycleOperation("rollback", name, "", false)
}

func (a *App) RollbackPlugin(name, planID string) (PluginOperationView, error) {
	return a.applyPluginOperation(func() (PluginOperationView, error) {
		return a.runPluginLifecycleOperation("rollback", name, planID, true)
	})
}

func (a *App) PluginDoctor(name string) PluginView {
	name = strings.TrimSpace(name)
	home := config.ReamesAgentHomeDir()
	verified, err := pluginpkg.VerifyInstalled(home, name)
	if err != nil {
		for _, p := range a.Plugins() {
			if p.Name == name {
				p.Error = err.Error()
				return p
			}
		}
		return PluginView{Name: name, Error: err.Error()}
	}
	view := pluginViewFromInstalled(home, verified.Installed)
	applyPluginPackageDetails(&view, verified.Package, verified.Warnings)
	return view
}

func (a *App) runPluginUpdate(name, planID string, apply bool) (PluginOperationView, error) {
	name = strings.TrimSpace(name)
	verified, err := pluginpkg.VerifyInstalled(config.ReamesAgentHomeDir(), name)
	if err != nil {
		return PluginOperationView{}, err
	}
	source := strings.TrimSpace(verified.Installed.Source)
	if source == "" {
		return PluginOperationView{}, fmt.Errorf("plugin %q has no recorded source", name)
	}
	return a.runPluginInstallSource(source, PluginInstallOptions{
		Name:    name,
		Link:    verified.Installed.InstallMode == pluginpkg.InstallModeLink,
		Replace: true,
		PlanID:  strings.TrimSpace(planID),
	}, apply)
}

func (a *App) runPluginLifecycleOperation(op, name, planID string, apply bool) (PluginOperationView, error) {
	body := map[string]any{
		"op":     op,
		"kind":   "plugin",
		"name":   strings.TrimSpace(name),
		"scope":  "global",
		"apply":  apply,
		"planId": strings.TrimSpace(planID),
	}
	return a.executePluginOperation(body)
}

func (a *App) runPluginInstallSource(source string, opts PluginInstallOptions, apply bool) (PluginOperationView, error) {
	mode := "copy"
	if opts.Link {
		mode = "link"
	}
	body := map[string]any{
		"source":  strings.TrimSpace(source),
		"kind":    "plugin",
		"mode":    mode,
		"replace": opts.Replace,
		"apply":   apply && !opts.DryRun,
	}
	if strings.TrimSpace(opts.Name) != "" {
		body["name"] = strings.TrimSpace(opts.Name)
	}
	if strings.TrimSpace(opts.PlanID) != "" {
		body["planId"] = strings.TrimSpace(opts.PlanID)
	}
	return a.executePluginOperation(body)
}

func (a *App) executePluginOperation(body map[string]any) (PluginOperationView, error) {
	raw, _ := json.Marshal(body)
	tab := a.activeTab()
	workspaceRoot := "."
	if tab != nil {
		a.reconcileTabWithPinnedSessionMeta(tab)
		if strings.TrimSpace(tab.WorkspaceRoot) != "" {
			workspaceRoot = tab.WorkspaceRoot
		}
	}
	httpClient, registryClient, registryErr := desktopPluginRegistryDependencies(workspaceRoot)
	tl := installsource.NewTool(installsource.Options{
		ProjectRoot: workspaceRoot, HTTPClient: httpClient, PluginRegistry: registryClient, PluginRegistryError: registryErr,
		OnDisconnect: func(serverName string) bool {
			if tab == nil || tab.Ctrl == nil {
				return false
			}
			removed, _ := tab.Ctrl.RemoveMCPServer(serverName)
			return removed
		},
		OnPluginDisconnect: func(pluginName, serverName string) bool {
			return disconnectPluginMCPFromTargets(a.snapshotPluginRuntimeTargets(), pluginName, serverName)
		},
		OnPluginRuntimeChange: func(pluginName string) []string {
			return revokePluginRuntimeTargets(a.snapshotPluginRuntimeTargets(), pluginName)
		},
		Approval: func([]installsource.ApprovalAction) error {
			return a.ensurePluginRuntimeMutationAllowed()
		},
	})
	out, err := tl.Execute(context.Background(), raw)
	if err != nil {
		return PluginOperationView{}, err
	}
	return decodePluginOperation(out)
}

func desktopPluginRegistryDependencies(workspaceRoot string) (*http.Client, *pluginregistry.Client, error) {
	cfg, err := config.LoadForRoot(workspaceRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load plugin registry configuration: %w", err)
	}
	httpClient, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{
		DialTimeout: 15 * time.Second, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure plugin registry network client: %w", err)
	}
	registryClient, err := pluginregistry.NewConfigured(cfg, httpClient)
	return httpClient, registryClient, err
}

func (a *App) ensurePluginRuntimeMutationAllowed() error {
	if err := a.ensureActiveTabRebuildAllowed("plugins"); err != nil {
		return err
	}
	return a.ensureLiveControllersRuntimeMutationAllowed("plugins")
}

// applyPluginOperation serializes package mutation with synchronous controller
// rebuilds. If an existing generation changes, it also invalidates startup
// builds that began before the state switch and performs a final fail-closed
// revoke against the controllers currently published by the app.
func (a *App) applyPluginOperation(run func() (PluginOperationView, error)) (PluginOperationView, error) {
	a.pluginRuntimeGate.Lock()
	defer a.pluginRuntimeGate.Unlock()
	a.runtimeRebuildMu.Lock()
	defer a.runtimeRebuildMu.Unlock()
	if err := a.ensurePluginRuntimeMutationAllowed(); err != nil {
		return PluginOperationView{}, err
	}
	release, err := a.beginPluginRuntimeMutation()
	if err != nil {
		return PluginOperationView{}, err
	}
	defer release()
	out, err := run()
	if err != nil {
		return out, err
	}
	if pluginOperationReplacesRuntime(out) {
		a.supersedePluginStartupBuilds()
		if name := pluginOperationName(out); name != "" {
			revokePluginRuntimeTargets(a.snapshotPluginRuntimeTargets(), name)
		}
	}
	if err := a.finishPluginMutationLocked(out); err != nil {
		return out, err
	}
	return out, nil
}

func (a *App) beginPluginRuntimeMutation() (func(), error) {
	targets := a.snapshotPluginRuntimeTargets()
	releases := make([]func(), 0, len(targets))
	releaseAll := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	for _, target := range targets {
		if target.ctrl == nil {
			continue
		}
		guard, ok := target.ctrl.(control.RuntimeMutationGuard)
		if !ok {
			releaseAll()
			return nil, fmt.Errorf("controller does not support safe plugin runtime mutation")
		}
		release, err := guard.BeginRuntimeMutation()
		if err != nil {
			releaseAll()
			return nil, rebuildControllerActiveWorkError("plugins")
		}
		releases = append(releases, release)
	}
	return releaseAll, nil
}

func pluginOperationReplacesRuntime(out PluginOperationView) bool {
	if !out.Applied {
		return false
	}
	for _, action := range out.Actions {
		switch action.Action {
		case "update_plugin_package", "rollback_plugin_package", "remove_plugin_package":
			return true
		}
	}
	return false
}

func pluginOperationName(out PluginOperationView) string {
	if name := strings.TrimSpace(out.Name); name != "" {
		return name
	}
	for _, action := range out.Actions {
		if name := strings.TrimSpace(action.Name); name != "" {
			return name
		}
	}
	return ""
}

// supersedePluginStartupBuilds prevents a controller assembled from the old
// plugin state from publishing after the lifecycle callback has revoked the
// controllers that were already visible. Non-active empty tabs restart against
// the new state; the active tab is rebuilt synchronously by the caller.
func (a *App) supersedePluginStartupBuilds() {
	var restart []*WorkspaceTab
	a.mu.Lock()
	activeID := a.activeTabID
	for _, tab := range a.tabs {
		if tab == nil || tab.removed || tab.buildCancel == nil {
			continue
		}
		a.supersedeTabBuildLocked(tab)
		if tab.Ctrl == nil && tab.ID != activeID {
			restart = append(restart, tab)
		}
	}
	a.mu.Unlock()
	for _, tab := range restart {
		a.startTabControllerBuild(tab)
	}
}

type pluginRuntimeTarget struct {
	ctrl control.SessionAPI
}

func (a *App) snapshotPluginRuntimeTargets() []pluginRuntimeTarget {
	var targets []pluginRuntimeTarget
	a.mu.RLock()
	collect := func(tabs map[string]*WorkspaceTab) {
		for _, tab := range tabs {
			if tab == nil || tab.Ctrl == nil {
				continue
			}
			targets = append(targets, pluginRuntimeTarget{ctrl: tab.Ctrl})
		}
	}
	collect(a.tabs)
	collect(a.detachedSessions)
	a.mu.RUnlock()
	return targets
}

func disconnectPluginMCPFromTargets(targets []pluginRuntimeTarget, pluginName, serverName string) bool {
	disconnected := false
	for _, target := range targets {
		if target.ctrl == nil {
			continue
		}
		disconnector, ok := target.ctrl.(interface {
			DisconnectPluginMCP(string, string) bool
		})
		if ok && disconnector.DisconnectPluginMCP(pluginName, serverName) {
			disconnected = true
		}
	}
	return disconnected
}

func revokePluginRuntimeTargets(targets []pluginRuntimeTarget, pluginName string) []string {
	var warnings []string
	for _, target := range targets {
		if target.ctrl == nil {
			continue
		}
		revoker, ok := target.ctrl.(interface {
			RevokePluginRuntime(string) []string
		})
		if ok {
			warnings = append(warnings, revoker.RevokePluginRuntime(pluginName)...)
		}
	}
	return warnings
}

func decodePluginOperation(raw string) (PluginOperationView, error) {
	var out PluginOperationView
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return PluginOperationView{}, fmt.Errorf("decode plugin operation: %w", err)
	}
	out.Status = strings.ToLower(strings.TrimSpace(out.Status))
	if out.Actions == nil {
		out.Actions = []PluginOperationAction{}
	}
	switch out.Status {
	case "planned":
		if !out.OK {
			return out, pluginOperationError(out)
		}
		if out.Applied || strings.TrimSpace(out.PlanID) == "" {
			return out, fmt.Errorf("plugin operation returned an invalid planned response")
		}
		return out, nil
	case "done":
		if !out.OK {
			return out, pluginOperationError(out)
		}
		if !out.Applied || strings.TrimSpace(out.PlanID) == "" {
			return out, fmt.Errorf("plugin operation returned an invalid applied response")
		}
		return out, nil
	case "partial":
		if !out.Applied || strings.TrimSpace(out.PlanID) == "" {
			return out, fmt.Errorf("plugin operation returned an invalid partial response")
		}
		return out, nil
	case "failed", "blocked", "denied":
		return out, pluginOperationError(out)
	default:
		return out, fmt.Errorf("plugin operation returned unknown status %q", out.Status)
	}
}

func pluginOperationError(out PluginOperationView) error {
	for _, action := range out.Actions {
		if strings.TrimSpace(action.Error) != "" {
			return fmt.Errorf("plugin operation %s: %s", out.Status, action.Error)
		}
	}
	if strings.TrimSpace(out.Next) != "" {
		return fmt.Errorf("plugin operation %s: %s", out.Status, out.Next)
	}
	return fmt.Errorf("plugin operation %s", out.Status)
}

// finishPluginMutationLocked refreshes the active controller after a successful
// apply. The caller holds runtimeRebuildMu so no synchronous build can publish
// an old plugin generation between revocation and this replacement.
func (a *App) finishPluginMutationLocked(out PluginOperationView) error {
	if !out.Applied {
		return nil
	}
	a.invalidateSkillRootsCache()
	if err := a.rebuildSettingLocked("plugins"); err != nil {
		if _, ok := a.deferredRebuildWarning("plugins", err); ok {
			return nil
		}
		return err
	}
	return nil
}
