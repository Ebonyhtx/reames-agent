# Reames Agent 详细执行计划

> 状态：唯一执行计划  
> 创建时间：2026-07-08  
> 前置阅读：`docs/REAMES_AGENT_AUTHORITY.md`  
> 约束：本计划用于指导后续执行；没有用户明确许可时，不执行代码导入、删除、重构、提交或 push。

## 0. 执行总原则

1. **先基线，后改造**：Reasonix 原样 baseline 不清楚前，不做 Reames 化。
2. **先公共边界，后多入口**：Desktop/Web/Gateway/CLI 都必须经过 ReamesClient-equivalent boundary。
3. **先真实测试，后主观 UI 调整**：截图和审美反馈重要，但必须绑定可复现 landmark/click/visual tests。
4. **保留 Reasonix cache pipeline**：不重写，不覆盖，只加 metadata-invisibility guard。
5. **每批小步可回滚**：每批有明确文件范围、验证命令、风险记录。
6. **不再散乱写文档**：方向更新写 `REAMES_AGENT_AUTHORITY.md`，执行进度/变更写本文件。

## 1. Phase A：文档收敛与接手入口

目标：让后来的人能接手，不再从几十份历史文档里猜方向。

### A0. 2026-07-09 新仓库接管基线

当前主仓库：

```text
Path:   F:\reames-agent
Branch: main
Remote: https://github.com/Ebonyhtx/reames-agent.git
Role:   新主项目；Reasonix 源码底座 + Reames/其他参考项目融合
```

本批已建立的基线命令：

```powershell
.\scripts\verify-baseline.ps1
go test ./... -run '^$'
```

注意事项：

- `desktop/` 是 nested Go module，root `go test ./...` 不会覆盖它；
- `desktop/frontend/node_modules` 当前未安装，UI 构建需要先安装前端依赖；
- full `cd desktop; go test . -count=1` 当前耗时较长，不作为本批硬门禁；本批门禁先固定 critical desktop baseline。

### A1. 标定权威文档

交付：

- `docs/REAMES_AGENT_AUTHORITY.md`
- `docs/REAMES_AGENT_EXECUTION_PLAN.md`

动作：

- 不删除旧文档；
- 旧文档作为证据库；
- 后续总方向只改 Authority；
- 后续执行计划只改 Execution Plan。

验收：

- 新接手者读两份文档能回答：
  - 项目目标是什么；
  - Reasonix 为什么是基座；
  - Reames 保留什么；
  - Hermes/Codex/Kimi 等融合什么；
  - 当前最大风险是什么；
  - 下一步从哪里开始。

## 2. Phase B：Reasonix 最新基线复现

目标：导入前先证明 Reasonix 最新源码在本机的真实状态。

### B1. 固定 Reasonix 源快照

当前快照：

```text
Path:   F:\code-reference\DeepSeek-Reasonix
Branch: main-v2
HEAD:   07c65c22226e4886004215168230e1e1edad734b
```

执行命令：

```powershell
cd F:\code-reference\DeepSeek-Reasonix
git fetch --all --prune
git pull --ff-only
git status --short --branch
git log -1 --oneline
```

验收：

- 工作树 clean；
- HEAD 记录进本计划；
- 如果 HEAD 变化，必须重新跑 B2-B5。

### B2. Provider/Agent cache baseline

执行：

```powershell
cd F:\code-reference\DeepSeek-Reasonix
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
go test ./internal/provider/openai ./internal/agent -run 'Test(Normalise|Normalize|Usage|Cache|SessionCache|SetSession|ReleaseCache|PlanModeDoesNotMutateSystemOrTools)' -count=1
```

必须保持通过。失败时不得继续导入。

### B3. Memory/config latest delta baseline

执行：

```powershell
cd F:\code-reference\DeepSeek-Reasonix
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
go test ./internal/memorycompiler ./internal/config -run 'Test.*' -count=1
```

意义：确认 `memory-v5-memory-candidates` 新增代码可用。

### B4. Desktop narrow baseline

执行：

```powershell
cd F:\code-reference\DeepSeek-Reasonix\desktop
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
go test . -run 'Test(Memory|Suggestion|Recovery)' -count=1
```

意义：确认桌面 memory/recovery 新增路径可用。

### B5. Desktop workspace baseline bug 复现与处理策略

当前已知失败命令：

```powershell
cd F:\code-reference\DeepSeek-Reasonix\desktop
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
go test . -run 'Test(Memory|Workspace|Suggestion|Slug|Tabs|Recovery)' -count=1
```

失败点：

```text
TestWorkspaceChangesGitStatus
TestWorkspaceChangesGitStatusFromRepoSubdirectory
TestWorkspaceChangesUntrackedDirectoryListsFiles
```

定位步骤：

1. 单独跑：

```powershell
go test . -run '^TestWorkspaceChangesGitStatus$' -count=1 -v
```

2. 用临时 debug test 或外部小程序验证：

- `git status --porcelain=v1 -z --untracked-files=all` raw 是否正常；
- `workspaceGit(...).Output()` raw 是否正常；
- `parseGitStatusPorcelainZ(raw)` 是否正常；
- `workspaceRelPathFromGitStatus(repoRoot, base, path)` 是否因 Windows 短路径/长路径不一致过滤。

修复策略候选：

| 策略 | 内容 | 优先级 |
|---|---|---|
| S1 | 在 `workspaceGitStatus` 中把 `repoRoot` 和 `base` 都 canonicalize 到同一 Windows path spelling 后再 `filepath.Rel` | 高 |
| S2 | 如果 `filepath.Rel(base, absPath)` 失败或越界，再尝试 `EvalSymlinks`/long path/short path normalization | 中 |
| S3 | 测试里避免空 App + cwd 短路径，显式设置 tab `WorkspaceRoot` | 低，不治本 |
| S4 | 暂时跳过 workspace git-status 测试 | 禁止，除非只作为临时外部环境隔离且记录 |

验收：

- 三个 workspace git-status 测试通过；
- 不破坏 `TestWorkspaceChangesUsesRequestedTabCheckpoints`；
- 不让 workspace 外路径出现在 files list；
- Windows 和非 Windows 都有合理路径处理。

## 3. Phase C：Staged import 设计

目标：原样导入 Reasonix，但不污染主线、不混改、不立刻删除 Reames。

### C1. 分支策略

用户明确允许执行后再做：

```powershell
cd F:\Reames-Lite
git switch -c codex/reasonix-base-migration
```

要求：

- 不在 `main` 上直接大规模复制；
- 分支创建后立即 push；
- 每批导入可回滚。

### C2. 导入布局

推荐：

```text
third_party/reasonix/NOTICE.md       # upstream commit/license/attribution
migration/reasonix-base/             # first unmodified import, optional staging
cmd/                                 # later promoted Reames CLI/server/gateway entrypoints
internal/                            # later Reasonix-derived runtime
desktop/                             # later Reasonix-derived Wails desktop
web/                                 # later browser UI
server/deploy/ or deploy/            # Docker/compose/systemd/reverse proxy
```

第一批不要做：

- 不 rebrand；
- 不改 UI；
- 不改 cache；
- 不混 Hermes；
- 不删 Python；
- 不把 packages/desktop 删掉。

第一批只做：

- copy source；
- preserve license；
- record commit；
- run baseline commands；
- record failures；
- push branch。

## 4. Phase D：Reames public boundary

目标：让 Reasonix-derived runtime 不被 UI/Web/Gateway 直接乱调。

### D1. 设计 Go public boundary

候选包：

```text
internal/api
internal/reamesclient
internal/runtimeapi
```

职责：

- Submit/Cancel/Steer；
- Approve/Deny/Answer；
- Session list/open/fork/rewind；
- Context/Usage/Cache status；
- Settings read/write；
- Workspace/files/git；
- Memory；
- Server/Gateway message ingress。

### D2. Boundary tests

必须新增测试：

1. Desktop 只能调用 public API/bridge，不直接 mutate provider messages。
2. Web/server submit 只能走 API envelope。
3. Gateway channel metadata 不进入 provider messages。
4. Settings/layout/theme/sidebar/right-dock state 不进入 provider messages。
5. Product copy 不进入 system prompt。

测试命名建议：

```text
internal/api/provider_payload_boundary_test.go
internal/api/gateway_metadata_boundary_test.go
desktop/frontend/src/__tests__/product-copy-visibility.test.ts
```

## 5. Phase E：Desktop UI 重建

目标：基于 Reasonix 真实桌面 UI，而不是继续修当前 Reames 手写 UI。

### E1. UI skeleton 保留

必须保留：

```text
AppChrome
TabBar
Transcript
Composer
ApprovalModal
CommandPalette
SettingsPanel
ContextPanel
WorkspacePanel
MemoryPanel
StatusBar
Welcome
bridge.ts
useController.ts
layout store
i18n/locales
```

### E2. Apple-light token pass

只改视觉 token，不改结构：

- 背景：浅色、毛玻璃、低饱和；
- 边框：细、半透明；
- 阴影：轻；
- 圆角：Apple-like；
- 字体：中文优先，系统字体；
- 不再黑金；
- 不再 Nintendo；
- 不再工程 dashboard。

### E3. 中文产品 copy pass

普通用户界面不能出现：

```text
migration
provider prompt
runtime/status
RPC
params
turn/start
baseline
cache prefix changed（默认视图）
Reasonix migration
```

这些词只能出现在高级诊断/开发者模式。

普通用户应看到：

- 新任务；
- 当前项目；
- 继续会话；
- 模型；
- 审批；
- 记忆；
- 上下文；
- 文件；
- 设置；
- 权限；
- 连接；
- 自动化；
- 发送；
- 停止；
- 继续。

### E4. UI 验收测试

必须有：

- first-screen landmark test；
- settings open/click test；
- command palette keyboard test；
- composer send/cancel test；
- approval modal test；
- context/status cache display test；
- no engineering self-talk test；
- zh-CN default copy test；
- screenshot/manual acceptance checklist。

## 6. Phase F：Server/Web

目标：让项目以后能像 Hermes/AgentArk 一样部署到云服务器，同时不牺牲本机桌面。

### F1. Preserve Reasonix `internal/serve`

先保留：

```text
internal/serve/serve.go
internal/serve/auth.go
internal/serve/broadcaster.go
internal/serve/*_test.go
```

不要一开始就重写成新 RPC。

### F2. Reames server mode

新增入口候选：

```text
cmd/reames-server
```

能力：

- bind host/port；
- loopback 默认安全；
- public bind 必须 auth；
- REST/SSE 起步；
- 后续再加 WebSocket snapshot/cursor。

### F3. Web UI

借鉴 Kimi/Codex：

- session snapshot；
- event cursor；
- pending approvals/questions/tasks；
- reconnect recovery；
- browser workspace controls；
- auth-aware API client。

## 7. Phase G：Gateway

目标：融合 Hermes 的强项，但不复制整个 Hermes。

### G1. Gateway runtime-only envelope

消息 envelope 必须包含：

```text
platform
channel_id
thread_id
message_id
sender_id
sender_display
attachments
received_at
routing_metadata
```

禁止直接进入 provider prompt。

只允许转换后的用户内容进入 user message，例如：

```text
用户在飞书发来：请帮我检查这个 PR
```

平台 metadata 留在 runtime/audit，不进 prompt。

### G2. Platform registry

借鉴 Hermes：

```text
internal/gateway/registry
internal/gateway/platforms/base
internal/gateway/adapters/<platform>
```

先做 generic webhook/API adapter，再做一个高价值平台。

### G3. Gateway tests

必须测试：

- signature validation；
- dedupe；
- rate limit；
- redaction；
- outbound approval；
- metadata prompt-invisibility；
- reconnect/backoff；
- channel allowlist。

## 8. Phase H：Legacy retirement

目标：删除旧东西，但只在新东西站住后删。

### H1. 删除条件

必须全部满足：

- Reasonix-derived runtime build pass；
- desktop can open first screen；
- settings center works；
- submit/cancel/approval basic flow works；
- cache/provider tests pass；
- Reames boundary tests pass；
- provider-visible metadata-invisibility tests pass；
- server smoke pass；
- old packages/desktop no longer required。

### H2. 删除/归档顺序

1. 标记旧 desktop 为 deprecated；
2. 移除旧 desktop route/build；
3. 删除伪 Reasonix UI/CSS；
4. 归档旧桌面计划；
5. Python core 只在新 runtime parity 后移动到 `legacy/python-reames/` 或保留分支；
6. README 和 docs index 最后统一更新。

## 9. 每批执行模板

每批必须写清楚：

```text
目标：
改动文件：
为什么改：
不改什么：
风险：
验证命令：
结果：
下一步：
```

推荐命令：

### Reames 当前仓库

```powershell
ruff check .
ruff format --check .
python -m pytest tests/test_reames_client.py tests/test_cli_architecture_boundary.py tests/test_cache_first.py tests/test_cache_shape.py tests/test_desktop_shell_contract.py -q
python scripts/cli_virtual_terminal.py --all --check
```

### Reasonix root

```powershell
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
go test ./internal/provider/openai ./internal/agent -run 'Test(Normalise|Normalize|Usage|Cache|SessionCache|SetSession|ReleaseCache|PlanModeDoesNotMutateSystemOrTools)' -count=1
go test ./internal/memorycompiler ./internal/config -run 'Test.*' -count=1
```

### Reasonix desktop

```powershell
cd F:\code-reference\DeepSeek-Reasonix\desktop
$env:GOPROXY='https://goproxy.cn,direct'
$env:GOSUMDB='sum.golang.google.cn'
go test . -run 'Test(Memory|Suggestion|Recovery)' -count=1
go test . -run '^TestWorkspaceChangesGitStatus$' -count=1 -v
```

## 10. 当前下一步

在用户允许“开始执行”之前，下一步只应该做调研/计划，不做代码导入。

如果用户允许执行，第一批实际执行应是：

1. 创建 `codex/reasonix-base-migration` 分支；
2. 做 Reasonix workspace git-status bug 最小修复方案验证；
3. 写可复用 baseline script；
4. 原样 staged import Reasonix；
5. 跑 baseline；
6. 只记录结果，不做 UI rebrand；
7. push 分支。

## 11. 完成定义

这个迁移不是文档完成就完成。真正完成至少要满足：

- 桌面首屏像一个可用桌面 Agent；
- 设置中心真实可点；
- 命令面板真实可用；
- composer/approval/context/workspace/memory/status 都有真实路径；
- Apple-light 中文 UI；
- 无普通用户可见工程自嗨文案；
- Reasonix cache-hit pipeline 保留；
- Reames boundary tests 存在并通过；
- Server/Web 可启动；
- Gateway metadata 不进 prompt；
- 旧桌面实验已删除或归档；
- 新接手者只读两份文档就能继续推进。
