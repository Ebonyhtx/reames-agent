# 下一位模型接手交接文档

日期：2026-07-15

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

- M0、M1、M2、M3、M4 已按各自路线图门槛关闭；进入 M5 本批时 `main`/`origin/main`
  基线为 `32e5e6e feat: close M4 cross-resource recovery`。当前提交和远端状态必须以
  `git log`、`git status` 和 GitHub Actions 为准。
- M5 进行中。本批收口 plugin manifest、内容身份、权限授权、两阶段生命周期、
  Desktop 自动化审批和旧 generation 运行时撤销，但不关闭原生交互、进程隔离、
  registry 签名、真实第三方 E2E 与发布链。
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
- 已经运行的 Hook/子进程不会被此 controller 内撤销强杀；Hook/MCP/插件进程 OS sandbox
  和故障隔离仍未完成。

## 本地验证门禁

2026-07-15 当前工作树的提交前结果：

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

## 未关闭边界

- 真实浏览器与原生 Wails 插件安装、授权、更新、回滚、移除和失败恢复交互。
- 所有模型宿主共用的 permission/control 结构化审批 UX。
- Hook/MCP/插件进程 OS sandbox、资源限制、崩溃和恶意进程隔离。
- 真实运营 registry、签名、provenance、密钥轮换和公开可信发布链。
- 至少一个真实第三方插件 E2E、干净 clone、远端 CI/CodeQL 和 candidate。
- `bash`、MCP、外部 API 和后台 opaque side effect 仍无任意副作用 exactly-once。

## 下一执行顺序

1. 完整复核 staged diff；只显式暂存允许文件，形成一个 M5 大提交。
2. 单次 push 后守候并修复该提交的新 CI/CodeQL，不为回填 run ID 额外 push。
3. 取得真实浏览器/原生 Wails 插件交互；随后统一模型宿主审批并推进插件进程隔离。
4. 外部环境到位时并行关闭真实第三方插件、M6 云节点/IM 和公开签名发布证据。

长期 GOAL 尚未完成；M5 本批的本地合同不得扩大为插件生态或整个项目完成。
