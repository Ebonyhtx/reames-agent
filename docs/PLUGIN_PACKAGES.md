# Reames Agent Plugin Packages

Reames Agent plugin packages bundle skills, hooks, and MCP servers behind one
installable unit.

## CLI Mode

Use `reames-agent plugin` when installing or managing plugin packages from a
terminal. Plugin packages are installed globally under the Reames Agent home
directory.

### Install From CLI

`install` accepts one source:

- A GitHub repository, such as `git:github.com/obra/superpowers` or
  `https://github.com/obra/superpowers`.
- A GitHub branch or subdirectory URL, such as
  `https://github.com/owner/repo/tree/main/path/to/plugin`.
- A local directory that contains `reames-agent-plugin.json`,
  `.codex-plugin/plugin.json`, or `.claude-plugin/plugin.json`.

Preview the install plan without writing files:

```bash
reames-agent plugin install git:github.com/obra/superpowers --dry-run
```

Install a plugin after reviewing the plan:

```bash
reames-agent plugin install git:github.com/obra/superpowers --yes --plan-id sha256:<id-from-preview>
```

Install with an explicit name or replace an installed plugin with the same name:

```bash
reames-agent plugin install git:github.com/obra/superpowers --name superpowers --replace --dry-run
reames-agent plugin install git:github.com/obra/superpowers --name superpowers --replace --yes --plan-id sha256:<id-from-preview>
```

Use a local directory in developer mode:

```bash
reames-agent plugin install /path/to/plugin --link --replace --dry-run
reames-agent plugin install /path/to/plugin --link --replace --yes --plan-id sha256:<id-from-preview>
```

CLI install flags:

- `--dry-run` plans and validates the install without writing files.
- `--yes` is required for any install that writes files.
- `--plan-id <id>` binds an apply to the digest, revision, permissions, and
  actions returned by the preview. Plugin apply refuses a missing or stale ID.
- `--replace` allows the source to replace an installed plugin with the same
  name.
- `--name <name>` or `--name=<name>` overrides the name from the plugin
  manifest for this install.
- `--link` links a local plugin directory instead of copying it into Reames Agent's
  plugin storage. Moving or deleting that directory breaks the linked plugin.

Running `reames-agent plugin install <source>` without `--dry-run` or `--yes`
refuses to write files and prints a reminder to rerun with one of those flags.
Install and remove commands print the structured JSON response from the same
install-source backend used by the desktop UI.

Installed plugin state is stored in:

```text
~/.reames-agent/plugin-packages.json
~/.reames-agent/plugins/<name>/versions/<sha256-tree-v1-id>/
```

Copy installs publish immutable, content-addressed generations. The state file
atomically selects the active generation and retains one verified predecessor
for rollback. New installs are disabled until their exact digest and requested
permissions are approved. A GitHub revision is recorded, but GitHub packages
are currently `github-https-unsigned`; HTTPS transport is not a Reames signature.

### Manage From CLI

List installed plugins:

```bash
reames-agent plugin list
```

Show one plugin's metadata, root, source, and exported capability counts:

```bash
reames-agent plugin show superpowers
```

`show` also prints the concrete capability inventory when available:

- **skills** include suggested `/<skill>` invocations and descriptions.
- **hooks** list lifecycle events, matchers, and commands or context files.
- **mcpServers** list server names, transports, and launch targets.

Verify the managed root, manifest, content digest, permission contract, and
skill roots:

```bash
reames-agent plugin doctor superpowers
```

Enable or disable a plugin without uninstalling it:

```bash
reames-agent plugin disable superpowers
reames-agent plugin enable superpowers
reames-agent plugin enable superpowers --yes
```

The first enable command prints the trust status, approved digest, and exact
permissions without enabling. Re-run with `--yes` to bind that approval. Linked
plugins must be re-planned if their bytes or permissions change.

Preview and apply an update:

```bash
reames-agent plugin update superpowers --dry-run
reames-agent plugin update superpowers --yes --plan-id sha256:<id-from-preview>
```

Permission expansion disables the updated generation until it receives a new
explicit grant. An update with permissions already covered by the previous
grant may remain enabled.

Preview and roll back to the previous verified generation:

```bash
reames-agent plugin rollback superpowers --dry-run
reames-agent plugin rollback superpowers --yes --plan-id sha256:<id-from-preview>
```

Remove a plugin:

```bash
reames-agent plugin remove superpowers --dry-run
reames-agent plugin remove superpowers --yes --plan-id sha256:<id-from-preview>
```

`remove` also accepts `uninstall` as an alias. Update, rollback, and remove use
the same preview/planId/apply contract. For linked local plugins, the external
source directory is left in place.

### Use Installed Plugins From CLI

Installed plugins do not open a separate chat surface. When a plugin is enabled,
Reames Agent loads its capabilities into normal interactive sessions:

- Run `/plugins` inside an interactive session to list installed plugin
  packages. Run `/plugins show <name>` to inspect a plugin's exported skills,
  hooks, MCP servers, and usage hints without leaving the chat.
- **Skills** appear in `/skills`. Invoke a skill with `/<skill> [args]`, or ask
  naturally and let the agent choose a matching skill by description.
- **Hooks** run automatically at their configured lifecycle events, such as
  `SessionStart`, `UserPromptSubmit`, `PreToolUse`, or `PostToolUse`.
- **MCP servers** join the normal MCP/tool flow. Ask for the task you want done;
  Reames Agent can call the plugin's tools when they are relevant.

After installing, enabling, disabling, or updating a plugin from a separate
terminal while a session is already running, start a new `reames-agent` session or
reopen `/skills` to verify the current session sees the expected skills.

## Desktop Settings

Open **Settings -> Plugins** to install and manage plugin packages without using
the CLI.

### Install Plugins

The installer has two modes:

- **Local folder**: click **Choose plugin folder** and select a plugin directory
  on disk. The selected path is shown next to the button.
- **Git repository**: enter a Git source such as
  `git:github.com/obra/superpowers`. **Install name (optional)** can override
  the plugin manifest name for this install or overwrite.

Use the action buttons after choosing the source and options:

- **Preview** validates the source and shows the planned install actions without
  writing files.
- **Install plugin** installs the selected source using the current options.
- **Refresh plugins** reloads the installed-plugin list from disk and config.

Installer options:

- **Overwrite same-name plugin** allows the current source to replace an
  installed plugin with the same name. Leave it off when duplicate-name installs
  should fail instead of replacing existing content.
- **Developer mode: link source folder** appears for **Local folder** installs.
  It links the selected directory instead of copying it into Reames Agent's plugin
  storage. Use it while developing or debugging a plugin. Moving or deleting the
  selected directory will break the linked plugin.

Preview is required before Desktop can apply an install. The apply action is
bound to the displayed `planId`; changing the source, name, link mode, or
replace option invalidates the preview and requires a new one.

### Manage Installed Plugins

The installed-plugin list shows each plugin package and its exported skills,
hooks, and MCP servers. Use **Refresh plugins** after editing plugin files or
changing config outside the app.

Expand a plugin row to manage it:

- Enable or disable the plugin.
- Review source trust, digest, requested/granted permissions, and rollback
  availability before enabling or changing generations.
- Read **How to use** for the plugin's exported skills, hooks, and MCP servers.
- **Update** pulls or refreshes an installed plugin when an update source is
  available. Update, rollback, and removal first display a version, permission,
  trust, digest, and risk summary; the confirmation applies only its `planId`.
- **Rollback** restores the previous verified generation when one is available.
- **Doctor** checks the plugin manifest and reports warnings or diagnostics.
- **Remove plugin** uninstalls the package after confirmation.

After update, rollback, removal, or disable, Desktop disconnects MCP servers
whose controller-bound package owner matches the changed plugin and removes
that plugin's hooks from every live or detached controller. A controller runtime
reservation and Desktop work-start gate prevent a new turn, shell, or session
rotation from starting after the idle check. Synchronous rebuilds are serialized
with the mutation, and startup builds that began with the old state are cancelled
before they can publish. Skill entry points in old
controllers fail closed until the controller is rebuilt or a new session is
opened, because the shared skill stores cannot safely swap a plugin generation
in place. MCP connection and ownership changes are serialized; disconnecting a
package MCP clears its ownership, so a later same-name user-authored server is
not disconnected by plugin lifecycle work.

### Use Installed Plugins From Desktop

The desktop settings page uses the same runtime model as the CLI:

- Expand an installed plugin to see its **How to use** section.
- In any desktop session, type `/plugins` to list installed plugins, or
  `/plugins show <name>` to see the same usage details from the chat surface.
- Skills are shown with suggested direct commands such as `/plan`; they are also
  discoverable from `/skills` in a session.
- Hooks and MCP servers are listed for transparency. They do not need a manual
  "run" button: enabled hooks trigger automatically, and MCP tools are available
  through ordinary tool use.
- If a currently open session does not reflect a plugin change, refresh the
  plugin list and open a new session.

## Native Manifest

Reames Agent plugins can declare `reames-agent-plugin.json` at the plugin root:

```json
{
  "schemaVersion": 1,
  "name": "example",
  "version": "1.0.0",
  "description": "Example plugin",
  "skills": ["skills"],
  "hooks": {
    "SessionStart": [
      {
        "command": "hooks/session-start",
        "description": "Load startup context"
      }
    ]
  },
  "mcpServers": {
    "helper": {
      "command": "bin/helper"
    }
  },
  "permissions": ["hooks.execute", "mcp.stdio", "skills.load"]
}
```

Relative paths are resolved inside the plugin root. Reames Agent does not run
third-party install scripts during plugin installation. Native schema v1
requires a semantic version and an exact permission set derived from the
declared capabilities. Supported permissions are `skills.load`,
`hooks.context`, `hooks.execute`, `mcp.stdio`, and `mcp.remote`. A native
manifest without `schemaVersion` remains readable as legacy compatibility, but
emits a warning and cannot be silently promoted from metadata-only state.
The historical filename `reamesAgent-plugin.json` is also read with a
deprecation warning; new packages must use `reames-agent-plugin.json`.

## Codex & Claude Compatibility

Reames Agent also reads Codex plugin manifests at `.codex-plugin/plugin.json` and
Claude Marketplace manifests at `.claude-plugin/plugin.json`. Claude plugin
capabilities Reames Agent does not map yet (`commands/`, `agents/`,
`hooks/hooks.json`, `.mcp.json`) surface as install warnings instead of being
silently dropped; multi-plugin `marketplace.json` indexes are not supported —
install each plugin directory individually. For packages such
as Superpowers and Claude-style skill packs, Reames Agent maps:

- `skills` to Reames Agent skill roots. A Claude manifest that declares no
  `skills` field falls back to the conventional `skills/` (or `.claude/skills/`)
  directory, matching Claude's own auto-discovery.
- `hooks/session-start-codex` to the Reames Agent `SessionStart` hook when present.
- A plugin-root `CLAUDE.md` file to a built-in `SessionStart` context hook. The
  file is read directly by Reames Agent, without spawning a shell command.
- `.claude/settings.json` command hooks to Reames Agent hook events when the event
  names match. Claude's `matcher` field maps to Reames Agent `match`; hook commands
  run as shell commands with the plugin root as `cwd`; Claude `timeout` values
  are interpreted as seconds.

Unsupported Claude hook item types are skipped with a warning. Reames Agent does not
run third-party install scripts or implement marketplace-specific install
protocols.

Reames Agent currently has no operated default plugin registry. Registry URLs
must be configured explicitly; do not treat an arbitrary registry index or an
unsigned GitHub repository as a trusted publisher.

Plugin hooks receive these environment variables:

- `REAMES_AGENT_PLUGIN_ROOT`
- `REAMES_AGENT_PLUGIN_NAME`
- `REAMES_AGENT_PLUGIN_VERSION`
- `REAMES_AGENT_HOME`
- `REAMES_AGENT_WORKSPACE_ROOT`
- `CLAUDE_PROJECT_DIR`

## Desktop Backend Methods

Desktop exposes plugin package operations through Wails methods:

- `Plugins`
- `PlanPluginInstall`
- `InstallPlugin`
- `PlanPluginUpdate`
- `UpdatePlugin`
- `PlanPluginRollback`
- `RollbackPlugin`
- `PlanPluginRemove`
- `RemovePlugin`
- `SetPluginEnabled`
- `PluginDoctor`
