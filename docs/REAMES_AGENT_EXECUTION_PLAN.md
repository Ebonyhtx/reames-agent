# Reames Agent 详细执行计划 v2

> 状态：当前唯一执行计划
> 更新时间：2026-07-09
> 前置阅读：`docs/REAMES_AGENT_AUTHORITY.md`
> 主仓库：`F:\reames-agent` / `main` / `https://github.com/Ebonyhtx/reames-agent.git`

## 0. 执行总原则

1. **新主仓库优先**：所有主线开发在 `F:\reames-agent` 进行；`F:\Reames-Lite` 只作 legacy/reference。
2. **Reasonix 是底座**：优先保留和适配 Reasonix runtime/desktop/server/provider，不重写已有成熟机制。
3. **桌面 UI 优先**：近期优先级是桌面 Agent 视觉与真实点击体验，不再做 CLI 面板式桌面。
4. **Apple-light + 中文优先**：主 UI 面向用户，不展示工程迁移/基线/契约术语。
5. **cache/provider 不乱动**：保留 Reasonix cache-hit/prefix-cache，Reames 只补 metadata 隔离和 boundary tests。
6. **每批验证、提交、push**：小步可回滚，文档同步更新，验证命令写清楚。

## 1. 当前基线

### 1.1 新仓库

```text
Path:   F:\reames-agent
Branch: main
Remote: https://github.com/Ebonyhtx/reames-agent.git
Role:   新主项目；Reasonix-derived 源码底座
```

### 1.2 已建立的验证入口

```powershell
.\scripts\verify-baseline.ps1
go test ./... -run '^$'
```

注意：

- root `go test ./...` 不覆盖 `desktop/` nested module；
- `desktop/frontend/node_modules` 当前未安装；
- UI 批次必须恢复 frontend build 和浏览器/桌面截图验证；
- full `cd desktop; go test . -count=1` 当前耗时较长，可先用 critical desktop baseline 做门禁，再逐步扩大。

## 2. Phase A：文档收敛与接手入口

目标：让后来接手的人不再从旧 Reames Lite、Reasonix、Hermes、桌面实验文档里猜方向。

### A1. 权威文档 v2

交付：

- `docs/REAMES_AGENT_AUTHORITY.md`
- `docs/REAMES_AGENT_EXECUTION_PLAN.md`
- `docs/DOCS_INDEX.md`

验收：

- 文档明确 `F:\reames-agent` 是新主项目；
- 文档不再以“把 Reasonix 导入旧 Reames-Lite”为主语；
- 文档明确旧 Reames Lite 是 legacy/contract/reference；
- 文档明确桌面 UI 是近期最高优先级；
- 文档明确 Reasonix cache pipeline 保留，不被 Reames 覆盖。

### A2. 文档瘦身策略

暂不大规模删除历史文档。先做三件事：

1. `DOCS_INDEX.md` 标明唯一入口；
2. README 指向权威文档；
3. 后续每批如果发现旧文档误导方向，只做“归档/标记过时/从索引移除”，不随手删。

## 3. Phase B：恢复桌面前端可验证基线

目标：让桌面 UI 能真实运行、截图、点击、构建，而不是只改 Go 后端。

### B1. 安装并验证前端依赖

执行：

```powershell
cd F:\reames-agent\desktop\frontend
npm install
npm run build
```

如果 npm 版本或 lockfile 冲突：

- 先记录错误；
- 不盲目升级 React/Vite；
- 优先还原 Reasonix-compatible 依赖；
- 修改后必须提交 lockfile。

验收：

- `desktop/frontend/node_modules` 存在；
- `npm run build` 通过；
- 不引入大规模无关依赖升级。

### B2. 启动桌面/前端预览

优先顺序：

```powershell
cd F:\reames-agent\desktop\frontend
npm run dev
```

如果需要 Wails：

```powershell
cd F:\reames-agent\desktop
wails dev
```

验收：

- 能打开主界面；
- 能截图；
- 能点击主导航、设置、会话、输入区；
- 控制台没有阻断性错误。

### B3. 建立 UI 验证方法

后续 UI 批次必须至少满足其中两项：

- in-app browser 截图；
- Playwright/browser 自动检查 landmark；
- 手动点击验证记录；
- 前端 build；
- desktop critical Go tests。

## 4. Phase C：桌面 Agent 视觉和交互重建

目标：优先解决用户最不满意的问题：视觉乱、丑、不像桌面 Agent、工程文案太多。

### C1. 主界面结构

必须围绕“用户使用 Agent”设计：

| 区域 | 必须呈现 |
|---|---|
| App chrome | 应用标题、窗口控制、全局命令、设置入口 |
| 左栏 | 会话/项目/最近任务/新建任务 |
| 中央 | 对话流、任务进度、工具结果、Agent 提问 |
| 输入区 | 任务输入、发送/停止、附件、模式选择 |
| 右栏 | 工作区、文件变更、上下文、记忆、缓存/成本 |
| 设置中心 | 模型、密钥、权限、MCP、插件、记忆、外观、网络、更新 |

不允许主界面把工程计划、基线报告、迁移说明当作核心内容。

### C2. Apple-light token pass

统一视觉 token：

- 背景：浅灰/白，避免黑金和高对比噪音；
- 卡片：柔和圆角、轻阴影、清晰边界；
- 字体：中文可读，字号层级稳定；
- 间距：更像桌面应用，少堆叠；
- 按钮：主次明确；
- 状态：运行、等待、审批、失败、完成都要有清楚但克制的视觉表达。

验收：

- 主界面截图视觉统一；
- 设置页截图视觉统一；
- 审批弹窗截图视觉统一；
- 空状态不像工程 demo。

### C3. 中文产品 copy pass

把 UI 文案从工程语言改成用户语言。

重点检查：

- 首页/空状态；
- 输入区 placeholder；
- 设置分组；
- 模型/密钥说明；
- 权限审批；
- 工具执行状态；
- 缓存/上下文/成本展示；
- 错误提示。

禁止在普通 UI 出现：

```text
baseline
migration
contract
provider-visible
cache-first boundary
Reasonix import
Phase A/B/C
P0 verification
```

### C4. 真实点击体验

关键路径：

1. 新建会话；
2. 输入任务；
3. 看到 Agent 状态；
4. 打开设置；
5. 修改模型/密钥/权限；
6. 打开 MCP/插件/记忆；
7. 触发或模拟审批；
8. 查看工作区文件变更；
9. 暂停/继续/取消任务。

验收不是“按钮存在”，而是点击后用户能理解下一步。

## 5. Phase D：Reames public boundary

目标：把 Reames Lite 的核心优势迁成 Go 项目的边界约束，而不是复制旧 Python 内部实现。

### D1. 定义边界

候选位置：

```text
internal/reamesapi/
internal/boundary/
internal/control facade
```

边界能力：

- session create/list/open；
- submit user task；
- stream event；
- approve/deny；
- ask/answer；
- cancel/pause/resume；
- settings read/write；
- workspace snapshot；
- provider usage/cache summary。

### D2. Boundary tests

必须覆盖：

- Desktop/Web/Gateway 不直接构造 provider messages；
- UI layout/settings/channel metadata 不进入 provider-visible prompt；
- tool schema 顺序和 system prompt prefix 稳定；
- 新增中文 copy 不污染模型上下文；
- gateway channel identity 只在 runtime envelope，不进 prompt。

## 6. Phase E：清理 legacy/Hermes/Python 残留

目标：让仓库结构变清楚，但不能一上来暴删导致可用能力丢失。

### E1. 先分类

把残留分成：

| 类别 | 处理 |
|---|---|
| 当前可编译 Go/desktop/server | 保留 |
| 被 README/AGENTS 引用但实际未验证 | 标记待验证 |
| Python/Hermes legacy，但有参考价值 | 移到 legacy/reference 或文档说明 |
| 明确无用、无引用、无测试 | 单独清理 PR/commit |
| 大文件/生成物 | 从 git 中移除或加入 ignore |

### E2. 删除门槛

删除前必须满足：

- 有 `git grep` 引用检查；
- 有验证命令；
- 删除范围单一；
- 提交信息说明原因；
- 不和 UI 重构混在一批。

## 7. Phase F：Server/Web/Cloud

目标：像 Hermes 一样可以部署到云服务器，但不牺牲桌面体验。

优先保留 Reasonix `internal/serve`：

- auth；
- token/password；
- cookie；
- CSRF/path 安全；
- SSE broadcaster；
- session lease；
- local/remote 访问模式。

新增 Reames 云部署需要：

- Dockerfile/docker-compose；
- systemd；
- nginx reverse proxy；
- 环境变量/密钥说明；
- 只读/审批/沙箱策略；
- Web UI 与 Desktop 共用 boundary。

## 8. Phase G：Gateway

目标：融合 Hermes 风格多渠道入口，但保持 provider prompt 干净。

渠道：

- Feishu；
- WeChat；
- QQ；
- Telegram；
- 后续可扩展 Discord/Slack。

Gateway 规则：

- channel metadata 不进 prompt；
- 用户身份、渠道、消息 ID 只在 runtime envelope；
- 外发消息要有审批/节流/去重；
- 文件/图片/富文本先标准化为 runtime attachment；
- 所有渠道复用同一 Reames boundary。

## 9. 每批执行模板

每批开始前：

```powershell
cd F:\reames-agent
git status --short --branch
```

常规验证：

```powershell
.\scripts\verify-baseline.ps1
go test ./... -run '^$'
```

UI 批次额外验证：

```powershell
cd F:\reames-agent\desktop\frontend
npm run build
```

desktop Go 关键验证：

```powershell
cd F:\reames-agent\desktop
go test . -run 'TestWorkspaceChangesGitStatus|TestWorkspaceChangesGitStatusFromRepoSubdirectory|TestWorkspaceChangesUntrackedDirectoryListsFiles|TestWorkspaceChangesGitBranchDetachedHead|TestParseGitStatusPorcelainZ|TestHeartbeatConfigPathUsesReamesAgentUserStateDir' -count=1
```

每批结束：

```powershell
git status --short
git add <changed-files>
git commit -m "<type>: <summary>"
git push origin main
```

## 10. 当前下一步

按优先级：

1. **恢复 desktop frontend build/dev**：安装依赖、跑 build、打开界面；
2. **做桌面 UI 视觉校准**：Apple-light、中文、用户语言、主界面和设置中心；
3. **建立 UI 截图/点击验收**：主界面、设置页、审批弹窗、工作区；
4. **补 Reames boundary guard tests**：metadata 不进 prompt、cache prefix 不被污染；
5. **再清理 legacy 残留**：文档、Python、Hermes 混入内容分批处理；
6. **最后推进云部署和 Gateway**：复用 server/boundary，不另起一套。

## 11. 阶段完成定义

### 桌面 UI 阶段完成

- 用户打开后第一眼像桌面 Agent；
- 主界面、设置页、审批弹窗视觉统一；
- 默认中文；
- 没有工程自嗨文字；
- 能新建/切换会话；
- 输入区、工具状态、文件变更、设置入口可点击；
- 有截图或自动化验证记录；
- frontend build 通过。

### 项目基线阶段完成

- `.\scripts\verify-baseline.ps1` 通过；
- root compile-only test 通过；
- desktop critical tests 通过；
- 文档入口清晰；
- main 已 push；
- 后续接手者能根据本文档继续推进。

### 全面融合阶段完成

- Desktop/CLI/Web/Gateway 共用 runtime boundary；
- provider/cache pipeline 稳定；
- metadata 隔离有测试；
- 云部署可用；
- Gateway 可接至少一个真实渠道；
- legacy 残留已归档或删除；
- README/AGENTS/docs 与实际项目一致。
