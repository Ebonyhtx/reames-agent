package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reames-agent/internal/config"
	"reames-agent/internal/installsource"
	"reames-agent/internal/netclient"
	"reames-agent/internal/pluginpkg"
	"reames-agent/internal/pluginregistry"
)

func pluginCommand(args []string) int {
	if len(args) == 0 {
		pluginUsage()
		return 0
	}
	switch args[0] {
	case "install":
		return pluginInstallCommand(args[1:])
	case "update":
		return pluginUpdateCommand(args[1:])
	case "rollback":
		return pluginRollbackCommand(args[1:])
	case "list":
		return pluginListCommand()
	case "show":
		return pluginShowCommand(args[1:])
	case "remove", "uninstall":
		return pluginRemoveCommand(args[1:])
	case "enable":
		return pluginSetEnabledCommand(args[1:], true)
	case "disable":
		return pluginSetEnabledCommand(args[1:], false)
	case "doctor":
		return pluginDoctorCommand(args[1:])
	case "registry":
		return pluginRegistryCommand(args[1:])
	case "help", "--help", "-h":
		pluginUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown plugin command %q\n\n", args[0])
		pluginUsage()
		return 2
	}
}

func pluginUsage() {
	fmt.Fprintln(os.Stderr, `usage:
	  reames-agent plugin install <source> [--yes] [--dry-run] [--link] [--replace] [--plan-id <id>]
	  reames-agent plugin update <name> [--yes] [--dry-run] [--plan-id <id>]
	  reames-agent plugin rollback <name> [--dry-run] [--yes --plan-id <id>]
	  reames-agent plugin list
	  reames-agent plugin show <name>
	  reames-agent plugin enable <name> --yes
	  reames-agent plugin disable <name>
	  reames-agent plugin remove <name> [--dry-run] [--yes --plan-id <id>]
	  reames-agent plugin doctor <name>
	  reames-agent plugin registry search [query]
	  reames-agent plugin registry show <name>
	  reames-agent plugin registry refresh
	  reames-agent plugin registry digest <checkout> [subpath]
	  reames-agent plugin registry audit <repository> --root <root.json> [--index <target>] [--at <RFC3339>]`)
}

func pluginInstallCommand(args []string) int {
	opts, source, err := parsePluginInstallArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !opts.dryRun && !opts.yes {
		fmt.Fprintln(os.Stderr, "plugin install writes files; re-run with --yes to apply, or --dry-run to preview")
		return 2
	}
	if !opts.dryRun && opts.planID == "" {
		fmt.Fprintln(os.Stderr, "plugin install requires an approved plan; run with --dry-run, then re-run with --yes --plan-id <id>")
		return 2
	}
	mode := "copy"
	if opts.link {
		mode = "link"
	}
	body := map[string]any{
		"source":  source,
		"kind":    "plugin",
		"apply":   !opts.dryRun,
		"mode":    mode,
		"replace": opts.replace,
	}
	if strings.TrimSpace(opts.name) != "" {
		body["name"] = strings.TrimSpace(opts.name)
	}
	if strings.TrimSpace(opts.planID) != "" {
		body["planId"] = strings.TrimSpace(opts.planID)
	}
	return runInstallSourceJSON(body)
}

type parsedPluginInstallArgs struct {
	yes     bool
	dryRun  bool
	link    bool
	replace bool
	name    string
	planID  string
}

func parsePluginInstallArgs(args []string) (parsedPluginInstallArgs, string, error) {
	var opts parsedPluginInstallArgs
	var source string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--yes":
			opts.yes = true
		case arg == "--dry-run":
			opts.dryRun = true
		case arg == "--link":
			opts.link = true
		case arg == "--replace":
			opts.replace = true
		case arg == "--name":
			i++
			if i >= len(args) {
				return opts, "", fmt.Errorf("--name requires a value")
			}
			opts.name = args[i]
		case strings.HasPrefix(arg, "--name="):
			opts.name = strings.TrimPrefix(arg, "--name=")
		case arg == "--plan-id":
			i++
			if i >= len(args) {
				return opts, "", fmt.Errorf("--plan-id requires a value")
			}
			opts.planID = args[i]
		case strings.HasPrefix(arg, "--plan-id="):
			opts.planID = strings.TrimPrefix(arg, "--plan-id=")
		case strings.HasPrefix(arg, "-"):
			return opts, "", fmt.Errorf("unknown plugin install flag %q", arg)
		default:
			if source != "" {
				return opts, "", fmt.Errorf("plugin install requires exactly one source")
			}
			source = arg
		}
	}
	if source == "" {
		return opts, "", fmt.Errorf("plugin install requires exactly one source")
	}
	return opts, source, nil
}

func pluginUpdateCommand(args []string) int {
	opts, name, err := parsePluginUpdateArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !opts.dryRun && !opts.yes {
		fmt.Fprintln(os.Stderr, "plugin update writes files; re-run with --yes to apply, or --dry-run to preview")
		return 2
	}
	if !opts.dryRun && opts.planID == "" {
		fmt.Fprintln(os.Stderr, "plugin update requires an approved plan; run with --dry-run, then re-run with --yes --plan-id <id>")
		return 2
	}
	installed, ok, err := findInstalledPlugin(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "plugin %q is not installed\n", name)
		return 1
	}
	if strings.TrimSpace(installed.Source) == "" {
		fmt.Fprintf(os.Stderr, "plugin %q has no recorded update source; reinstall it from an explicit source\n", name)
		return 1
	}
	mode := installed.InstallMode
	if mode != pluginpkg.InstallModeLink {
		mode = pluginpkg.InstallModeCopy
	}
	body := map[string]any{
		"source":  installed.Source,
		"kind":    "plugin",
		"name":    installed.Name,
		"apply":   !opts.dryRun,
		"mode":    mode,
		"replace": true,
	}
	if opts.planID != "" {
		body["planId"] = opts.planID
	}
	return runInstallSourceJSON(body)
}

type parsedPluginUpdateArgs struct {
	yes    bool
	dryRun bool
	planID string
}

func parsePluginUpdateArgs(args []string) (parsedPluginUpdateArgs, string, error) {
	return parsePluginPlanArgs(args, "update")
}

func parsePluginPlanArgs(args []string, operation string) (parsedPluginUpdateArgs, string, error) {
	var opts parsedPluginUpdateArgs
	var name string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--yes":
			opts.yes = true
		case arg == "--dry-run":
			opts.dryRun = true
		case arg == "--plan-id":
			i++
			if i >= len(args) {
				return opts, "", fmt.Errorf("--plan-id requires a value")
			}
			opts.planID = args[i]
		case strings.HasPrefix(arg, "--plan-id="):
			opts.planID = strings.TrimPrefix(arg, "--plan-id=")
		case strings.HasPrefix(arg, "-"):
			return opts, "", fmt.Errorf("unknown plugin %s flag %q", operation, arg)
		default:
			if name != "" {
				return opts, "", fmt.Errorf("plugin %s requires exactly one plugin name", operation)
			}
			name = arg
		}
	}
	if name == "" {
		return opts, "", fmt.Errorf("plugin %s requires exactly one plugin name", operation)
	}
	return opts, name, nil
}

func pluginRollbackCommand(args []string) int {
	opts, name, err := parsePluginPlanArgs(args, "rollback")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !opts.dryRun && !opts.yes {
		fmt.Fprintln(os.Stderr, "plugin rollback changes the active generation; re-run with --dry-run to preview")
		return 2
	}
	if !opts.dryRun && opts.planID == "" {
		fmt.Fprintln(os.Stderr, "plugin rollback requires an approved plan; run with --dry-run, then re-run with --yes --plan-id <id>")
		return 2
	}
	body := map[string]any{
		"op": "rollback", "kind": "plugin", "name": name, "scope": "global", "apply": !opts.dryRun,
	}
	if opts.planID != "" {
		body["planId"] = opts.planID
	}
	return runInstallSourceJSON(body)
}

func pluginRemoveCommand(args []string) int {
	opts, name, err := parsePluginPlanArgs(args, "remove")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !opts.dryRun && !opts.yes {
		fmt.Fprintln(os.Stderr, "plugin remove writes files; re-run with --dry-run to preview")
		return 2
	}
	if !opts.dryRun && opts.planID == "" {
		fmt.Fprintln(os.Stderr, "plugin remove requires an approved plan; run with --dry-run, then re-run with --yes --plan-id <id>")
		return 2
	}
	body := map[string]any{
		"op": "uninstall", "kind": "plugin", "name": name, "scope": "global", "apply": !opts.dryRun,
	}
	if opts.planID != "" {
		body["planId"] = opts.planID
	}
	return runInstallSourceJSON(body)
}

func runInstallSourceJSON(body map[string]any) int {
	raw, _ := json.Marshal(body)
	httpClient, registryClient, registryErr := pluginRegistryDependencies(false)
	tl := installsource.NewTool(installsource.Options{
		HTTPClient: httpClient, PluginRegistry: registryClient, PluginRegistryError: registryErr,
	})
	out, err := tl.Execute(context.Background(), raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(out)
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !resp.OK {
		return 1
	}
	return 0
}

func pluginRegistryCommand(args []string) int {
	if len(args) == 0 {
		pluginRegistryUsage()
		return 2
	}
	switch args[0] {
	case "help", "--help", "-h":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "plugin registry help accepts no arguments")
			return 2
		}
		pluginRegistryUsage()
		return 0
	case "search":
		if len(args) > 2 {
			fmt.Fprintln(os.Stderr, "plugin registry search accepts at most one query")
			return 2
		}
	case "show":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "plugin registry show requires exactly one plugin name")
			return 2
		}
	case "refresh":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "plugin registry refresh accepts no arguments")
			return 2
		}
	case "digest":
		if len(args) < 2 || len(args) > 3 {
			fmt.Fprintln(os.Stderr, "plugin registry digest requires a checkout and optional subpath")
			return 2
		}
	case "audit":
		return pluginRegistryAuditCommand(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown plugin registry command %q\n", args[0])
		return 2
	}
	if args[0] == "digest" {
		subpath := ""
		if len(args) == 3 {
			subpath = args[2]
		}
		revision, digest, err := installsource.RegistryGitSourceEvidence(context.Background(), args[1], subpath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("revision: %s\ndigest: %s\n", revision, digest)
		return 0
	}
	_, client, err := pluginRegistryDependencies(true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch args[0] {
	case "search":
		query := ""
		if len(args) == 2 {
			query = args[1]
		}
		index, err := client.Refresh(context.Background())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		entries := pluginregistry.Search(index, query)
		if len(entries) == 0 {
			fmt.Println("no signed registry plugins matched")
			return 0
		}
		for _, entry := range entries {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", entry.Name, entry.Version, entry.Category, entry.Author, entry.Description)
		}
		return 0
	case "show":
		entry, err := client.Resolve(context.Background(), args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("name: %s\nversion: %s\ndescription: %s\nauthor: %s\ncategory: %s\nsource: %s\nsubpath: %s\nrevision: %s\ndigest: %s\npermissions: %s\nregistry: %s\nmetadataURL: %s\nbootstrapRoot: %s\nrootVersion: %d\nregistryEntryDigest: %s\nprovenance: %s\nattestationDigest: %s\n",
			entry.Name, entry.Version, entry.Description, entry.Author, entry.Category, entry.Source, entry.Subpath,
			entry.Revision, entry.Digest, strings.Join(entry.Permissions, ","), entry.RegistryName,
			entry.RegistryMetadataURL, entry.BootstrapRootSHA256, entry.RootVersion, entry.ReleaseEvidenceSHA256, entry.ProvenanceStatus, entry.AttestationSHA256)
		return 0
	case "refresh":
		index, err := client.Refresh(context.Background())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("ok: %s updated=%s plugins=%d\n", index.Registry, index.Updated.UTC().Format(time.RFC3339), len(index.Plugins))
		return 0
	}
	return 2
}

func pluginRegistryUsage() {
	fmt.Fprintln(os.Stderr, "usage: reames-agent plugin registry search [query] | show <name> | refresh | digest <checkout> [subpath] | audit <repository> --root <root.json> [--index <target>] [--at <RFC3339>]")
}

var auditPluginRegistry = pluginregistry.AuditRepository

func pluginRegistryAuditCommand(args []string) int {
	var repository, trustedRoot, indexTarget string
	var referenceTime time.Time
	var rootSet, indexSet, timeSet bool
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--root":
			if rootSet {
				fmt.Fprintln(os.Stderr, "plugin registry audit --root may be specified only once")
				return 2
			}
			rootSet = true
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "plugin registry audit --root requires a value")
				return 2
			}
			trustedRoot = args[i]
		case strings.HasPrefix(arg, "--root="):
			if rootSet {
				fmt.Fprintln(os.Stderr, "plugin registry audit --root may be specified only once")
				return 2
			}
			rootSet = true
			trustedRoot = strings.TrimPrefix(arg, "--root=")
		case arg == "--index":
			if indexSet {
				fmt.Fprintln(os.Stderr, "plugin registry audit --index may be specified only once")
				return 2
			}
			indexSet = true
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "plugin registry audit --index requires a value")
				return 2
			}
			indexTarget = args[i]
		case strings.HasPrefix(arg, "--index="):
			if indexSet {
				fmt.Fprintln(os.Stderr, "plugin registry audit --index may be specified only once")
				return 2
			}
			indexSet = true
			indexTarget = strings.TrimPrefix(arg, "--index=")
		case arg == "--at":
			if timeSet {
				fmt.Fprintln(os.Stderr, "plugin registry audit --at may be specified only once")
				return 2
			}
			timeSet = true
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "plugin registry audit --at requires an RFC3339 value")
				return 2
			}
			parsed, err := time.Parse(time.RFC3339, args[i])
			if err != nil {
				fmt.Fprintln(os.Stderr, "plugin registry audit --at requires an RFC3339 value")
				return 2
			}
			referenceTime = parsed
		case strings.HasPrefix(arg, "--at="):
			if timeSet {
				fmt.Fprintln(os.Stderr, "plugin registry audit --at may be specified only once")
				return 2
			}
			timeSet = true
			parsed, err := time.Parse(time.RFC3339, strings.TrimPrefix(arg, "--at="))
			if err != nil {
				fmt.Fprintln(os.Stderr, "plugin registry audit --at requires an RFC3339 value")
				return 2
			}
			referenceTime = parsed
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(os.Stderr, "unknown plugin registry audit flag %q\n", arg)
			return 2
		default:
			if repository != "" {
				fmt.Fprintln(os.Stderr, "plugin registry audit requires exactly one repository directory")
				return 2
			}
			repository = arg
		}
	}
	if strings.TrimSpace(repository) == "" {
		fmt.Fprintln(os.Stderr, "plugin registry audit requires exactly one repository directory")
		return 2
	}
	if strings.TrimSpace(trustedRoot) == "" {
		fmt.Fprintln(os.Stderr, "plugin registry audit requires --root from an out-of-band trusted source")
		return 2
	}
	report, err := auditPluginRegistry(context.Background(), pluginregistry.AuditOptions{
		RepositoryDir: repository, TrustedRootPath: trustedRoot, IndexTarget: indexTarget, ReferenceTime: referenceTime,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(string(body))
	return 0
}

func pluginRegistryDependencies(strict bool) (*http.Client, *pluginregistry.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		if strict {
			return nil, nil, fmt.Errorf("load plugin registry configuration: %w", err)
		}
		return nil, nil, err
	}
	httpClient, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{
		DialTimeout: 15 * time.Second, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 20 * time.Second,
	})
	if err != nil {
		if strict {
			return nil, nil, fmt.Errorf("configure plugin registry network client: %w", err)
		}
		return nil, nil, err
	}
	registryClient, err := pluginregistry.NewConfigured(cfg, httpClient)
	if err != nil && strict {
		return httpClient, nil, err
	}
	return httpClient, registryClient, err
}

func pluginListCommand() int {
	st, err := pluginpkg.LoadState(config.ReamesAgentHomeDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(st.Plugins) == 0 {
		fmt.Println("no plugins installed")
		return 0
	}
	for _, p := range st.Plugins {
		state := "disabled"
		if p.Enabled {
			state = "enabled"
		}
		version := p.Version
		if version == "" {
			version = "-"
		}
		trust := p.TrustStatus
		if trust == "" {
			trust = "legacy-unverified"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", p.Name, state, version, trust, p.Source)
	}
	return 0
}

func pluginShowCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "plugin show requires a plugin name")
		return 2
	}
	p, ok, err := findInstalledPlugin(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "plugin %q is not installed\n", args[0])
		return 1
	}
	root := pluginpkg.ResolveRoot(config.ReamesAgentHomeDir(), p.Root)
	pkg, warnings, err := pluginpkg.ParseDir(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	skills, hooks, mcp := pkg.CapabilityCounts()
	fmt.Printf("name: %s\nversion: %s\nenabled: %t\nkind: %s\nmanifestSchema: %d\ninstallMode: %s\nroot: %s\nsource: %s\nsourceKind: %s\nsourceRevision: %s\ntrust: %s\ndigest: %s\nregistryEntryDigest: %s\npermissions: %s\ngrantedPermissions: %s\nrollbackAvailable: %t\nskills: %d\nhooks: %d\nmcpServers: %d\n",
		p.Name, p.Version, p.Enabled, p.ManifestKind, p.ManifestSchema, p.InstallMode, root, p.Source, p.SourceKind, p.SourceRevision,
		p.TrustStatus, p.Digest, p.RegistryEntryDigest, strings.Join(p.Permissions, ","), strings.Join(p.GrantedPermissions, ","), p.Previous != nil, skills, hooks, mcp)
	printPluginInventory(pkg.Inventory())
	for _, warning := range warnings {
		fmt.Println("warning:", warning)
	}
	return 0
}

func printPluginInventory(inv pluginpkg.Inventory) {
	if len(inv.Skills) > 0 {
		fmt.Println("usage:")
		fmt.Println("  skills are available in interactive sessions; run /skills to browse them, or invoke a skill directly with /<name>.")
		fmt.Println("skills:")
		for _, sk := range inv.Skills {
			desc := sk.Description
			if desc == "" {
				desc = "(no description)"
			}
			invocation := sk.Invocation
			if invocation == "" {
				invocation = "/" + sk.Name
			}
			if sk.RunAs != "" {
				fmt.Printf("  %s\t%s\t%s\n", invocation, sk.RunAs, desc)
			} else {
				fmt.Printf("  %s\t%s\n", invocation, desc)
			}
		}
	}
	if len(inv.Hooks) > 0 {
		fmt.Println("hooks:")
		for _, hook := range inv.Hooks {
			target := hook.Command
			if target == "" {
				target = hook.ContextFile
			}
			match := hook.Match
			if match == "" {
				match = "*"
			}
			if hook.Description != "" {
				fmt.Printf("  %s\tmatch=%s\t%s\t%s\n", hook.Event, match, target, hook.Description)
			} else {
				fmt.Printf("  %s\tmatch=%s\t%s\n", hook.Event, match, target)
			}
		}
	}
	if len(inv.MCPServers) > 0 {
		fmt.Println("mcpServers:")
		for _, server := range inv.MCPServers {
			target := server.Command
			if target == "" {
				target = server.URL
			}
			fmt.Printf("  %s\t%s\t%s\n", server.Name, server.Transport, target)
		}
	}
}

func pluginDoctorCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "plugin doctor requires a plugin name")
		return 2
	}
	p, ok, err := findInstalledPlugin(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "plugin %q is not installed\n", args[0])
		return 1
	}
	verification, err := pluginpkg.VerifyInstalled(config.ReamesAgentHomeDir(), p.Name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid:", err)
		return 1
	}
	root := pluginpkg.ResolveRoot(config.ReamesAgentHomeDir(), verification.Installed.Root)
	pkg := verification.Package
	warnings := verification.Warnings
	for _, skillRoot := range pkg.SkillRoots() {
		if st, err := os.Stat(skillRoot); err != nil || !st.IsDir() {
			fmt.Fprintf(os.Stderr, "missing skill root: %s\n", skillRoot)
			return 1
		}
	}
	for _, warning := range warnings {
		fmt.Println("warning:", warning)
	}
	fmt.Printf("ok: %s digest=%s trust=%s (%s)\n", p.Name, verification.Installed.Digest, verification.Installed.TrustStatus, filepath.Clean(root))
	return 0
}

func pluginSetEnabledCommand(args []string, enabled bool) int {
	if len(args) < 1 || len(args) > 2 || (len(args) == 2 && args[1] != "--yes") {
		fmt.Fprintln(os.Stderr, "plugin enable/disable requires a plugin name")
		return 2
	}
	if enabled && len(args) == 1 {
		p, ok, err := findInstalledPlugin(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "plugin %q is not installed\n", args[0])
			return 1
		}
		fmt.Fprintf(os.Stderr, "plugin %s requests permissions: %s\ntrust: %s\ndigest: %s\nre-run with --yes to grant these permissions and enable it\n",
			p.Name, strings.Join(p.Permissions, ", "), p.TrustStatus, p.Digest)
		return 2
	}
	var err error
	if enabled {
		p, ok, findErr := findInstalledPlugin(args[0])
		if findErr != nil {
			err = findErr
		} else if !ok {
			err = fmt.Errorf("plugin %q is not installed", args[0])
		} else {
			err = pluginpkg.Enable(config.ReamesAgentHomeDir(), pluginpkg.EnableRequest{
				Name: p.Name, ExpectedDigest: p.Digest, GrantedPermissions: p.Permissions,
			})
		}
	} else {
		err = pluginpkg.SetEnabled(config.ReamesAgentHomeDir(), args[0], false)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("%s %s\n", map[bool]string{true: "enabled", false: "disabled"}[enabled], args[0])
	return 0
}

func findInstalledPlugin(name string) (pluginpkg.InstalledPlugin, bool, error) {
	st, err := pluginpkg.LoadState(config.ReamesAgentHomeDir())
	if err != nil {
		return pluginpkg.InstalledPlugin{}, false, err
	}
	for _, p := range st.Plugins {
		if p.Name == name {
			return p, true, nil
		}
	}
	return pluginpkg.InstalledPlugin{}, false, nil
}
