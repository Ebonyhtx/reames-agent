# Configuration Paths

Starting with **Reames Agent v1.8.1**, Reames Agent uses one user-facing home directory
for global configuration and user-owned state. CLI and desktop share this
location.

## Reames Agent Home

| Platform | Reames Agent home |
| --- | --- |
| macOS | `~/.reames-agent` |
| Linux | `~/.reames-agent` |
| Windows | `%APPDATA%\reames-agent` |

Set `REAMES_AGENT_HOME` to override Reames Agent home for tests, CI, or portable
installations. Normal users should not need it.

When `REAMES_AGENT_HOME` is set, the runtime is fully self-contained: all
configuration, state, cache, and data live under that directory tree. Legacy
migration, OS-home convention directory scanning, and all other fallback paths
are skipped so no data leaks in from a system-wide production install.

Advanced test and portable setups may set `REAMES_AGENT_STATE_HOME` to move runtime
state such as sessions, archives, and memory. It does not move global config or
provider credentials: those remain under `REAMES_AGENT_HOME`. If an older build wrote
provider keys to `REAMES_AGENT_STATE_HOME/.env`, Reames Agent imports those keys
non-destructively when `<Reames Agent home>/.env` is missing them.

## What Lives There

| Data | Path |
| --- | --- |
| Global config | `<Reames Agent home>/config.toml` |
| Global provider credentials | `<Reames Agent home>/.env` |
| Legacy credentials import source | `<Reames Agent home>/credentials` |
| Global slash commands | `<Reames Agent home>/commands/` |
| Global skills | `<Reames Agent home>/skills/` |
| Global hooks | `<Reames Agent home>/settings.json` |
| Hook trust store | `<Reames Agent home>/trust.json` |
| MCP identity-bound reader trust | `<Reames Agent home>/mcp-security.json` |
| Sessions | `<state root>/sessions/` |
| Archives | `<state root>/archive/` |
| Memory | `<state root>/memory/` and `<state root>/projects/` |

`<state root>` defaults to `<Reames Agent home>`. It only differs when
`REAMES_AGENT_STATE_HOME` is set.

`mcp-security.json` is host-local security state, not project configuration. It
binds reader decisions to the workspace, MCP transport/identity, launcher lock,
and tool capability fingerprints. Reames Agent protects it from model-visible
file and shell reads. Do not copy it between machines as a portable allowlist.

The global user config is named `config.toml`. Project-local config files keep
the name `reames-agent.toml`. If someone says "global reames-agent.toml", they usually
mean `<Reames Agent home>/config.toml`.

## Global `config.toml`

`<Reames Agent home>/config.toml` stores non-secret configuration shared by the CLI
and desktop app. It may contain the same provider, plugin, UI, desktop, tool,
skill, sandbox, bot, and agent settings that Reames Agent renders into user config.
Provider entries store the name of the credential variable in `api_key_env`, not
the secret value.

Example:

```toml
config_version = 1
default_model = "deepseek/deepseek-v4-flash"
language = "zh"
credentials_store = "auto"   # legacy compatibility; provider keys are in .env

[ui]
theme = "auto"
cursor_shape = "underline"   # CLI/TUI text cursor: underline|block|bar

[desktop]
provider_access = ["deepseek"]

[agent]
auto_plan = "off"
max_steps = 0

[[providers]]
name        = "deepseek"
kind        = "openai"
base_url    = "https://api.deepseek.com"
models      = ["deepseek-v4-flash", "deepseek-v4-pro"]
default     = "deepseek-v4-flash"
api_key_env = "DEEPSEEK_API_KEY"

[[plugins]]
name    = "example"
command = "example-mcp-server"
```

Do not put API key values in `config.toml`. This file is regular configuration:
it is safe to inspect, edit, migrate, and include in diagnostics after standard
redaction. Secrets belong in the global `.env` below.

`[ui].cursor_shape` affects only the CLI/TUI composer. The default `underline`
avoids terminal block-cursor artifacts with double-width CJK characters; use
`block` or `bar` if you prefer those cursor shapes.

### Custom provider `api_key_env` names

When a custom provider is added from the desktop settings or `reames-agent setup`,
Reames Agent stores a generated `api_key_env` in `config.toml` and writes the secret
value to the matching key in the global `.env`. The generated name is stable, so
the same provider keeps using the same credential slot after restart.

Reames Agent derives the default from the provider name. Names that normalize to
ASCII keep readable env names such as `LOCAL_GATEWAY_API_KEY`; names made
entirely of non-ASCII characters get a stable hash suffix such as
`CUSTOM_d39b9067_API_KEY` so two Chinese provider names do not share
`CUSTOM_API_KEY`.

In the CLI custom-provider wizard, the provider name is generated from the base
URL first, then the same provider-name rule is applied. For example
`https://token.sensenova.cn/v1` creates provider name
`custom-token-sensenova-cn`, whose default key env is
`CUSTOM_TOKEN_SENSENOVA_CN_API_KEY`. Press Enter to accept that default, or type
an explicit env name such as `CUSTOM_API_KEY` if you intentionally want to share
one credential across providers.

Existing configs are not rewritten on upgrade. If an old custom provider already
uses `CUSTOM_API_KEY`, it will keep working with that key. If several old custom
providers accidentally share `CUSTOM_API_KEY`, edit each provider's
`api_key_env` to a distinct name and save the corresponding API key again.

### Custom provider endpoint URLs

Custom OpenAI-compatible providers normally store an API endpoint in `base_url`.
Reames Agent sends chat requests to `base_url + "/chat/completions"` and probes model
discovery candidates such as `/models` and `/v1/models`. If a gateway gives you a
complete chat request URL, set `chat_url`; Reames Agent will use it directly and will
not append `/chat/completions`. If model discovery needs a separate address, set
`models_url`.

If a gateway requires vendor-specific top-level request body fields, set
`extra_body`, for example `extra_body = { enable_thinking = true }`. These values
are merged into the OpenAI-compatible chat JSON request body without allowing
core fields such as `model`, `messages`, `tools`, or `stream` to be overridden.

## Global `.env`

`<Reames Agent home>/.env` is the single runtime source for provider API keys saved
by Reames Agent. The setup wizard, desktop settings, CLI missing-key prompts, and
provider-key delete actions all read or write this file through the same
credential helpers.

Structure:

```dotenv
DEEPSEEK_API_KEY=sk-...
GEMINI_API_KEY=...
ANTHROPIC_API_KEY=...
# reames-agent-cleared OLD_API_KEY
```

Rules:

- one `KEY=value` assignment per line;
- blank lines and `#` comments are ignored;
- `export KEY=value` and quoted values are accepted when reading;
- multiline values are rejected by Reames Agent writes;
- keys must use shell-style names such as `DEEPSEEK_API_KEY`;
- `# reames-agent-cleared KEY` comments are non-secret tombstones written after a key
  is deleted so legacy stores do not silently re-import it;
- Reames Agent writes this file with restricted permissions where the OS supports
  them.

For provider requests, Reames Agent resolves only this global `.env`. Project `.env`
files, home `.env` files, inherited shell environment variables, the old
`credentials` file, and the OS keyring do not act as runtime provider-key
fallbacks. Project `.env`, home `.env`, and inherited shell environment values
are not imported into the global credentials file. The old `credentials` file
and old keyring entries are read only as non-destructive migration sources when
the new global `.env` is missing a key. Project `.env` files are still read as
workspace-scoped, non-provider expansion sources for `${VAR}` references in
MCP/plugin env, headers, URLs, commands, and args; those values are not written
into the process environment, and Reames Agent control variables such as
`REAMES_AGENT_HOME`, `REAMES_AGENT_STATE_HOME`, and `XDG_CONFIG_HOME` are ignored there.

Caches remain in the OS cache directory, for example
`~/Library/Caches/reames-agent` on macOS, `$XDG_CACHE_HOME/reames-agent` or
`~/.cache/reames-agent` on Linux, and `%LOCALAPPDATA%\reames-agent\cache` on Windows.
Set `REAMES_AGENT_CACHE_HOME` to override the cache root. When `REAMES_AGENT_HOME` is
set, the cache is placed under `$REAMES_AGENT_HOME/cache` (unless
`REAMES_AGENT_CACHE_HOME` is also set, which takes precedence).

## Config Priority

Runtime configuration is resolved in this order:

```text
command-line flags
> project ./reames-agent.toml
> global <Reames Agent home>/config.toml
> compatible legacy global config
> built-in defaults
```

Writes always target the new global path:

```text
macOS/Linux: ~/.reames-agent/config.toml
Windows:     %APPDATA%\reames-agent\config.toml
```

## Legacy Migration

Starting with **v1.8.1**, Reames Agent automatically checks legacy locations on
startup before the first config load. Migration is synchronous, one-time, and
non-destructive: old files are copied or converted to Reames Agent home and left
untouched.

Legacy config sources include:

```text
~/Library/Application Support/reames-agent/config.toml
~/.config/reames-agent/config.toml
~/.reames-agent/reames-agent.toml
~/.reames-agent/config.json
```

Legacy credentials, memory files, and sessions are also imported into Reames Agent
home when the new destination does not already exist. Legacy provider keys are
copied into `<Reames Agent home>/.env` only when that file does not already contain
the same key. If the new global config already exists, it wins and legacy config
files are only kept as compatibility fallbacks.

Starting in **v1.9.1**, Reames Agent also backfills MCP servers from known legacy
paths, legacy `config.json`, desktop-registered projects, and restored tab
projects into the global `<Reames Agent home>/config.toml`. Existing global
`[[plugins]]` entries win by name, so project or legacy entries never overwrite a
server the user already configured globally. Source files are left untouched, and
the backfill writes a one-time marker so a user-deleted global MCP server is not
recreated repeatedly from an old project config.

## Manual Migration Rescue

If Reames Agent has already created the new home directory but some legacy data was
not present yet, or if the desktop app was opened before the old paths were
available, run the migration rescue command from either frontend:

```text
/migrate
```

In the CLI TUI, type `/migrate` into the chat input. In the desktop app, type the
same command into the composer. The command prints progress notices while it:

1. checks legacy config and credentials,
2. scans known legacy memory locations,
3. scans known legacy session directories,
4. imports memory files and sessions that were not previously imported, and
5. prints a final summary.

If old v0.x sessions live outside the known legacy locations — for example a
Windows v0.52 install/data directory chosen during setup — pass that directory
explicitly:

```text
/migrate --from "D:\OldReames Agent"
```

The explicit form imports sessions only. The path may be the old install
directory, a `.reames-agent`/data directory, or the `sessions` directory itself;
Reames Agent checks the common layouts below that root and uses a source-specific
marker, so a previous plain `/migrate` run does not hide the later import.

The rescue command is intentionally non-destructive. It does not overwrite an
existing `<Reames Agent home>/config.toml`; if the new config already exists, copy
any missing legacy settings across by hand. It copies legacy memory files only
when the destination file is absent. It also respects session import markers, so
sessions that were already imported and later deleted by the user will not be
restored on a later `/migrate` run.

Version limits:

- Automatic migration starts in **v1.8.1**.
- `/migrate` is available only in Go-based Reames Agent builds that include the
  command. If Reames Agent reports `unknown command`, upgrade first and rerun it.
- The command is not available in the legacy `0.x` TypeScript line.
- Plain `/migrate` rescans the legacy locations listed above. Use
  `/migrate --from <path>` only for a known v0.x session source; it is not a
  backup restore tool or a downgrade importer.
