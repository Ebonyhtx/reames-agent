# Reames Agent 权威接手文档

> 状态：权威接手入口  
> 创建时间：2026-07-08  
> 适用范围：以 Reasonix 最新源码为基座改造 Reames Lite 为多平台/桌面 Agent  
> 结论：新接手者先读本文件；要开始执行时再读 `docs/REAMES_AGENT_EXECUTION_PLAN.md`。

## 0. 为什么需要这份文档

当前仓库已有很多历史计划、审计、进度、桌面实验文档。继续让新接手者从几十份文档里拼结论会非常混乱。本文件把已经真实调研过的结论收敛成一个权威入口：

- 最终产品目标是什么；
- Reasonix、Reames、Hermes、Codex、Kimi、MiMo、Scream、Claude、AgentArk、Impeccable、Apple refs 分别承担什么角色；
- 哪些模块该保留、该移植、该融合、该删除、该谨慎处理；
- 已经确认的源码事实和风险；
- 后续执行时不能再犯的错误。

从本文件创建后，之前分散文档只作为“证据库/历史记录”使用，不再让它们彼此竞争成为方向来源。

## 1. 一句话产品目标

把 Reames Lite 从当前 Python/CLI 为主、桌面实验反复失败的项目，重建为一个 **Reasonix 源码基座 + Reames 公共边界与产品约束 + Hermes 云/网关能力 + Apple-light 中文桌面体验** 的多平台桌面 Agent。

目标形态：

```text
Reames Agent
├─ Desktop：像 Reasonix/Codex 桌面 Agent，而不是 CLI 面板；Apple-light，中文优先
├─ CLI：保留本地编程 Agent 能力，后续接入同一公共 runtime 边界
├─ Server/Web：可在本机、局域网、云服务器部署和访问
└─ Gateway：Hermes-style 平台适配器，支持外部消息渠道，但 metadata 不进 prompt
```

## 2. 当前权威源码快照

### 2.1 Reames Agent 当前主仓库

- 路径：`F:\reames-agent`
- 分支：`main`
- 远端：`https://github.com/Ebonyhtx/reames-agent.git`
- 定位：新主项目；以 Reasonix 源码为底座，吸收 Reames Lite 和其他参考项目优点。
- 接手原则：先稳定 Reasonix/Go/Wails 基线，再做 Reames 公共边界、桌面体验、云部署和网关融合。
- 注意：root `go test ./...` 不覆盖 `desktop/` nested module，桌面基线必须单独验证。

### 2.2 Reames Lite legacy/contract 仓库

- 路径：`F:\Reames-Lite`
- 分支：`main`
- 最新已知提交：`1230f781c docs: refresh reasonix baseline and import policy`
- 当前角色：legacy/contract/reference；不再作为主线开发仓库。

重要 Reames 资产：

| 路径 | 价值 | 后续处理 |
|---|---|---|
| `docs/ARCHITECTURE.md` | ReamesClient 公共边界、cache-first、provider-visible 边界、CLI/Desktop/外部通道隔离规则 | 保留为 Reames 契约来源，但不要照搬 Python 内部实现到 Reasonix |
| `packages/core/src/reames/api/client.py` | 当前 Python `ReamesClient` 的接口证据 | 作为 Go `internal/api` / `ReamesClient` 等价层设计参考 |
| `tests/test_reames_client.py` | 公共 API 契约测试 | 迁移到 Go/跨语言 boundary tests |
| `tests/test_cli_architecture_boundary.py` | UI 不可直接 import 内部的边界测试 | 保留思想，移植成新架构边界测试 |
| `tests/test_cache_first.py`、`tests/test_cache_shape.py` | Reames cache-first 约束证据 | 不替换 Reasonix cache，只用来补“metadata 不进 prompt”的测试 |
| `packages/desktop` | 当前被用户否定的手写桌面实验 | 不再作为主路线；Reasonix baseline 成功后逐步淘汰 |

### 2.3 Reasonix 参考仓库

- 路径：`F:\code-reference\DeepSeek-Reasonix`
- 分支：`main-v2`
- 当前 HEAD：`07c65c22226e4886004215168230e1e1edad734b`
- 提交：`Merge pull request #6162 from SivanCola/feature/memory-v5-memory-candidates`
- License：MIT，`Copyright (c) 2026 Reasonix Contributors`
- 当前状态：工作树干净

Reasonix 是主基座，不是单纯 UI 参考。原因：它已经具备 Go runtime、Wails desktop、React/Vite UI、server/auth、permissions、memory、plugin、Windows desktop、DeepSeek cache-hit 机制和大量测试。

## 3. 最重要的纠错：不要再说“移植 Reames cache-first 到 Reasonix”

这个说法是错的。

源码调研确认：Reasonix 已经适配 DeepSeek/OpenAI-compatible cache-hit 机制。

关键证据：

| Reasonix 文件 | 已确认事实 |
|---|---|
| `internal/provider/provider.go` | `Usage` 已有 `CacheHitTokens` / `CacheMissTokens`，并支持 cache-aware pricing |
| `internal/provider/openai/openai.go` | 已归一化 DeepSeek `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` 与 OpenAI/MiMo `prompt_tokens_details.cached_tokens` |
| `internal/provider/openai/realcache_test.go` | 有 env-gated 真实 DeepSeek cache probe |
| `internal/agent/cache_shape.go` | 有 `PrefixShape`、`CaptureShape`、`CompareShape`，用于 prefix diagnostics |
| `internal/agent/cache_diagnostics_test.go` | 验证 usage events 带 cache diagnostics |
| `internal/agent/cachehit_e2e_test.go` | 用 mock DeepSeek endpoint 验证 byte-stable prefix、cache rate、session aggregate cache |
| `internal/agent/agent.go` | 累计 `sessCacheHit` / `sessCacheMiss` 并发到事件 |
| `desktop/frontend/src/components/ContextPanel.tsx` | 桌面侧已有 context/cache 展示 |
| `desktop/frontend/src/components/StatusBar.tsx` | 桌面状态栏已有单轮/会话 cache rate 概念 |

正确原则：

```text
Reasonix cache pipeline = 保留
Reames cache-first = 作为边界约束和测试补充
```

Reames 后续只补：

- UI/settings/layout/gateway/channel metadata 不进入 provider prompt；
- 公共 runtime boundary；
- 中文 Apple-light 产品体验；
- 测试和文档纪律。

## 4. Reasonix 必须保留的模块

这些模块应作为新项目基础，不要重写：

### 4.1 Runtime / Agent / Controller

| 文件/目录 | 价值 | 保留方式 |
|---|---|---|
| `internal/control/controller.go` | turn run、submit、approval、ask、steer、plan、goal、session、checkpoint、memory、MCP、tool approval | 保留为 runtime 核心，外包一层 Reames API |
| `internal/event/event.go` | 统一事件模型，含 usage/cache diagnostics/approval/ask/tool/memory stats | 保留，后续映射到 Desktop/Web/Gateway 事件协议 |
| `internal/agent` | Agent loop、cache shape、usage、session、planmode、tools、subagent 等 | 保留，先跑基线测试，再做 Reames 化 |
| `internal/permission` | allow/ask/deny、subject extraction、session/persistent grants | 保留，映射为 Reames 统一审批体验 |
| `internal/plugin`、`internal/skill` | MCP/plugin/skill 生命周期 | 保留，后续融合 Claude/Scream 的 UX 概念 |

### 4.2 Provider / Cache

| 文件/目录 | 价值 | 保留方式 |
|---|---|---|
| `internal/provider` | Provider 接口、Usage、Pricing、cache counters | 保留 |
| `internal/provider/openai` | DeepSeek/OpenAI/MiMo compatible provider | 保留，禁止用 Reames Python provider 覆盖 |
| `internal/agent/cache_shape.go` | prefix diagnostics | 保留并在 UI 高级诊断中展示 |
| `internal/agent/cachehit_e2e_test.go` | cache-hit regression tests | 导入后必须继续跑或等价迁移 |

### 4.3 Desktop Go / Wails

| 文件 | 价值 | 保留方式 |
|---|---|---|
| `desktop/app.go` | Wails bridge 主入口，Submit/Approve/Settings/Context/History/Memory/Fork/Rewind/Workspace 等大量真实方法 | 保留为桌面 bridge 基础，但要加 Reames public boundary |
| `desktop/settings_app.go` | 设置读写 bridge | 保留并本地化/重命名配置 |
| `desktop/sessions.go`、`desktop/tabs.go` | session/tab 持久化和多标签行为 | 保留；注意最新版 `tabs.go` 有较大变化，导入前重审 |
| `desktop/workspace.go`、`desktop/workspace_changes.go` | 工作区、文件、git 状态、工作区面板基础 | 保留但谨慎，当前已发现 Windows path baseline 问题 |
| `desktop/bot_runtime_app.go`、`desktop/bot_connection_app.go` | bot/channel 设置和运行时 bridge | 作为 Hermes-style gateway 的初始 seam，不要删 |
| `desktop/memory_suggestions*.go` | 最新 memory-v5 candidate/suggestions | 作为新 HEAD 的重要增量保留 |

### 4.4 Desktop React UI

| 文件/组件 | 价值 | 保留方式 |
|---|---|---|
| `desktop/frontend/src/App.tsx` | 主 UI composition：chrome、tabs、workbench、composer、status、right dock、settings、palette | 导入为 UI skeleton，不再手写假 UI |
| `components/AppChrome.tsx` | 标题栏、窗口控制、tabs、command entry | 保留，改 Apple-light 和中文 |
| `components/TabBar.tsx` | 多会话/标签 | 保留 |
| `components/Transcript.tsx` | 消息流、工具卡、reasoning、compaction、question jump | 保留 |
| `components/Composer.tsx` | 输入框、模式、goal/plan/tool approval、attachments | 保留，作为桌面 Agent 核心交互 |
| `components/ApprovalModal.tsx` | 工具/计划审批 | 保留并中文化 |
| `components/CommandPalette.tsx` | 命令面板 | 保留，命令改成用户语言 |
| `components/SettingsPanel.tsx` | 完整设置中心：general/models/bots/MCP/skills/plugins/memory/hooks/shortcuts/permissions/sandbox/network/appearance/updates | 必保留；这是不要重写设置页的核心理由 |
| `components/ContextPanel.tsx` | context/cache/cost/token/source/type | 保留，普通用户文案简化，高级诊断可展开 |
| `components/WorkspacePanel.tsx` | 文件树、git changes、history、preview | 保留，但先处理 workspace baseline 问题 |
| `components/MemoryPanel.tsx` | memory facts/docs/archive/suggestions | 保留 |
| `components/StatusBar.tsx` | cache/model/workspace/git/context/cost/balance | 保留，中文化、用户化 |
| `lib/bridge.ts`、`lib/useController.ts` | 前端 runtime seam 和状态控制器 | 保留，后续套 Reames boundary |
| `lib/i18n.tsx`、`locales/*.ts` | 多语言基础 | 保留，zh-CN 作为默认目标 |
| `store/layout.ts` | layout persistence | 保留，防止布局漂移 |

### 4.5 Server / Web

| 文件/目录 | 价值 | 保留方式 |
|---|---|---|
| `internal/serve/serve.go` | REST/SSE server：submit/cancel/approve/plan/context/history/status/sessions/skills/todos 等 | 保留，作为 `reames-server` 起点 |
| `internal/serve/auth.go` | token/password/cookie/rate-limit/loopback/TLS/safe redirect | 保留，云部署必须用 |
| `internal/serve/broadcaster.go` | SSE fanout | 保留，后续再决定是否加 WebSocket |
| `internal/serve/*_test.go` | auth/serve/session/lease/lock/csrf/path tests | 保留并作为 cloud safety baseline |

## 5. Reames 必须保留的东西

Reames 不再做桌面 UI 基座，但保留以下核心约束：

1. **公共 API 边界**：所有 UI/CLI/Web/Gateway 只能通过 ReamesClient-equivalent boundary 进入 runtime。
2. **provider-visible payload 隔离**：UI 状态、layout、settings、gateway metadata、channel identity、debug state、产品文案不得进入 provider-visible messages/tool schemas。
3. **cache-first 作为边界测试**：不替换 Reasonix cache，但检测任何 Reames 新增层是否污染 prefix。
4. **中文产品体验**：默认中文，不显示工程自嗨文案。
5. **测试纪律**：每个执行批次必须有明确验证命令。
6. **文档收敛纪律**：后续不再随意新增散乱文档；方向看本文件，执行看 `REAMES_AGENT_EXECUTION_PLAN.md`。

## 6. 其他参考项目只融合“机制”，不做主基座

| 项目 | 不作为基座的原因 | 只融合什么 |
|---|---|---|
| Hermes | 强项是 gateway/channel/server，不是桌面 Agent shell | platform registry、adapter contract、message envelope、redaction、dedupe、signature、rate limit、outbound approval、Docker/compose |
| Codex | 强项是 app-server/thread/approval/sandbox protocol，不是 Reames 产品壳 | approval schema、thread lifecycle、typed app-server boundary、worktree/sandbox concepts |
| Kimi Code | 强项是 Web event/snapshot/cursor，不是桌面壳 | `/api/v1` 风格、WebSocket/Snapshot/Cursor/session event model |
| MiMo Code | 强项是 ACP session/permission queue、桌面密度 | permission queue、session update density、composer/workbench concepts |
| Scream Code | 强项是 agent core modes/hooks/subagents | goal/plan/subagent/hook/skill concepts |
| Claude Code reference | 强项是 plugin/skill/hook UX | skills/plugins/hooks/background agent UX/accessibility |
| AgentArk | 强项是 server/deploy/secrets/audit | Docker/server posture、encrypted secrets、audit/sentinel/noisy-output reduction |
| Impeccable | 强项是 restraint/design QA | design tokens、anti-pattern tests、视觉克制 |
| awesome-design-md Apple | 用户指定 Apple 风格参考 | Apple-light surface/tokens/spacing/visual language |

## 7. 应该删除或淘汰的东西

不要现在鲁莽删除。删除条件是：Reasonix baseline import 能构建、能跑测试、能打开桌面 UI，且 Reames boundary tests 已建立。

| 对象 | 命运 | 删除/迁移条件 |
|---|---|---|
| 当前 `packages/desktop` 手写 UI 实验 | 淘汰 | Reasonix desktop staged import 可运行后替换 |
| 伪 Reasonix 复刻组件/CSS | 删除 | 新 UI skeleton 基于 Reasonix 后删除 |
| 工程自嗨 UI 文案 | 删除/替换 | UI copy pass 阶段全部替换为用户语言 |
| 旧桌面完成宣称 | 归档或标注历史 | 新权威文档建立后，不再作为完成证据 |
| “移植 Reames cache-first 覆盖 Reasonix”相关表述 | 删除/修正 | 已明确错误 |
| 过时桌面计划和重复审计 | 历史证据 | 不再作为方向入口 |
| Python runtime 主实现 | 暂不删 | 新 Go/Reasonix-derived runtime 达到 parity 后再进入 legacy |

## 8. 已知真实风险

### 8.1 Reasonix workspace git-status baseline 问题

调研中发现：

- `go test . -run 'Test(Memory|Workspace|Suggestion|Slug|Tabs|Recovery)' -count=1` 在 Reasonix `desktop` 模块失败；
- 失败集中在：
  - `TestWorkspaceChangesGitStatus`
  - `TestWorkspaceChangesGitStatusFromRepoSubdirectory`
  - `TestWorkspaceChangesUntrackedDirectoryListsFiles`
- 失败表现：`WorkspaceChanges("")` 返回空 file list。

进一步定位结论：

- 独立 `git status --porcelain=v1 -z --untracked-files=all` 能正确输出 `AM tracked.txt` 与 `?? untracked.txt`；
- Reasonix `workspaceGit(...)` 也能拿到 raw bytes；
- `workspaceGitStatus(base)` 返回空；
- 当前倾向原因：Windows 下 `t.TempDir()`/cwd 使用短路径，例如 `ADMINI~1`、`TEMP_C~1`，而 `git rev-parse --show-toplevel` 返回长路径，导致 `workspaceRelPathFromGitStatus(repoRoot, base, path)` 归一化时把真实文件误判为 workspace 外路径并过滤。

这不是阻止选择 Reasonix 的理由，但它是导入前必须处理的 baseline bug，不能隐藏。

### 8.2 Go dependency proxy

默认 `proxy.golang.org` 在当前环境曾超时；使用以下环境变量后测试通过：

```powershell
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
```

后续 baseline 脚本必须显式设置或允许配置 Go proxy。

### 8.3 文档失控

当前最大协作风险不是代码，而是文档太多、互相竞争。后续接手者默认只看：

1. `docs/REAMES_AGENT_AUTHORITY.md`
2. `docs/REAMES_AGENT_EXECUTION_PLAN.md`

其他文档只作为证据库。

## 9. 新接手者工作规则

1. 不要继续修当前手写桌面 UI，当主线已经转为 Reasonix base。
2. 不要直接删除 Python/Reames 代码，先把 Reasonix-derived baseline 跑起来。
3. 不要重写 Reasonix cache/provider/agent；只加 Reames boundary guard。
4. 不要让 Desktop/Web/Gateway metadata 进入 prompt。
5. 不要把用户看不懂的工程词放进产品 UI。
6. 不要只看 UI 截图就改；先看 Reasonix source + tests。
7. 不要新增第三份“总计划”；执行只更新 `REAMES_AGENT_EXECUTION_PLAN.md`，背景只更新本文件。

## 10. 当前结论

最真实、最稳的路线是：

```text
Reasonix latest source as base
+ preserve Reasonix DeepSeek cache-hit pipeline
+ wrap with Reames public boundary
+ localize/restyle to Apple-light Chinese desktop Agent
+ extend Reasonix server with Hermes/Kimi/Codex gateway/web concepts
+ retire current Reames desktop experiment only after baseline parity
```

这不是“复制别人 UI 改 logo”，也不是“把 Reames 机制塞进 Reasonix”。正确做法是：用 Reasonix 作为已经成型的桌面 Agent/runtime 基座，把 Reames 的边界、产品、中文、测试纪律和其他参考项目的强项融合进去。
