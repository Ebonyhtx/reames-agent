# Gateway command contract audit

> Date: 2026-07-10  
> Scope: platform-independent IM gateway command parsing and mobile status alias

## Conclusion

The Gateway now has a small platform-independent slash command parser and a
Hermes-like `/current` alias for `/status`.

This keeps IM commands such as `/stop`, `/new`, `/current`, `/approve`, and
`/deny` as gateway-layer controls instead of model prompts, while avoiding
prefix-only false positives such as `/statusx` or `/stopwatch`.

## Evidence

Code and tests:

- `internal/bot/session.go`
  - adds `ParseSlashCommand`;
  - recognizes only fixed command verbs at the beginning of the message;
  - keeps leading-whitespace text as a normal user message;
  - adds `/current` to queue-bypass commands.
- `internal/bot/gateway.go`
  - dispatches slash commands by parsed verb rather than string prefix;
  - routes `/current` to the same status response as `/status`;
  - advertises `/status 或 /current` in `/help`.
- `internal/bot/session_test.go`
  - covers `/current`;
  - prevents `/statusx` and `/stopwatch` from being treated as control commands;
  - covers normalized uppercase parsing and leading-whitespace non-command text.
- `internal/bot/gateway_test.go`
  - verifies `/current` returns the same status categories users expect from `/status`;
  - verifies `/help` mentions the alias.
- `docs/BOT_GUIDE.md` and `docs/BOT_GUIDE.zh-CN.md`
  - document `/current` as a status alias.

## Boundary

This does not add a new social platform and does not change the Agent runtime.
It is a gateway command contract hardening step that makes future Feishu,
WeChat, QQ, Telegram, or similar adapters share the same command semantics.

Still missing for a full cloud/server proof:

- real Feishu/Lark round trip against an actual app;
- Linux server install + `gateway run` service smoke on a clean machine;
- durable remote approval pairing for high-risk operations;
- persisted gateway health/feedback aggregation.
