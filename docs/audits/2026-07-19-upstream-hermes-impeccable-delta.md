# Hermes / Impeccable incremental review

Date: 2026-07-19

Reviewed ranges:

- Hermes: `7a43ab042f65182bb8cb00cebbd1320867d751db..34e66a0d527a762b128cebf3bd9165cd8d968c06`
- Impeccable: `8967edc988ee146823bca3c51fcf51296e9dec18..e4ab5e24bdf5321b72163d2fbcbe6fa985c848ba`

Reasonix, Codex, and Claude Code remained unchanged at their reviewed SHAs
during this check. Hermes and Impeccable are mechanism references, so this is a
code-level applicability review, not version parity.

## Hermes

Eleven non-merge commits changed fourteen files.

### Adopted: Windows PowerShell 5.1 ASCII installer ratchet

Hermes `97249cfc8` replaced non-ASCII punctuation in `install.ps1` and added a
byte-level test. Windows PowerShell 5.1 can decode a UTF-8-without-BOM script as
the legacy ANSI code page before parsing, so one typographic character can make
the installer fail before its own error handling runs.

Reames `scripts/install.ps1` already contains zero bytes above `0x7f`, but the
property was accidental. `scripts/test_installers.py` now locks it with
`test_powershell_installer_is_ascii_for_windows_powershell_51`; the existing
PowerShell dry-run tests continue to exercise parsing and argument behavior.

### Adopted: fail-safe UTF-16 credential dotenv handling

Hermes `7d597cc5d` and `90d3ba5be` expose a directly isomorphic Windows fault:
Notepad can save `.env` as UTF-16, while a naive UTF-8 repair can corrupt the
first key; UTF-32 must be detected before UTF-16 because the little-endian BOMs
overlap.

Reames also owns a global credential `.env`. Its previous `godotenv.Read` path
silently rejected UTF-16, while a later settings save could treat the raw bytes
as UTF-8 lines and rewrite damaged credentials. `internal/config` now decodes
UTF-8/UTF-8-BOM, UTF-16 LE/BE, strongly detected BOM-less UTF-16, and valid
GB18030 without mutating files on read. A successful Reames-owned credential
update atomically normalizes supported input to UTF-8. UTF-32, lossy/binary
input, embedded NULs, and truncated UTF-16 fail closed and leave the source
bytes untouched. Tests cover both UTF-16 byte orders, non-mutating reads,
preservation of an existing key across normalization, and exact UTF-32 byte
preservation after a rejected save.

### Existing equivalent coverage: env quoting, key indirection, and home paths

Hermes `4441e11c7` quotes `.env` values containing internal whitespace. Reames'
`isBareDotEnvValue` already rejects every space, tab, newline, comment, quote,
and backslash from the bare form and round-trips quoted values through
`godotenv`. Hermes `65bf42b66` resolves an auxiliary task's `key_env`; Reames
resolves every `ProviderEntry.APIKeyEnv` from the global credential store before
the shared Controller/Agent/subagent runtime is assembled, so it has no second
auxiliary plaintext-key route. Hermes `2bae4df8b` removes hardcoded profile-home
paths from its prompt. Reames path ownership already resolves through
`REAMES_AGENT_HOME`/platform config APIs, and prompt-facing guidance uses the
generic Reames Agent home rather than a competing profile path. No duplicate
mechanism is added for these three changes.

### Existing non-isomorphic protection: MCP completed-future timeout spin

Hermes `3df8bd347` and `1cec5c69d` fix a Python-specific collision where
`concurrent.futures.TimeoutError` aliases built-in `TimeoutError`: a completed
future containing an inner timeout was mistaken for a poll timeout, repeatedly
read without sleeping, grew tracebacks, and eventually OOMed.

Reames MCP is Go. `internal/plugin.Client` derives a single context deadline,
calls the transport once, and classifies `context.DeadlineExceeded`; it has no
polling `Future.result(timeout)` loop or aliased exception class. Tool-specific,
server call, parent-context, HTTP session recovery, and timeout diagnostics
already have Go tests. Copying the Python fix would add no protection.

### Tracked performance candidate: incremental streaming Markdown blocks

Hermes `bd4953b30`, `e934ee440`, and `d8b59bd60` add exact LRU plus append-only
block lexing, property comparisons against a full lex, and a setext-heading
two-block repair boundary. The change is real and code-level, not a release-note
claim.

Reames currently batches stream deltas to animation frames but `ReactMarkdown`
still parses the full growing document. That is an isomorphic long-response CPU
candidate. Hermes' implementation cannot be copied directly: it depends on
Streamdown independently rendering the returned block array, while Reames uses
one remark/rehype document with GFM, math, KaTeX, Mermaid, code components, and
document-level Markdown semantics. A Reames adoption must benchmark its actual
renderer and prove byte/AST/DOM equivalence for fences, setext, lists, tables,
HTML, reference links, math, Mermaid, non-append rewrites, session switches, and
final output. It is recorded as a Desktop performance candidate, not claimed in
this M6 recovery batch.

## Impeccable

`428b86b1` expands its design anti-pattern detector to find 3–12 px chromatic
single-edge stripes expressed as inset `box-shadow`, including Astro/style
blocks, token order, shorthand, cascade, comments, authored neutral colors, and
line reporting. `7ff8f921` corrects documentation and contribution policy.

`331540dd` adds a deliberately narrow file-scoped single-rule ignore: a
wildcard is accepted only with an explicit rule and canonical file scope;
bare wildcards, unknown flags, empty globs, and flag-shaped globs fail closed.
Its identity and status output use sorted canonical file collections.
`e4ab5e24` synchronizes the generated provider copies of that behavior.

Reames has no runtime dependency on Impeccable's Node detector. A source scan
found inset 3 px accents only on explicit active/selected/enabled UI states in
the current product, alongside border/background changes; active/current/
selected states are also deliberately exempt in the upstream detector. No
automatic CSS rewrite or new runtime is adopted. The broader structural check
remains a design-review signal for future unselected decorative stripes.
Reames does not ship Impeccable's detector, configuration schema, hook runtime,
or generated provider copies, so no same-schema code is imported. The new
change is retained as a governance signal: exemptions must be scoped to a
specific rule and canonical resource set, unknown policy flags fail closed,
and generated copies must stay synchronized with their source.

## Freeze action

After this review, the clean local mirrors were fast-forwarded and the two
SHAs explicitly accepted in `docs/upstreams/upstreams.lock.json`. Acceptance
means “reviewed and classified”, not “copied”. The generated `artifacts/` report
is temporary and must be removed after the batch is delivered.
