# Codex / Claude / Hermes / Grok Build 最新增量审计

日期：2026-07-20

## 治理前提

- DeepSeek Reasonix 是一级主源码上游；
- OpenAI Codex 与 Claude Code 是二级战略代码上游，分别承担 GPT/OpenAI 与 Claude/Anthropic 原生协议和
  代码级能力跟踪；
- Hermes、Grok Build 及其他 `F:\code-reference` 仓库仅提供机制、体验和测试信号。

“二级战略”要求读取真实提交和代码路径，但不表示 vendor Rust/TypeScript runtime。三级参考的版本变化不会
自动产生 parity 义务。

## OpenAI Codex `0fb559f0..3e2f7972`

6 个非 merge 提交，11 个 TUI 文件，`+341/-102`：

- transcript rendering 避免 clone thread data；
- Markdown collector 成为 streaming 单一事实源；
- side conversation 启动时不 replay inherited turns，并修复 liveness selection race；
- buffered history line 改为借用；
- MCP image history cell 只验证解码，不长期持有 decoded bitmap。

代码级结论：

- Reames subagent 从独立 transcript 和显式 workspace/context envelope 启动，不复制 App Server side-thread 的
  inherited-turn replay；这是 `existing-equivalent`，仍保留 Codex 用例作为回归信号。
- Reames Desktop history 已把大 tool result 从前端 items 中剥离，通过 `ToolResultForTab` 按展开请求；Go
  history DTO 不解码并持有 MCP bitmap。`3e2f7972` 是已有边界的内存回归信号。
- Rust `Vec` borrow 和 Codex terminal insertion 属于实现特定优化；Reames CLI/React streaming 只需以长历史、
  Markdown、图片结果和内存 benchmark 验证，不复制 Rust helper。
- 该区间没有 OpenAI Responses、reasoning、tool schema、Realtime、App Server wire、插件或 Browser/CDP
  原生协议变化，因此不修改 P8 provider parity，也不虚构新模型支持。

## Claude Code

本地官方镜像仍为 `015170d3fd84fb57ef4685a64b673fadd0690dc1`，没有新的公开源码提交。Reames 不从
changelog/feed 反推 Messages、Thinking、Prompt Caching、tool/vision 或插件 runtime parity；Claude 的二级
战略地位保持不变，下一次真实代码变化仍须 code-level review。

## Hermes `36f2a966..299e409f`

前四个提交把 cron create/list/provider 解析绑定到 effective profile/model。Reames 当前 cron Job schema 尚无
provider/model/profile 字段；该信号进入 M7 multi-profile scheduling，不在 M6 为追求表面对齐扩大持久 schema。

`299e409f` 为 subagent 建立 cache 下可 `tail -f` 的 best-effort 人类日志。Reames 已有执行中持续写入、崩溃可
恢复的 full-fidelity subagent JSONL、running metadata、interrupted tombstone、`continue_from` 和 Controller
delivery projection，因此不复制第二份易漂移 transcript。可取机制是“运行中可观察”：后续 UI/headless 应从
现有 canonical child transcript 投影 bounded live view，而不是另写 Python 风格 cache log；日志 retention、
脱敏、权限和父子身份必须复用 Reames store/trust 边界。

## Grok Build `7cfcb20d..ba76b0a6`

该官方仓库仍是一次 monorepo snapshot，同一提交变化 143 文件、约 `+9465/-3419`。按三级机制参考分类：

- Stop/SubagentStop gate：最多 8 次 continuation、session-end 5 秒预算，并把 background task/subagent/cron
  快照交给 hook。可作为 Goal/evidence 完成前验证和 storm breaker 信号，但 Hook 不能越过 Reames evidence
  或把失败伪装成完成；当前不复制 Rust stop loop。
- `x.ai/session/state/import`：metadata columns + updates，清洗 host-specific 字段，以 `summary` 最后写入作为
  commit marker。可作为 P9 ACP/App-Server 跨主机 handoff 设计输入；Reames 现有 session lease、atomic file、
  runtime sidecar 和 recovery transaction 仍是唯一权威，不新增第二套 session store。
- permission manager：opaque `shell -c/eval` floor、env-risk 分类、classifier deny budget 与 deny guidance 是
  安全回归信号。Reames 的硬 permission/sandbox/path/credential gate 不允许被 LLM classifier override；是否
  增加有界自动拒绝要单独 threat-model 和故障注入。
- hooks、scheduler、ACP background notification、subagent coordinator 和 JSONL storage 有大规模重构，
  但没有证据支持当前 M6 引入 Rust runtime、xAI auth、managed policy、telemetry、online memory 或 marketplace。

## 路线图结论

- 本批直接代码采用只来自一级 Reasonix；Codex/Claude 继续维持原生模型和代码能力矩阵，当前区间无协议增量。
- Hermes/Grok 新增信号分别进入 M7 multi-profile、P9 live subagent/headless/session handoff 与未来 permission/
  stop-gate 加固，不改变当前 M6 交付门槛。
- 上游锁只能逐项接受 `reasonix`、`codex`、`hermes`、`grok-build`；禁止 `--accept-all`。
