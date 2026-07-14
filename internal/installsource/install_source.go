// Package installsource: install_source.go is the tool entrypoint. It
// defines the public Options/Execute surface, the JSON Schema, and the
// end-to-end pipeline that turns a request into a plan and (optionally)
// into a series of apply calls.
package installsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"reames-agent/internal/config"
	"reames-agent/internal/pluginpkg"
	"reames-agent/internal/skill"
	"reames-agent/internal/tool"
)

// MCPConnectResult is what the ConnectMCP callback returns. Disconnect is
// optional; when non-nil, the apply step will call it to undo a connect
// whose persistence (SaveTo) failed — closing the "ghost install" window.
type MCPConnectResult struct {
	ToolCount  int
	Disconnect func() // optional; nil means rollback is not possible
}

// MCPConnector is the host-provided hook that turns a PluginEntry into a
// live MCP connection. The returned Disconnect, if any, is used by the
// install_source tool to roll back a failed persistence step.
type MCPConnector func(config.PluginEntry) (MCPConnectResult, error)

// ApprovalFunc is invoked between plan and apply when apply=true. Return
// nil to allow the install, or a non-nil error to refuse it. The action
// list reflects the exact set the apply step is about to perform; a host
// (e.g. the desktop TUI) can show it to the user and decide synchronously.
type ApprovalAction = action
type ApprovalFunc func(actions []ApprovalAction) error

// OnDisconnectFunc tells the host to remove a server from the live session and
// drop the corresponding mcp__<name>__ tools from its Registry. It returns true
// when a live server was actually removed, letting replace/rollback restore the
// old connection only when there was one.
type OnDisconnectFunc func(serverName string) bool

// OnPluginDisconnectFunc revokes a live MCP server only when the host confirms
// that the server came from the named plugin package. This prevents a package
// uninstall from disconnecting a same-name server owned by user config.
type OnPluginDisconnectFunc func(pluginName, serverName string) bool

// OnPluginRuntimeChangeFunc fail-closes non-MCP capabilities from the previous
// plugin generation in the current host. Returned warnings are included in the
// structured operation result so callers know a rebuild or new session is
// required before the verified generation becomes available.
type OnPluginRuntimeChangeFunc func(pluginName string) []string

// Options configure the install_source tool. ProjectRoot "" and HomeDir
// "" fall back to os.Getwd / os.UserHomeDir at construction time.
type Options struct {
	ProjectRoot           string
	HomeDir               string
	HTTPClient            *http.Client
	ConnectMCP            MCPConnector
	OnDisconnect          OnDisconnectFunc
	OnPluginDisconnect    OnPluginDisconnectFunc
	OnPluginRuntimeChange OnPluginRuntimeChangeFunc
	Approval              ApprovalFunc
}

type installSourceTool struct {
	root                  string
	home                  string
	reamesAgentHome       string
	httpClient            *http.Client
	connectMCP            MCPConnector
	onDisconnect          OnDisconnectFunc
	onPluginDisconnect    OnPluginDisconnectFunc
	onPluginRuntimeChange OnPluginRuntimeChangeFunc
	approval              ApprovalFunc
}

// NewTool returns a tool.Tool that callers register with the agent's
// Registry. The returned tool is safe to call from any goroutine; the
// underlying config/config.SaveTo paths do their own per-file locking.
func NewTool(opts Options) tool.Tool {
	root := opts.ProjectRoot
	if root == "" {
		if wd, err := currentDir(); err == nil {
			root = wd
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	home := opts.HomeDir
	if home == "" {
		if h, err := userHomeDir(); err == nil {
			home = h
		}
	}
	reamesAgentHome := ""
	if opts.HomeDir != "" {
		reamesAgentHome = filepath.Join(home, ".reames-agent")
	} else if dir := config.ReamesAgentHomeDir(); dir != "" {
		reamesAgentHome = dir
	} else if home != "" {
		reamesAgentHome = filepath.Join(home, ".reames-agent")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	// install_source fetches untrusted URLs (SKILL.md, .mcp.json, GitHub
	// manifests); guard the dial against SSRF the same way web_fetch does, so a
	// prompt-injected source can't reach cloud metadata / internal services.
	client = ssrfGuardClient(client)
	return &installSourceTool{
		root:                  root,
		home:                  home,
		reamesAgentHome:       reamesAgentHome,
		httpClient:            client,
		connectMCP:            opts.ConnectMCP,
		onDisconnect:          opts.OnDisconnect,
		onPluginDisconnect:    opts.OnPluginDisconnect,
		onPluginRuntimeChange: opts.OnPluginRuntimeChange,
		approval:              opts.Approval,
	}
}

func (*installSourceTool) Name() string   { return "install_source" }
func (*installSourceTool) ReadOnly() bool { return false }

func (*installSourceTool) Description() string {
	return "Plan, install, uninstall, or roll back a Reames Agent skill, MCP server, or plugin package from a URL, local file/folder, .mcp.json, executable, or package name. Plugin install/update/rollback/uninstall operations return a deterministic plan with per-action risk and require the same planId when applied. op='uninstall' removes an installed item; op='rollback' restores a plugin's previous verified generation."
}

func (*installSourceTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
	  "op":{"type":"string","enum":["install","uninstall","rollback"],"description":"Whether to install (default), uninstall, or roll back a plugin to its previous verified generation."},
	  "source":{"type":"string","description":"URL, local file/folder path, .mcp.json path, or package name to install from. Ignored for uninstall and rollback (use name instead)."},
  "kind":{"type":"string","enum":["auto","skill","mcp","plugin"],"description":"Capability kind. Defaults to auto."},
	  "apply":{"type":"boolean","description":"false (default) returns a deterministic plan; true performs the plan and requires its matching planId for plugin install, update, rollback, and uninstall."},
  "scope":{"type":"string","enum":["project","global"],"description":"Where to persist config or copy skills. MCP installs default to global so every project can use them; project-root .mcp.json imports default to project; skills default to project when a workspace exists, otherwise global."},
  "mode":{"type":"string","enum":["auto","copy","link","register"],"description":"Skill install mode. auto registers multi-skill roots and copies single skills into the canonical <skill-name>/SKILL.md layout; copy copies skill files/folders; link creates symlinks; register adds a skill root to [skills].paths."},
	  "name":{"type":"string","description":"Optional install name override. Required for uninstall and rollback."},
  "transport":{"type":"string","enum":["auto","stdio","http","sse"],"description":"MCP transport override. URL sources default to http unless --sse-like; package sources default to stdio."},
  "command":{"type":"string","description":"Optional stdio MCP command override for package/local executable installs."},
  "args":{"type":"array","items":{"type":"string"},"description":"Optional stdio MCP args override."},
  "env":{"type":"object","additionalProperties":{"type":"string"},"description":"Environment variables for stdio MCP servers."},
  "headers":{"type":"object","additionalProperties":{"type":"string"},"description":"HTTP headers for remote MCP servers. Prefer ${VAR} placeholders for secrets."},
  "tier":{"type":"string","enum":["background","eager"],"description":"Persisted MCP startup tier. Defaults to background."},
  "replace":{"type":"boolean","description":"Allow replacing an existing MCP config entry with the same name. Skills still refuse to overwrite existing files."},
  "strict":{"type":"boolean","description":"Skill install strictness. true (default) requires name+description frontmatter; false copies the file as-is (use only for files you trust)."},
  "planId":{"type":"string","description":"Optional. Echoed from a previous planned response to confirm the host is approving the same plan."}
},
"required":[]
}`)
}

// Execute parses args, plans, and (if apply=true and Approval allows)
// performs the writes. JSON output is always returned on success even when
// the plan is empty, so the model can read structured `next` hints.
func (t *installSourceTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return "", fmt.Errorf("install_source: invalid args: %w", err)
	}
	req.Source = strings.TrimSpace(req.Source)
	if req.Op == "" {
		req.Op = "install"
	}
	if req.Op != "install" && req.Op != "uninstall" && req.Op != "rollback" {
		return "", fmt.Errorf("install_source: op %q is not supported (want install|uninstall|rollback)", req.Op)
	}
	if req.Op == "install" && req.Source == "" {
		return "", errors.New("install_source requires a non-empty source")
	}
	if (req.Op == "uninstall" || req.Op == "rollback") && strings.TrimSpace(req.Name) == "" {
		return "", fmt.Errorf("install_source: op=%s requires a non-empty name", req.Op)
	}
	req.Kind = normalizeKind(req.Kind)
	req.Scope, req.scopeExplicit = t.normalizeScope(req.Scope)
	req.Mode = normalizeMode(req.Mode)
	req.Transport = normalizeTransport(req.Transport)
	if norm, ok := normalizeTier(req.Tier); ok {
		req.Tier = norm
	}

	if req.Op == "uninstall" {
		return t.executeUninstall(ctx, req)
	}
	if req.Op == "rollback" {
		return t.executeRollback(ctx, req)
	}

	actions, warnings, err := t.plan(ctx, req)
	if err != nil {
		return "", err
	}
	planID := computePlanID(req, actions)
	if len(actions) == 0 {
		out := response{
			OK:       false,
			Status:   "blocked",
			Op:       req.Op,
			Applied:  false,
			Source:   req.Source,
			Kind:     "",
			Scope:    req.Scope,
			Mode:     req.Mode,
			PlanID:   planID,
			Warnings: warnings,
			Next:     "No installable Reames Agent skill, MCP server, or plugin package was detected. Ask the user for a direct SKILL.md, skill root, .mcp.json, plugin manifest, MCP endpoint, or package name.",
		}
		return marshalJSON(out), nil
	}

	if !req.Apply {
		for i := range actions {
			actions[i].Status = "planned"
		}
		scope := commonActionScope(actions)
		out := response{
			OK:       true,
			Status:   "planned",
			Op:       req.Op,
			Applied:  false,
			Source:   req.Source,
			Kind:     summarizeKind(actions),
			Kinds:    kindCounts(actions),
			Scope:    scope,
			Mode:     req.Mode,
			PlanID:   planID,
			Actions:  publicActions(actions),
			Warnings: warnings,
			Next:     "Review the plan (especially each action's riskLevel). Call install_source again with apply=true and the same planId to install.",
		}
		return marshalJSON(out), nil
	}

	if containsPluginAction(actions) && req.PlanID == "" {
		return "", newErr(ErrApprovalDenied, "plugin apply requires the planId returned by a prior preview")
	}
	if req.PlanID != "" && req.PlanID != planID {
		return "", newErr(ErrApprovalDenied, "planId mismatch (got %s, expected %s); re-plan and re-approve", req.PlanID, planID)
	}
	if t.approval != nil {
		if err := t.approval(publicActions(actions)); err != nil {
			return marshalJSON(response{
				OK:       false,
				Status:   "denied",
				Op:       req.Op,
				Applied:  false,
				Source:   req.Source,
				Kind:     summarizeKind(actions),
				Kinds:    kindCounts(actions),
				Scope:    req.Scope,
				Mode:     req.Mode,
				PlanID:   planID,
				Actions:  publicActions(actions),
				Warnings: append(warnings, "host approval was denied: "+err.Error()),
				Next:     "Ask the user to confirm, or run with a less risky plan (e.g. lower scope, fewer actions).",
			}), nil
		}
	}

	return t.executeApply(ctx, req, actions, warnings, planID), nil
}

// executeApply runs the apply phase. The first failed action short-circuits
// the rest only when a single failure implies the plan is unusable; for
// MCP installs in particular, partial completion is reported honestly.
func (t *installSourceTool) executeApply(ctx context.Context, req request, actions []action, warnings []string, planID string) string {
	ok := true
	anySucceeded := false
	for i := range actions {
		if err := t.apply(ctx, req, &actions[i]); err != nil {
			ok = false
			actions[i].Status = "failed"
			actions[i].Error = err.Error()
			if actions[i].Next == "" {
				actions[i].Next = nextForError(err)
			}
			continue
		}
		actions[i].Status = "done"
		anySucceeded = true
		warnings = append(warnings, actions[i].Warnings...)
	}
	status := "done"
	next := "Installed and verified."
	if !ok {
		if anySucceeded {
			status = "partial"
			next = "Some actions succeeded; the failed ones are listed in actions[].status=failed. Re-plan those and retry."
		} else {
			status = "failed"
			next = "No action succeeded. Fix the first failed action[] entry and retry install_source with apply=true."
		}
	}
	return marshalJSON(response{
		OK:       ok,
		Status:   status,
		Op:       req.Op,
		Applied:  true,
		Source:   req.Source,
		Kind:     summarizeKind(actions),
		Kinds:    kindCounts(actions),
		Scope:    commonActionScope(actions),
		Mode:     req.Mode,
		PlanID:   planID,
		Actions:  publicActions(actions),
		Warnings: warnings,
		Next:     next,
	})
}

// executeUninstall handles op=uninstall. It locates the named entry in the
// active config (skills via the on-disk layout, MCP via cfg.Plugins), requires
// the exact preview planId, consults the host approval hook, and then asks the
// host to disconnect the approved runtime entry.
func (t *installSourceTool) executeUninstall(ctx context.Context, req request) (string, error) {
	actions := []action{}
	scopes := t.uninstallSearchScopes(req)
	for _, scope := range scopes {
		actions = t.uninstallActionsForScope(req.Name, scope, req.Kind)
		if len(actions) > 0 {
			break
		}
	}

	scope := commonActionScope(actions)
	if len(actions) == 0 {
		if len(scopes) == 1 {
			scope = scopes[0]
		} else {
			scope = strings.Join(scopes, "/")
		}
		return marshalJSON(response{
			OK:      false,
			Status:  "blocked",
			Op:      req.Op,
			Applied: false,
			Source:  req.Source,
			Name:    req.Name,
			Scope:   scope,
			Next:    "No installed skill or MCP server matched that name in the chosen scope.",
		}), nil
	}
	planID := computePlanID(req, actions)
	if !req.Apply {
		for i := range actions {
			actions[i].Status = "planned"
		}
		return marshalJSON(response{
			OK: true, Status: "planned", Op: req.Op, Applied: false, Source: req.Source, Name: req.Name,
			Kind: summarizeKind(actions), Kinds: kindCounts(actions), Scope: scope, PlanID: planID,
			Actions: publicActions(actions), Next: "Review the removal plan, then call again with apply=true and the same planId.",
		}), nil
	}
	if req.PlanID == "" {
		return "", newErr(ErrApprovalDenied, "uninstall requires the planId returned by a prior preview")
	}
	if req.PlanID != planID {
		return "", newErr(ErrApprovalDenied, "planId mismatch (got %s, expected %s); re-plan and re-approve", req.PlanID, planID)
	}
	if t.approval != nil {
		if err := t.approval(publicActions(actions)); err != nil {
			return marshalJSON(response{
				OK: false, Status: "denied", Op: req.Op, Applied: false, Source: req.Source, Name: req.Name,
				Kind: summarizeKind(actions), Kinds: kindCounts(actions), Scope: scope, PlanID: planID,
				Actions: publicActions(actions), Warnings: []string{"host approval was denied: " + err.Error()},
				Next: "Keep the installed item or review a different removal plan.",
			}), nil
		}
	}

	// Each action is independent after the exact removal plan is approved.
	ok := true
	anySucceeded := false
	var warnings []string
	for i := range actions {
		if err := t.apply(ctx, req, &actions[i]); err != nil {
			ok = false
			actions[i].Status = "failed"
			actions[i].Error = err.Error()
			actions[i].Next = "Inspect the error, then retry op=uninstall."
			continue
		}
		actions[i].Status = "done"
		anySucceeded = true
		warnings = append(warnings, actions[i].Warnings...)
	}
	status := "done"
	if !ok {
		status = "partial"
		if !anySucceeded {
			status = "failed"
		}
	}
	return marshalJSON(response{
		OK:       ok,
		Status:   status,
		Op:       req.Op,
		Applied:  true,
		Source:   req.Source,
		Name:     req.Name,
		Kind:     summarizeKind(actions),
		Kinds:    kindCounts(actions),
		Scope:    scope,
		PlanID:   planID,
		Actions:  publicActions(actions),
		Warnings: warnings,
		Next:     "Removed.",
	}), nil
}

func (t *installSourceTool) executeRollback(ctx context.Context, req request) (string, error) {
	installed, ok, err := pluginpkg.FindInstalled(t.reamesAgentHome, req.Name)
	if err != nil {
		return marshalJSON(response{
			OK: false, Status: "failed", Op: req.Op, Applied: false, Name: req.Name,
			Kind: "plugin", Scope: "global", Next: "Plugin state could not be read: " + err.Error(),
		}), nil
	}
	if !ok || installed.Previous == nil {
		return marshalJSON(response{
			OK: false, Status: "blocked", Op: req.Op, Applied: false, Name: req.Name,
			Kind: "plugin", Scope: "global", Next: "The plugin is not installed or has no verified rollback generation.",
		}), nil
	}
	previous := installed.Previous
	added, removed := permissionDiff(installed.Permissions, previous.Permissions)
	act := action{
		Kind:               "plugin",
		Action:             "rollback_plugin_package",
		Status:             "planned",
		RiskLevel:          RiskMedium,
		RiskReasons:        []string{"atomically switches the active plugin to its previous verified generation"},
		Name:               installed.Name,
		Target:             pluginpkg.ResolveRoot(t.reamesAgentHome, previous.Root),
		Scope:              "global",
		ConfigPath:         pluginpkg.StatePath(t.reamesAgentHome),
		ManifestKind:       previous.ManifestKind,
		Version:            previous.Version,
		CurrentVersion:     installed.Version,
		Digest:             previous.Digest,
		CurrentDigest:      installed.Digest,
		CurrentStateToken:  pluginpkg.InstalledStateToken(installed),
		Permissions:        append([]string(nil), previous.Permissions...),
		AddedPermissions:   added,
		RemovedPermissions: removed,
		SourceKind:         previous.SourceKind,
		SourceRevision:     previous.SourceRevision,
		TrustStatus:        previous.TrustStatus,
		WillEnable:         previous.Enabled && permissionSetCovers(previous.GrantedPermissions, previous.Permissions),
		RollbackAvailable:  true,
	}
	if len(added) > 0 {
		act.RiskLevel = RiskHigh
		act.RiskReasons = append(act.RiskReasons, "restored generation requests permissions not used by the current generation: "+strings.Join(added, ", "))
	}
	planID := computePlanID(req, []action{act})
	if !req.Apply {
		return marshalJSON(response{
			OK: true, Status: "planned", Op: req.Op, Applied: false, Name: req.Name,
			Kind: "plugin", Kinds: kindTally{Plugin: 1}, Scope: "global", PlanID: planID,
			Actions: []action{act}, Next: "Review the rollback target and permission differences, then call again with apply=true and the same planId.",
		}), nil
	}
	if req.PlanID == "" {
		return "", newErr(ErrApprovalDenied, "plugin rollback requires the planId returned by a prior preview")
	}
	if req.PlanID != planID {
		return "", newErr(ErrApprovalDenied, "planId mismatch (got %s, expected %s); re-plan and re-approve", req.PlanID, planID)
	}
	if t.approval != nil {
		if err := t.approval([]ApprovalAction{act}); err != nil {
			act.Status = "denied"
			return marshalJSON(response{
				OK: false, Status: "denied", Op: req.Op, Applied: false, Name: req.Name,
				Kind: "plugin", Kinds: kindTally{Plugin: 1}, Scope: "global", PlanID: planID,
				Actions: []action{act}, Warnings: []string{"host approval was denied: " + err.Error()},
				Next: "Keep the current generation active or review a different rollback plan.",
			}), nil
		}
	}
	if err := t.apply(ctx, req, &act); err != nil {
		act.Status = "failed"
		act.Error = err.Error()
		act.Next = "Inspect the rollback verification error; the current generation remains active."
		return marshalJSON(response{
			OK: false, Status: "failed", Op: req.Op, Applied: false, Name: req.Name,
			Kind: "plugin", Kinds: kindTally{Plugin: 1}, Scope: "global", PlanID: planID,
			Actions: []action{act}, Warnings: act.Warnings, Next: act.Next,
		}), nil
	}
	act.Status = "done"
	return marshalJSON(response{
		OK: true, Status: "done", Op: req.Op, Applied: true, Name: req.Name,
		Kind: "plugin", Kinds: kindTally{Plugin: 1}, Scope: "global", PlanID: planID,
		Actions: []action{act}, Warnings: act.Warnings, Next: "Rolled back to the previous verified plugin generation.",
	}), nil
}

func containsPluginAction(actions []action) bool {
	for _, act := range actions {
		if act.Kind == "plugin" {
			return true
		}
	}
	return false
}

func (t *installSourceTool) uninstallSearchScopes(req request) []string {
	if req.scopeExplicit && req.Scope != "" {
		return []string{req.Scope}
	}
	scopes := []string{}
	if strings.TrimSpace(t.root) != "" {
		scopes = append(scopes, "project")
	}
	return append(scopes, "global")
}

func (t *installSourceTool) uninstallActionsForScope(name, scope, kind string) []action {
	var actions []action
	cfgPath := t.configPath(scope)
	cfg := config.LoadForEdit(cfgPath)

	// Skills: try the flat file, then the directory layout, in the chosen
	// scope. We don't require a kind - "name" disambiguates.
	if kind == "auto" || kind == "skill" {
		if path, ok := t.resolveSkillPath(name, scope); ok {
			actions = append(actions, action{
				Kind:       "skill",
				Action:     "remove_skill",
				Name:       name,
				Target:     path,
				Scope:      scope,
				ConfigPath: cfgPath,
				RiskLevel:  RiskLow,
			})
		} else if rootAction, ok := t.resolveRegisteredSkillRoot(name, scope, cfgPath, cfg); ok {
			actions = append(actions, rootAction)
		}
	}

	// MCP: scan the chosen config for the named plugin.
	if kind == "auto" || kind == "mcp" {
		for _, p := range cfg.Plugins {
			if p.Name == name {
				actions = append(actions, action{
					Kind:       "mcp",
					Action:     "remove_mcp_server",
					Name:       p.Name,
					Target:     p.URL,
					Scope:      scope,
					Transport:  pluginTransport(p),
					ConfigPath: cfgPath,
					RiskLevel:  RiskMedium,
					RiskReasons: []string{
						"disconnects a running server and drops its tools from the active session",
					},
				})
				break
			}
		}
	}
	if (kind == "auto" || kind == "plugin") && (scope == "global" || scope == "") {
		if st, err := pluginpkg.LoadState(t.reamesAgentHome); err == nil {
			for _, p := range st.Plugins {
				if p.Name != name {
					continue
				}
				root := pluginpkg.ResolveRoot(t.reamesAgentHome, p.Root)
				actions = append(actions, action{
					Kind:              "plugin",
					Action:            "remove_plugin_package",
					Name:              p.Name,
					Target:            root,
					Scope:             "global",
					ConfigPath:        pluginpkg.StatePath(t.reamesAgentHome),
					ManifestKind:      p.ManifestKind,
					Version:           p.Version,
					Digest:            p.Digest,
					Permissions:       append([]string(nil), p.Permissions...),
					CurrentStateToken: pluginpkg.InstalledStateToken(p),
					RiskLevel:         RiskMedium,
					RiskReasons: []string{
						"removes a plugin package and disables its skills, hooks, and MCP servers",
					},
				})
				break
			}
		}
	}
	return actions
}

// resolveSkillPath finds the on-disk location of a previously installed
// skill of the given name in the chosen scope. The bool reports whether
// the path is a real install (Lstat succeeded). Both flat (<name>.md) and
// directory (<name>/) layouts are checked.
func (t *installSourceTool) resolveSkillPath(name, scope string) (string, bool) {
	if !config.IsValidSkillName(name) {
		return "", false
	}
	var root string
	if scope == "global" {
		if t.reamesAgentHome == "" {
			return "", false
		}
		root = filepath.Join(t.reamesAgentHome, skill.SkillsDirname)
	} else {
		root = filepath.Join(t.root, ".reames-agent", skill.SkillsDirname)
	}
	flat := filepath.Join(root, name+".md")
	if _, err := lstat(flat); err == nil {
		return flat, true
	}
	dir := filepath.Join(root, name)
	if _, err := lstat(filepath.Join(dir, skill.SkillFile)); err == nil {
		return dir, true
	}
	return "", false
}

func (t *installSourceTool) resolveRegisteredSkillRoot(name, scope, cfgPath string, cfg *config.Config) (action, bool) {
	if !config.IsValidSkillName(name) {
		return action{}, false
	}
	for _, rawPath := range cfg.Skills.Paths {
		path := t.resolvePath(config.ExpandVars(rawPath))
		cands, err := scanSkillRoot(path, false)
		if err != nil {
			continue
		}
		var names []string
		found := false
		for _, cand := range cands {
			names = append(names, cand.Name)
			if cand.Name == name {
				found = true
			}
		}
		if !found {
			continue
		}
		sort.Strings(names)
		return action{
			Kind:       "skill",
			Action:     "remove_skill_root",
			Name:       name,
			Target:     rawPath,
			Scope:      scope,
			ConfigPath: cfgPath,
			Skills:     names,
			SkillCount: len(names),
			RiskLevel:  RiskMedium,
			RiskReasons: []string{
				"removes a registered skill root from [skills].paths and may hide every skill in that folder",
			},
		}, true
	}
	return action{}, false
}

func (t *installSourceTool) configPath(scope string) string {
	if scope == "global" {
		if p := config.UserConfigPath(); p != "" {
			return p
		}
	}
	return filepath.Join(t.root, "reames-agent.toml")
}

func (t *installSourceTool) normalizeScope(scope string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "project":
		return "project", true
	case "global":
		return "global", true
	default:
		return "", false
	}
}

func (t *installSourceTool) installScope(req request, kind, source string) string {
	if req.scopeExplicit && req.Scope != "" {
		return req.Scope
	}
	if kind == "mcp" {
		if t.isProjectMCPJSONSource(source) {
			return "project"
		}
		return "global"
	}
	if strings.TrimSpace(t.root) != "" {
		return "project"
	}
	return "global"
}

func (t *installSourceTool) isProjectMCPJSONSource(source string) bool {
	if isURL(source) || !strings.EqualFold(filepath.Base(source), ".mcp.json") {
		return false
	}
	root := strings.TrimSpace(t.root)
	if root == "" {
		return false
	}
	sourceAbs, sourceErr := filepath.Abs(source)
	rootAbs, rootErr := filepath.Abs(root)
	if sourceErr != nil || rootErr != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(rootAbs), filepath.Clean(sourceAbs))
	if err != nil {
		return false
	}
	return rel == ".mcp.json" || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func commonActionScope(actions []action) string {
	if len(actions) == 0 {
		return ""
	}
	scope := actions[0].Scope
	for _, action := range actions[1:] {
		if action.Scope != scope {
			return "mixed"
		}
	}
	return scope
}

func (t *installSourceTool) resolvePath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		p = filepath.Join(t.home, p[2:])
	} else if p == "~" {
		p = t.home
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(t.root, p)
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// computePlanID hashes the request plus the full public action set so a later
// apply call with the same planId can be verified to be approving exactly the
// same plan. It intentionally excludes Apply and PlanID; everything that changes
// what will be written/connected must live either in req's planning fields or in
// the action DTO.
func computePlanID(req request, actions []action) string {
	public := publicActions(actions)
	sort.Slice(public, func(i, j int) bool {
		return actionPlanKey(public[i]) < actionPlanKey(public[j])
	})
	payload := struct {
		Op        string            `json:"op"`
		Source    string            `json:"source"`
		Kind      string            `json:"kind"`
		Scope     string            `json:"scope"`
		Mode      string            `json:"mode"`
		Name      string            `json:"name"`
		Transport string            `json:"transport"`
		Command   string            `json:"command"`
		Args      []string          `json:"args,omitempty"`
		Env       map[string]string `json:"env,omitempty"`
		Headers   map[string]string `json:"headers,omitempty"`
		Tier      string            `json:"tier"`
		Replace   bool              `json:"replace"`
		Strict    bool              `json:"strict"`
		Actions   []action          `json:"actions"`
	}{
		Op:        req.Op,
		Source:    req.Source,
		Kind:      req.Kind,
		Scope:     commonActionScope(actions),
		Mode:      req.Mode,
		Name:      req.Name,
		Transport: req.Transport,
		Command:   req.Command,
		Args:      req.Args,
		Env:       req.Env,
		Headers:   req.Headers,
		Tier:      req.Tier,
		Replace:   req.Replace,
		Strict:    req.strict(),
		Actions:   public,
	}
	body, _ := json.Marshal(payload)
	h := sha256.New()
	h.Write(body)
	return "sha256:" + hex.EncodeToString(h.Sum(nil)[:16])
}

// kindCounts tallies the per-kind action count for the response. Skill
// skills and MCP servers in the same plan get separate counts so the
// caller can summarize accurately.
func kindCounts(actions []action) kindTally {
	var out kindTally
	for _, a := range actions {
		switch a.Kind {
		case "skill":
			out.Skill++
		case "mcp":
			out.MCP++
		case "plugin":
			out.Plugin++
		}
	}
	return out
}

// nextForError maps a sentinel error to a short remediation hint. Callers
// use it as the default `next` value when a plan step fails.
func nextForError(err error) string {
	switch {
	case errors.Is(err, ErrAuthRequired):
		return "Authentication is required. Add the needed token as an environment variable or header placeholder, then retry."
	case errors.Is(err, ErrBinaryMissing):
		return "Install the missing local runtime or use an absolute command path, then retry."
	case errors.Is(err, ErrAlreadyExists):
		return "Choose another name, remove the existing entry, or retry MCP installs with replace=true."
	case errors.Is(err, ErrUnsafeLinkTarget):
		return "The link target escapes the project/home root. Pick a source path inside the workspace or home directory."
	case errors.Is(err, ErrApprovalDenied):
		return "Host denied the install. Re-run without apply=true to revise the plan, or ask the user to confirm."
	case errors.Is(err, ErrManifestMissing):
		return "No installable manifest was found at the source. Provide a direct SKILL.md, .mcp.json, executable, or package name."
	case errors.Is(err, ErrInvalidManifest):
		return "The manifest was found but did not validate. Check required fields (command/url/tier)."
	case errors.Is(err, ErrSourceUnreadable):
		return "The source could not be read. Check the URL/path and try again."
	default:
		return "Inspect the error, fix the source or environment, then retry."
	}
}

// currentDir / userHomeDir / lstat are tiny wrappers that exist so tests
// can stub them; the wrappers today just call the stdlib versions.
var (
	currentDir  = defaultCurrentDir
	userHomeDir = defaultUserHomeDir
	lstat       = defaultLstat
)

func defaultCurrentDir() (string, error)            { return os.Getwd() }
func defaultUserHomeDir() (string, error)           { return os.UserHomeDir() }
func defaultLstat(path string) (os.FileInfo, error) { return os.Lstat(path) }
