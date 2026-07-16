# 下一位模型接手交接文档

日期：2026-07-16

仓库：`F:\reames-agent`

分支：`main`

本页只记录当前接手边界。工作树、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和
远端检查比本文更权威。

## 用户目标与工作节奏

持续大步推进到高可信可交付状态。参考 DeepSeek Reasonix、`F:\code-reference` 和
`F:\Reames-Lite` 时只吸收适用机制；每批同步代码、测试、文档和证据，充分本地验证
后集中 commit/push，避免碎片 push 重复消耗 CI。

单元/合同测试、localhost fixture、真实浏览器、原生 Desktop、远端 candidate、真实
Provider/IM/云节点证据必须分层表述，不能互相冒充。

## 受保护路径

以下未跟踪路径属于用户或其他会话，禁止修改、暂存或提交：

```text
.agents/
artifacts/
docs/audits/2026-07-09-reference-feature-gap-map.md
```

只使用显式 `git add -- <paths>`，禁止 `git add .` 和 `git add -A`。

## 当前基线

- M0、M1、M2、M3、M4 已按各自路线图门槛关闭；当前已验证远端基线为
  `92e15e7 feat: unify structured source approvals`。该提交的 CI
  `29386016928` 8/8、CodeQL `29386016898` 3/3 全绿；最近完整 Desktop candidate
  `29378899444` 仍是三平台全绿。当前提交和远端状态必须以 `git log`、`git status` 和
  GitHub Actions 为准。
- M5 进行中。已收口 plugin manifest、内容身份、权限授权、两阶段生命周期、Desktop
  自动化审批、旧 generation 运行时撤销、package-owned Hook/MCP 进程隔离与一条真实固定
  revision 第三方 E2E；真实 Chromium、源码 production Wails 与已安装 Windows candidate
  均已有分层证据。仍未关闭运营 registry、签名/provenance、密钥轮换与公开发布信任链。
- candidate `29378899444` 的 Windows interaction/accessibility/native/plugin 四份 JSON
  均通过，`boundary_changes=[]`、`errors=[]`；installer/executable SHA-256 分别为
  `779706C1FA70D172912527E9130C4D9FDEFC1AD5C40885EF7BB719445438DF09` 和
  `4326C3B5DFC690DA584E8E1F20A8AD061CD03EFAB0780F8C9F6E2ECEE6DA394F`。
- M6 的真实 linger logout/reboot、干净云节点、真实 IM 和公开签名 release 仍为
  `external-blocked`，不能用 mock 或 localhost 代替。

## M5 本批合同

### Manifest、内容与授权

- 原生 `schemaVersion: 1` 要求 semver 和与实际 skills/hooks/MCP 完全一致的权限集合；
  损坏 native manifest 不回退到兼容格式。
- `sha256-tree-v1` 覆盖路径、大小、执行位和文件字节；拒绝 symlink/reparse/special
  file，并限制文件数与总字节数。
- copy 安装发布不可变 generation，状态 v2 原子选择 active/previous；新安装默认禁用，
  enable 绑定 exact digest 和 exact grants，权限扩张更新自动禁用。
- GitHub 来源记录 shallow clone commit revision，但信任状态仍是 unsigned HTTPS，
  不得表述为 Reames 签名或 provenance。

### 两阶段生命周期与恢复

- install/update/rollback/uninstall 都强制 `preview -> planId -> apply`；完整 state token 在
  状态锁内再次比较，审批后的并发 enabled/grants/previous 变化或并发卸载 fail closed。
- 状态 mutation 同时使用进程内 mutex 与 OS 文件锁；受管 staging/create/publish/delete
  使用 `os.Root` 相对路径并保持目录句柄身份。
- 故障注入覆盖 copy、publish、state write、rollback、uninstall、cleanup、staging 身份
  替换、tamper 和 mutable link 漂移；inactive orphan 可报告/清理，但没有 durable
  lifecycle journal，不能声明断电 exactly-once。

### Desktop 与运行时所有权

- Desktop Go/Wails 方法和 React 已自动化覆盖 install/update/rollback/remove 的计划与
  apply、版本/权限/信任/摘要差异、planned/done/partial/failed/blocked/denied 展示，
  以及 exact digest/grants 的显式 enable 授权。
- 插件 MCP owner 不落盘，绑定 controller 实际加载的配置并在同名用户接管前清除；
  MCP connect/disconnect 与 owner 更新由同一个 mutation mutex 串行，同名接管不会与
  插件撤销交错后留下错误 owner。模型侧 `install_source` 生命周期回调也通过 controller
  动态 owner-aware 接口断开插件 MCP，不再读取启动时静态 owner map。
- 更新、回滚、卸载或禁用会先取得 Desktop work-start 写门和所有 visible/detached
  controller 的 runtime-mutation reservation；reservation 与 `ExecuteCommand` 原子交接并
  复用 rotation gate，空闲检查后不能再起跑新 turn、Shell、会话旋转或后台入口。
- mutation 与同步 rebuild 共用 `runtimeRebuildMu`；成功后取消按旧状态启动但尚未发布的
  异步 build，重新枚举当前 controller，精确断开旧插件 MCP、按
  `REAMES_AGENT_PLUGIN_NAME` 移除旧 Hook，并暂停旧 controller 的 Skill 入口到重建或
  新会话。同名用户 MCP 不受影响。
- package-owned Hook/MCP 现携带 owner/root/state/home/workspace policy，使用 core-only
  wrapper env + 显式 child env、独立 state/tmp、严格 OS sandbox 和敏感 read barrier；
  manifest 环境原始键值不进入 wrapper argv；隐藏 helper 只在隔离边界内恢复并清除编码
  child env，payload 缺失/损坏或宿主漏注册 dispatch 时 fail closed。运行中 Hook 会登记
  owner cancel，disable/update/rollback/
  uninstall 可回收进程树并用 disabled 标记阻断 snapshot 后 late start；MCP Remove 继续关闭
  transport。用户手工 Hook/MCP 与 LSP 不自动迁移到该策略。
- Windows package-owned 无扩展名 shebang Hook 会使用经过 WSL 排除的 Git Bash；真实
  `obra/superpowers@d72560e462a74e10d161b7f993d5fc3282bfa1e2` 已在隔离 home 中完成
  unsigned 固定 revision copy install、精确 `hooks.context/hooks.execute/skills.load` 授权、
  完整性验证和原生 sandbox SessionStart E2E，`hookCount=2`、`contextCount=2`、无 notice。

### 跨宿主结构化审批

- `install_source apply=false` 按调用级只读；`apply=true` 在执行前重算确定性计划，并要求
  先前 preview 返回的精确 `planId`。审批包含 actions、风险、target、权限、版本/digest、
  trust、source revision 和 MCP URL/command/args/env/headers；URL 凭据、敏感 query/参数、
  环境变量与请求头在展示和 pending persistence 前结构化脱敏。
- Controller 强制 fresh-human 决策；YOLO、Auto、Guardian、已批准 Plan 执行窗口、
  session/persistent grant 和 headless autonomy 均不能代答。显式 deny 在联网或读盘预览前
  拦截；无结构化审批能力及 headless apply 的宿主零预览、零执行 fail closed。
- Desktop、Bubble Tea CLI、Bot、Serve/event wire 与 ACP 消费同一计划；pending snapshot
  和 replay 保留完整字段，结构化批准后 PreToolUse hooks 仍可阻断。
- 本批 43 个交付文件（33 个既有文件修改、10 个新增文件）已通过 root
  build/vet/internal 全测、七个高风险包 race、Desktop
  vet/full test、前端 `test:all`/production build/bundle budget、基线与合同、119 项 Python
  合同（2 skipped）、真实 Chrome plugin smoke、实际 upstream scan、品牌残留 0 和六目标
  `CGO_ENABLED=0` 交叉编译。这些结果是本地证据；提交、push 与远端 CI/CodeQL 状态必须
  以当前 `git log`、`git status` 和 GitHub Actions 单独核验。

## 本地验证门禁

`a0c09de` 提交前结果：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race ./internal/hook ./internal/control ./internal/pluginpkg ./internal/installsource -count=1 -timeout 900s
Desktop: go vet . && go test . -count=1 -timeout 300s
Frontend: corepack pnpm test:all && corepack pnpm build
Tool contract and Go brand residue checks
Python/Node upstream tracking tests
linux/darwin/windows x amd64/arm64, CGO_ENABLED=0
gofmt -l: empty; git diff --check: passed
```

以上门禁均通过。一次将内部全套、Desktop 和前端同时高负载并行的运行曾使 ACP approval
和 delegation 各有一个短超时；两个失败用例隔离各重复 10 次全绿，随后
`go test ./internal/... -count=1 -timeout 300s` 单独全量通过。该噪声未被隐藏，也没有通过
放宽测试时限或删除断言处理。工具合同、基线脚本、品牌残留 0、上游追踪实际扫描和六目标
临时目录交叉编译均通过；M5 四包 race、Desktop 全测和前端 `test:all/build` 全绿。

六目标二进制和综合报告只写入系统临时目录。以上为本地证据，不替代集中 push 后的新
CI/CodeQL；不要为了回填静态 run ID 单独 push。

第二批新增的定向证据：

```text
Real Chromium: backend=browser-mock, Chrome 150.0.7871.115, full plugin UI lifecycle passed
Native Wails: sha256 11D8391D..., 15.2s, stale-plan/install/enable/update/rollback/doctor/remove passed
Native state: update digest changed, rollback restored original digest, boundary_changes=[], errors=[]
Python UIA/plugin contracts, TypeScript, component actions and production frontend build passed
```

普通 CI 的真实 Chromium smoke 已在 run `29378573077` 通过；Desktop candidate
`29378899444` 已生成安装后 Windows plugin lifecycle 通过证据。

当前流中断事务修复已重新通过完整本地门禁：root build/vet/internal 全测、
`internal/control` 全测与 race、Desktop vet/full test、前端 `test:all`/production build
与 bundle budget、工具/文档/公共发布/部署合同、119 个 Python 合同测试（2 skipped）、
Node upstream reconciliation、实际 upstream scan，以及六目标 `CGO_ENABLED=0` 交叉编译。
聚焦回归同时覆盖 partial transcript 成功提交和注入提交失败后的 fail-closed 回滚；
`git diff --check` 通过；其远端 CI/CodeQL/candidate 现已由上述 `e9de895` runs 关闭。

当前进程隔离批次已重新通过 root build/vet/internal 全测，hook/control/plugin/pluginpkg/
installsource/processpolicy/sandbox 七包 race，Desktop build/vet/full test，前端
`test:all`/production build/bundle budget，文档/公开/部署/发布/工具合同，119 项 Python
合同（2 skipped）、Node upstream reconciliation、真实 Chrome smoke、六目标 CLI 与
Linux/macOS 四包 16 个测试二进制 `CGO_ENABLED=0` 交叉编译。Windows 原生 re-exec E2E
确认显式 child env 在隔离内可用、保留变量已清除；Linux 单元合同确认私有 `/tmp` 后只读
重挂 generation/helper。提交前仍需最终差异审查、显式暂存与干净 clone；这些本地结果不
替代 push 后的新远端 CI/CodeQL。

## 未关闭边界

- package process 当前允许网络，且无跨三平台统一硬 CPU/RSS 配额；用户手工 Hook/MCP
  与 LSP 仍是高权限未自动隔离进程。
- 真实运营 registry、签名、provenance、密钥轮换和公开可信发布链。
- 最终候选干净 clone；关闭 M5 时最新提交必须远端 CI/CodeQL 全绿。
- `bash`、MCP、外部 API 和后台 opaque side effect 仍无任意副作用 exactly-once。

## 下一执行顺序

1. 核对当前分支、提交和远端状态；若本批尚未 push，显式暂存受跟踪文件后集中
   commit/push 一次，若已 push 则不要为回填静态 run ID 重复推送。
2. 守候最新 CI/CodeQL；若失败，从日志根因修复而不是重跑掩盖。
3. 完成最终候选干净 clone；随后推进运营 registry、签名/provenance 和密钥轮换。
4. 外部环境到位时并行关闭 M6 云节点/IM 和公开签名发布证据。

长期 GOAL 尚未完成；M5 本批的本地合同不得扩大为插件生态或整个项目完成。
