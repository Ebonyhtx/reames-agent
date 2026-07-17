# 2026-07-18 Reasonix 全量代际与 bug-fix parity 审计

## 结论

本次审计不再以 GitHub 说明、release note 或少量候选提交代替源码更新。审计对象是 DeepSeek Reasonix `main-v2` 从 Reames 初始导入基线到当前主线的完整 Git 范围：

```text
Reames import commit: ef3a132
Reasonix baseline:     07c65c22226e4886004215168230e1e1edad734b
Reasonix reviewed:     3637d0f028bb8223d50ba9490a0ab5140eada4f3
stable tag:            v1.17.14 / desktop-v1.17.14
all commits:           672
non-merge commits:     498
exact fix/perf set:    314
range diff:            993 files, +144544 / -27687
```

机器可重跑的逐提交账本位于 [`reasonix-generation-07c65c2-3637d0f.json`](../upstreams/reviews/reasonix-generation-07c65c2-3637d0f.json)。它列出每个非 merge 提交的 SHA、时间、完整 subject、涉及文件、归属审计区域，以及是否属于精确 lowercase conventional `fix(...)` / `fix:` / `perf(...)` / `perf:` 集合。生成器与测试分别是 `scripts/audit_reasonix_generation.py` 和 `scripts/test_audit_reasonix_generation.py`。

最终判断：Reasonix 当前代际的核心可靠性、Provider/MCP、安全、恢复和工作模式机制已经完成代码级分类。Reames 不整分支 merge，不追求品牌或版本号一致；已经证明有缺口的机制按 Reames 的 `control.Controller`、cache-first、权限、checkpoint 和 evidence 边界吸收。主题资产、生产发布链、Reasonix 遥测/marketplace 和较弱的 trust 简化不进入本批。

## 方法与完成标准

1. 使用 Git 对象而不是网页说明，固定 baseline/reviewed SHA。
2. 枚举范围内全部 498 个非 merge 提交与触及路径；314 个精确 fix/perf 提交形成 bug-fix parity population。
3. 每个提交至少归入一个审计区域；跨区域提交可重复计数，因此区域计数之和大于 498。
4. 对安全、数据丢失、卡死、stale state、重复调用、权限和恢复类变更优先进行源码/测试对照。
5. 结果只能是：已吸收、Reames 已有等价或更强机制、架构/产品不适用、明确进入后续路线。不能用“看过说明”作为完成证据。

## 区域覆盖矩阵

| 区域 | Reasonix 信号 | Reames 决策与证据 |
| --- | --- | --- |
| source-baseline | 早期 `main-v2` 是 Reames Go/Wails 基座 | `ef3a132` 明确记录导入 `07c65c2`；本审计固定到 `3637d0f0`，不把 Reames 后续独立架构误写成上游原样代码。 |
| active-refs-tags | 稳定 `v1.17.14`；主线之后仍有依赖、inline renderer、macOS home migration、status bar、Chrome tabs、missing-drive crash、reasoning jank 等分支 | 分支逐项分类见下节；未 merge 分支不是主线完成声明。`origin/v1` 是另一条产品线，不进入 `main-v2` 源码基线。 |
| agent-runtime | 空答案重试、DeepSeek reasoning-only stop、delivery/readiness、subagent writer | `d3cfa5c2` 已按显式 `RequiresToolCallReasoning` capability 吸收，见 `internal/agent/agent.go` 与 `empty_final_test.go`；Goal/Todo/evidence、writer worktree 和 durable child journal 已比早期上游更强。 |
| controller-transports | Desktop/CLI/Serve/ACP/Bot 多入口持续演化 | Reames 所有入口通过 `control.SessionAPI`/版本化 command/event DTO；transport runtime import ratchet 的 allowlist 为空。工作模式新增仍落在 boot/controller 装配，而不是前端复制 Agent loop。 |
| provider-cache | schema、Anthropic usage、错误体卡死、finish reason、DeepSeek/MiMo 兼容 | MCP schema 隔离、credential-free cache identity、Anthropic 累计 usage、MiMo dialect、失败 body deadline、stream completion 与 reasoning-only stop 均已有源码和回归；稳定 system prompt 与工具顺序继续受 cache 合同保护。 |
| tools-mcp-plugins | MCP persistent session、identity/trust、schema 故障隔离、插件生命周期 | 采用 schema 与 re-verification 修复；拒绝 `8f2c209a` 的 trust 简化方向。Reames 保留 identity-bound receipt、launcher exact pin、capability drift 撤权、不可变 generation、fresh-human destructive approval 和 package sandbox。 |
| permissions-sandbox | config 写审批、secret/env 隔离、Windows sandbox/worktree hardening | 已有 per-write/fresh-human gate、凭据环境隔离、`os.Root` writer、跨进程 lease/worktree、package Hook/MCP OS sandbox 与 Safe Mode fail-closed。 |
| session-memory-compaction | `dae65e25` 会话切换/保存修复；`c966d027` 删除 Memory v5 compiler | `dae65e25` 的 verified snapshot fencing、event-log salvage、last-click-wins 已吸收。Memory Compiler 不跟随删除：Reames 默认 `observe` 不注入 Provider prompt，`compact` 需显式选择，并有 task classifier、最多 5 次注入、30 秒冷却、噪声抑制和完整回归；该差异保留为产品选择，不冒充上游一致。 |
| goal-plan-work-modes | `e7e5bc2c` runtime profiles/delivery、`38f3774a` TUI work mode、`88be1a23` Plan/权限分离、Desktop token economy | 已闭环两个正交轴：普通/计划/目标与 economy/balanced/delivery。Delivery 复用 Todo、`complete_step`、project checks、checkpoint、evidence 和现有子代理交付，不建立第二套 runtime。 |
| desktop-ui-accessibility | 主题、Composer、history 时间、external opener、reasoning jank、status items、tab crash | 历史时间、Windows opener、status item 配置、reasoning streaming 截断/显示模式切换和 serialized navigation 已有等价或更强实现；missing-drive 分支的异常已由统一 navigation catch/toast 覆盖。Theme Pack V2 进入 P5，只吸收安全导入/内容寻址/可撤销预览，不复制官方图片与品牌。 |
| cli-acp | TUI work mode、model picker、responsive composer、ACP config | 交互 CLI、`run`、`serve`、`acp` 的 `--profile`、TUI `/work-mode`、会话继承、失败原子 rebuild、approval 状态保留和 `work_mode` ACP config option 已实现；模型/effort 后续重建保持当前 profile。 |
| serve-gateway | 多入口 profile 与远程 gateway | `serve --profile` 并在模型/effort rebuild 保持 profile；Bot/Gateway 明确默认 balanced，尚无稳定 per-connection 合同时不增加隐式渠道状态。 |
| recovery-update | offline Guard、Safe Mode、updater transaction、session recovery | P2/P3 已建立 Guard preflight、crash-loop ledger、repair transaction、完整安装单元回滚、recovery-only Desktop 和 DTO 数组合同；真实签名/notarization 仍是 external-blocked。 |
| build-release | `3637d0f0` 合并 GitHub/SignPath 审批；Reasonix 多 workflow 正式发布 | 不采用。Reames 生产发布继续禁用，只保留 candidate、CI、CodeQL、六目标交叉编译和发布棘轮；正式 signing/provenance/registry 必须有真实外部证据。 |
| bug-fix-parity | 314 个精确 fix/perf 提交 | 全部在机器账本中逐 SHA/路径枚举并映射到上述区域。高风险修复优先源码对照；UI 微调、品牌、发布和产品专用项按区域决策处理，不再用抽样表冒充完整集合。 |

## 主线后关键提交决策

| 提交 | 变化 | 决策 |
| --- | --- | --- |
| `dae65e25` | stalled error body、verified save、损坏 event log、会话切换卡顿/内容丢失 | 已按 Reames persistence/Controller 架构吸收并有故障注入。 |
| `8f2c209a` | MCP persistent sessions，同时简化 trust | 只采用 persistent-session 修复信号；拒绝放宽 Reames identity receipt 与 fresh-human trust。 |
| `f590a66e` | TUI picker 未返回 pending model switch | Reames 当前 quick picker/model switch 已有等价异步 command 回归。 |
| `1bd5f04d` | CLI composer、状态栏与 TUI work mode | work-mode 合同已采用；纯样式变化只在有 Reames 缺口时吸收。 |
| `c966d027` | 删除 Memory v5 execution compiler | 明确产品分歧，暂不删除；默认 observe 不注入，compact 显式 opt-in 且有额外限制。后续若真实 benchmark 证明净负收益再独立退役，不能因上游删除而机械删除。 |
| `7f00d2c2` | Theme Pack V2 与官方主题资源 | 进入 P5。只吸收安全 schema、原子 store、内容 digest、preview rollback；不复制 Reasonix 官方图片、品牌、marketplace 或 endpoint。 |
| `d3cfa5c2` | DeepSeek reasoning-only `finish_reason=stop` 被误重试 | 已直接吸收并限制在明确 DeepSeek thinking capability，其他 Provider 继续保留空答案重试。 |
| `3637d0f0` | 正式 release 审批/SignPath workflow | 当前不适用；Reames 生产发布尚未启用。 |

## 活跃未合并分支

机器账本记录了 12 个未并入 reviewed SHA 的远端 ref。分类如下：

- 四个 Dependabot 分支：只作为独立依赖升级候选；必须通过 Reames 自己的 Go/Frontend/bundle/candidate 门禁，不能按上游版本自动升级。
- `feature/inline-renderer`：breaking TUI 默认行为。Reames 已有 viewport/native scrollback 双路径和大量终端回归；在真实 Windows/macOS/Linux/Termux 交互证据前不切默认。
- `feature/macos-home-migration-pr`：Reasonix 品牌 home 迁移不适用于独立 Reames home；Reames 已有显式 legacy config/session import，不读取或接管 Reasonix 用户目录。
- `feature/status-bar-display-options`：Reames 已实现可见项选择、顺序调整、持久化和测试，判定等价或更强。
- 两个 Chrome tab strip 分支：保留为 UI 回归信号；Reames 的 tab layout/overflow 合同独立，未证明当前缺口前不移植像素实现。
- `fix/desktop-open-tab-crash-missing-drive`：Reames serialized navigation 已统一捕获 blank/topic/history 失败并 toast，判定已覆盖。
- `fix/desktop-streaming-reasoning-jank`：Reames 已有 `displayReasoningText`、streaming DOM 截断、compact 展示与 streaming 中 display-mode 变化处理，判定已覆盖。
- `origin/v1`：与 `main-v2` 大幅分叉的另一条产品线，不属于本项目 primary-base tracking range。

## 工作模式实现合同

- 内部历史值：`full`；公开名：`balanced`。
- `economy`：核心工具 + `connect_tool_source`，可选来源按需连接。
- `balanced`：完整稳定工具面，默认模式。
- `delivery`：与 balanced 使用相同工具 schema，只增加稳定 `<delivery-profile>` system contract。
- Desktop tab/session、CLI session meta、ACP sidecar 会保存 economy/delivery；balanced 省略以保持兼容。
- 活动 turn、待确认交互和后台任务期间不进行半切换；Desktop/TUI 构建失败保留旧 Controller，ACP 复用既有排队与原子 rebuild 状态机。
- Plan/Goal 和工具权限与工作模式正交；Delivery 不绕过审批、sandbox 或 Plan gate。

## 遥测与外部依赖边界

Reames 自有远端启动统计、metrics、crash 和 performance 上传已经永久删除。panic、卡顿和 bot 诊断只写本机 `<Reames Agent home>/diagnostics/reports/`；反馈账本只写部署节点自己的本地 JSONL，不连接 Reames endpoint。

开发、测试和普通 CI 不需要用户服务器。真实 Provider、IM、云部署、签名/notarization、公开 registry 运营和 provenance 仍分别属于可选集成或 `external-blocked` 发布证据，不会阻塞仓库内 P4。

## 验证命令

```powershell
python -m unittest scripts.test_audit_reasonix_generation -v
python scripts/audit_reasonix_generation.py --repo F:\code-reference\DeepSeek-Reasonix `
  --baseline 07c65c22226e4886004215168230e1e1edad734b `
  --reviewed 3637d0f028bb8223d50ba9490a0ab5140eada4f3 `
  --out docs/upstreams/reviews/reasonix-generation-07c65c2-3637d0f.json
python -m unittest scripts.test_check_upstreams -v
go test ./internal/boot ./internal/control ./internal/cli ./internal/acp ./internal/serve ./internal/bot -count=1
```
