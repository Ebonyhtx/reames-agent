# 2026-07-17 M4 可写子代理 Worktree 隔离与交付审计

## 结论

M4 持续加固 P1 已形成仓库内可验证闭环。产品装配中的 writer-capable `task`、
`runAs=subagent` Skill 和二级委派不再直接写父工作区：宿主从当前 committed `HEAD`
分配独立 `reames/subagent-*` branch 与托管 Git worktree，重绑全部 workspace built-in，
并以独立 workspace lease 串行化 writer。只读 task/Skill 继续共享父工作区且不分配 worktree；
非 Git、bare repository、无初始提交或选中子目录不在 `HEAD` 时 fail closed。

子代理完成只产生 delivery，不自动修改父工作区。父会话经统一 `control.Controller` 预览
branch/worktree、文件、commit、patch digest 和测试 receipt，再显式选择 `apply`、`merge`、
`rollback` 或 `reject`。CLI slash command、HTTP、Wails binding 与 Desktop “Changes” 面板
只是同一 Controller DTO 的投影，没有第二套 Agent runtime 或前端专用交付语义。

## 采用来源与本地重构

- Reasonix `03b39a65` 提供 workspace lease、writer isolation 和 delivery recovery 主机制；
  `03b39a65` 后续 readiness/concurrent-writer 修复一并纳入行为审查。
- Reasonix `bfb0bf4d` 与 `9a5011ce` 的 Windows hardening 促成 `core.longpaths=true`、
  hidden Git process、延长 worktree checkout timeout 和长路径真实测试。
- Reames 额外修复 Git for Windows 对 directory junction 使用物理路径、运行时保留逻辑路径
  导致的 registry 误判：canonicalization 会解析最近存在的父目录，worktree 已缺失时仍可对账。
- MiMo Code 的 worktree lifecycle 用于核对原子创建/回收与嵌套 child；Hermes 的统一
  worktree dialog 只用于交付 UX，不引入 Electron gateway、compute host 或第二套调度器。

## 运行时合同

1. 父 session、`sa_ref`、source root、execution root、repo root、branch、base HEAD、prefix 和
   source dirty 状态一起持久化；dirty source 不复制到 child，并明确显示。
2. Writer worktree 在首次模型执行前完成 registry 重绑。Provider 解析、registry 重绑、
   初始 transcript/effect journal 或 background job 建立失败时，只有本次新分配的 worktree
   会被回收；`continue_from` 的既有恢复 worktree 永不因准备失败被误删。
3. Child 文件 mutation 只写 child effect journal，不失效父 checkpoint/evidence；只有父会话
   apply/merge 才是 source mutation boundary。二级 child 的 source 是一级 worktree，不能越级
   修改主工作区。
4. Cancel/deadline 保存 `interrupted` 与 Git-derived 文件清单；普通失败保存 `failed`；崩溃
   启动恢复在 coordinator 绑定后对账，分类 `interrupted`、`lost` 或 `orphaned`，不自动重放工具。
5. 移入 Desktop 回收站会连同 subagent metadata 一起移动并保留 worktree，允许恢复；永久
   purge、ACP/Serve 删除和 ClearSession 必须先删除托管 worktree/branch，失败则保留 metadata
   供后续重试。
6. `sa_ref` 同时使用进程内锁与 Unix `flock` / Windows `LockFileEx`；真实子进程测试证明
   两个进程不能同时续跑同一引用。

## 交付与断电恢复

- `apply` 要求 source clean，将 sealed binary patch 以 `--check --index --3way` 应用并 staged；
  `merge` 创建 `--no-ff` commit。两者都在 runtime reservation 与 source workspace lease 内执行。
- 每次 source mutation 前先原子持久化 `applying`/`merging` intent，包含 source HEAD/status、
  delivery commit 和时间。intent 写失败时 Git 不变。
- 终态 metadata 写之前崩溃时，启动恢复按证明分类：source 仍等于 pre-state 则回到 `ready`；
  clean merge commit 的两个 parent 精确匹配 pre-head 与 delivery commit 时恢复为 `merged`；
  apply 后状态或其他漂移无法排除后续人工编辑时转为 `acceptance_interrupted`。
- `acceptance_interrupted` 禁止自动 reject、续跑或覆盖式 rollback。用户必须检查 Git state
  并人工解决；这是刻意的 fail-closed 边界，不用 `reset --hard` 猜测哪些变化属于 Reames。
- 已完成 apply/merge 的 rollback 只在当前 HEAD/status 与记录的 post-state 精确相等时执行；
  apply 回到 clean pre-head，merge 生成 mainline revert commit。任何后续 drift 都拒绝 rollback。

Controller 交付动作不伪装成一次对话 turn checkpoint：它使用自己的 durable acceptance
transaction。普通完成态可由该 transaction rollback；断电后的 ambiguous apply 明确要求人工
处理，不能声称 arbitrary source mutation exactly-once。

## 投影与测试

- 工具：`subagent_delivery_preview`、`subagent_delivery`。
- CLI：`/deliveries`、`/delivery preview|apply|merge|rollback|reject <sa_ref>`。
- Serve：`GET /deliveries`、`GET /delivery?ref=...`、`POST /delivery`，POST 继续受 JSON-only
  CSRF guard 保护。
- Desktop：Changes 面板展示状态、branch、文件、commit 与 test receipt；rollback/reject 使用
  native confirmation；`applying`、`merging`、`acceptance_interrupted` 不提供不安全操作。

真实 Git 回归覆盖：apply/merge/rollback/reject、dirty source、带空格路径、Windows 长 checkout、
junction identity、取消后的 dirty worktree、启动恢复、lost/orphaned、嵌套交付、跨进程引用锁、
永久 session purge、intent/终态持久化失败、Controller runtime reservation 与 HTTP 全链路。

## 明确限制

- Worktree 隔离只覆盖经 workspace-bound built-in 执行的 Git 文件 writer；child 不继承 MCP、LSP、
  memory mutation 或 source-root live service。外部 API 和 opaque shell side effect 不因此 exactly-once。
- source dirty changes 不进入 child；用户需要先提交、stash 或接受 child 基于 committed HEAD 工作。
- apply 的断电后 dirty state不能在不覆盖潜在人工编辑的前提下自动归因，因此保持
  `acceptance_interrupted`，而不是扩大自动恢复声明。
- Git 可执行文件缺失、仓库损坏或托管目录不可访问会阻断 writer/cleanup；read-only delegation
  仍可使用。

## 验证门槛

本批提交前已完成以下本地证据：

| 门禁 | 结果 |
|---|---|
| `go build ./...`、`go vet ./...` | 通过 |
| `go test ./internal/... -count=1 -timeout 300s` | 通过；包含真实 Git、跨进程锁、崩溃恢复与清理回归 |
| `go test -race ./internal/workspacelease ./internal/worktree ./internal/agent ./internal/control ./internal/serve ./internal/boot -count=1 -timeout 600s` | 通过 |
| `desktop: go test . -count=1 -timeout 300s` | 通过 |
| `desktop/frontend: pnpm test:all`、`pnpm build` | 通过；bundle budget、CSS 和类型门禁通过 |
| `pnpm smoke:plugin-browser` | 真实 Chromium 生命周期通过；无 console/page error 和横向溢出 |
| 六目标 `CGO_ENABLED=0` CLI 交叉编译 | Linux/macOS/Windows 的 amd64/arm64 全部通过 |
| `scripts/verify-baseline.ps1`、public/docs/deploy/release/upstream contracts、无凭据 Gateway smoke | 通过；Unix release dry-run 按 Windows 本地合同预期留给 Linux CI |
| clean-clone | 用独立临时 index 将全部 tracked/untracked 源码固化为 detached tree；root build/vet/full tests、Desktop full tests、frozen-lockfile install、frontend full tests/build、docs/public checks 全部通过 |
| Desktop Changes 响应式实测 | 真实浏览器在桌面宽度保持 dock；窄屏使用全宽 overlay，可关闭，无横向溢出和 console error |

远端 CI/CodeQL 只在整批单次 push 后记录，不以本地结果代替；push 前该项保持待验证。
