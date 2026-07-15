# M5 插件生命周期信任审计

> 日期：2026-07-15
>
> 范围：M5 生命周期、跨宿主审批、Desktop 原生交互和运行时所有权合同
>
> 结论：核心、跨宿主审批与 installed candidate 已形成分层证据；M5 尚未完成

## 缺口与采用机制

旧实现会在 copy 更新前删除旧目录，状态写失败可能丢失 active 内容；uninstall
先删状态或目录时也没有完整故障语义。计划 ID 没有绑定插件内容，GitHub branch 和
本地目录可在预览后漂移，新安装会直接启用，状态没有跨进程锁，也没有可审阅的
摘要、权限差异和回滚 generation。

本批参考并适配：

- Codex `dc23c7bcc8fd543fe727c7b4447a3d785836932f`：同盘 staging、激活失败保留
  旧 root、本地与远端版本身份分离；
- Claude Code `be02c39841a59e2ac1f35ac12285def02acdbb5a`：loader 前显式同意、固定
  revision 优先、不得清理 active 版本；
- DeepSeek Reasonix `0e0cb63c712e89f8ab8f23cd1a30f374f9f386ed`：保留现有 Go/Wails
  工程底座；
- Reames Lite `1230f781cf103bb6f3624c7455325e5d3dd7c902`：只保留轻量发现/启停
  契约，不复用其缺少内容身份和授权边界的供应链模型。

## 已实现合同

1. 原生 `schemaVersion: 1` 要求合法 semver，权限必须与 skills、hooks 和 MCP
   实际能力推导的集合完全一致；规范 native manifest 文件名为
   `reames-agent-plugin.json`，旧 `reamesAgent-plugin.json` 仅作为带弃用警告的兼容别名；
   规范 manifest 已存在但损坏时不回退到兼容格式。
2. `sha256-tree-v1` 覆盖相对路径、大小、执行位和文件字节，拒绝 symlink、reparse
   与特殊文件，并限制 4096 文件、64 MiB。
3. copy 安装发布到 `plugins/<name>/versions/<digest>/`；状态 schema v2 原子选择
   active generation 并保留一个完整 previous release。新安装默认禁用。
4. 启用请求绑定 exact digest 与 exact permission grant；link 或落盘内容变化不会
   被静默采纳。权限扩张更新自动禁用。
5. install/update/rollback/uninstall 都先产生确定性 action；插件 apply 必须携带匹配
   `planId`。完整 `InstalledPlugin` 另有 opaque state token，update/rollback/uninstall 在最终
   状态锁内再次比较，审批后并发改变 enabled/grants/previous 或并发卸载会 fail closed。
   GitHub 计划记录 shallow clone 的 commit revision 并在 apply 重新校验。
6. 状态 mutation 使用进程内 mutex 和 OS 文件锁；真实 helper-process 竞争验证并发
   Upsert/Remove 不丢更新，持锁进程退出后内核会释放锁。
7. 受管创建、rename 发布和删除通过以 Reames home 为锚的 `os.Root` 相对操作；
   staging 从创建、复制、摘要到发布保持同一目录句柄并复核文件身份；无效/重复 state
   name、目录 symlink/junction、staging 路径替换和越界删除 fail closed。
8. runtime 验证执行 digest-before → parse → digest-after，并直接使用这次返回的
   `Package`，不在验证后再次按路径解析；验证窗口内变化会被拒绝。
9. Desktop 暴露 install/update/rollback/remove 的 `Plan*` 与 apply 方法；React 只有在
   当前预检输入和 `planId` 一致时允许确认，显式 enable 页面绑定 exact digest 与权限，
   自动化覆盖 planned/done/partial/failed/blocked/denied 展示合同。
10. 插件 MCP owner 只存在于内存，并绑定 controller 实际加载的配置；MCP connect、
    disconnect 和 owner 更新由同一个 mutation mutex 串行，主动断开会清除 owner，因此
    同名用户 MCP 后续接管不会继承插件所有权或在并发撤销中被误断开。更新、回滚、卸载
    或禁用时，Desktop 先取得 work-start 写门和每个 live/detached controller 的
    runtime-mutation reservation；reservation 与 `ExecuteCommand` 原子交接并复用 rotation
    gate，阻止空闲检查后新 turn、Shell、会话旋转或后台入口起跑。随后
    `runtimeRebuildMu` 串行同步 rebuild，取消按旧状态启动但尚未发布的异步 build，再精确
    断开旧插件 MCP、按 `REAMES_AGENT_PLUGIN_NAME` 撤销旧 Hook，并暂停旧 controller 的
    Skill 发现/调用工具；模型侧 `install_source` 生命周期回调同样调用 controller 的动态
    owner-aware 断开接口，不再使用启动时 owner 快照。新 generation 只在 controller 重建
    或新会话后加载。
11. `install_source` 以调用级只读性区分 preview 与 apply；apply 在执行前重算确定性计划，
    要求模型携带先前 preview 的精确 `planId`，并向审批方展示 actions、风险、target、
    权限、版本/digest、信任、source revision 及 MCP URL/command/args/env/headers；敏感
    URL、参数、环境变量与请求头在展示和 pending persistence 前结构化脱敏。Controller
    的 fresh-human 决策不能由
    YOLO、Auto、Guardian、Plan 执行窗口、已有 grant 或 headless autonomy 替代；显式
    deny 在联网/读盘预览前拦截，不支持结构化审批及 headless apply 的宿主零预览、零执行地 fail closed。
    Desktop、Bubble Tea CLI、Bot、Serve/event wire 和 ACP 消费同一计划，pending snapshot
    与 replay 保留完整字段；PreToolUse hooks 仍可在用户批准后阻断执行。

## 故障与运行时证据

自动测试覆盖：

- copy/publish 失败不产生 active generation；rename 后目录同步失败清理 orphan；
- state write 失败保留旧 active 与内容；rollback state write 失败保留 current；
- uninstall state write 失败不删内容，cleanup 失败只留下已从 state 移除的 inactive
  orphan 并返回 warning；
- digest tamper 让已启用插件在 `LoadInstalled` 前被跳过；mutable link 变化后旧授权
  不能启用；
- Approval callback 内模拟并发状态改变时，update/rollback/uninstall 的锁内 state token
  比较拒绝 stale apply；验证中途内容变化和发布前 staging 身份替换也有确定性回归；
- CLI 真实路径覆盖 install、权限摘要与 enable、update preview/apply、rollback
  preview/apply、remove preview/apply；
- Desktop Go/React 自动化覆盖两阶段 install/update/rollback/remove、权限扩张自动禁用、
  exact enable grant、operation status，以及 visible/detached controller 的禁用撤销；
- ownership 回归证明配置碰撞、运行时同名用户接管以及模型侧 `install_source` 生命周期
  操作均使用 controller 当前 owner，不会因启动时静态视图误断开用户 MCP；startup build
  generation 取消、并发 Hook/Skill/Registry 撤销有定向和 race 覆盖；
- runtime reservation 回归证明 mutation 期间 versioned Submit、直接 Shell 和 rotation
  fail closed；Desktop 并发 Submit 会等待 mutation 完成后恢复，而不是穿过 active-work
  检查窗口启动旧 generation turn；
- 跨宿主审批回归覆盖 deny-before-preview、unsupported-host fail closed、exact-plan 绑定、
  planning 只读性、YOLO/headless 边界、pending snapshot/replay、eventwire/ACP/IM 字段和
  Desktop 风险/版本/修订/信任/权限展示与批准/拒绝动作。

本批本地门禁命令：

```powershell
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race ./internal/hook ./internal/control ./internal/pluginpkg ./internal/installsource -count=1 -timeout 900s
Push-Location desktop; go vet .; go test . -count=1 -timeout 300s; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
```

工具合同、品牌残留、上游追踪脚本和六目标 `CGO_ENABLED=0` 交叉编译也属于本批提交前
门禁；跨平台二进制只写入系统临时目录。以上仍是本地证据，不替代集中 push 后的新
CI/CodeQL。

## 浏览器与原生 Wails 交互证据

第二批把 React 合同提升为两条互不冒充的实际交互链：

- `desktop/frontend/scripts/smoke-plugin-browser.mjs` 使用 `playwright-core` 驱动系统
  Chrome/Edge，不下载浏览器。证据固定标记 `backend=browser-mock`，只声明真实
  Chromium 的设置、install、enable、update、rollback、doctor、remove 和布局链路；
  本机 Chrome `150.0.7871.115` 运行通过，`console_errors=[]`、`page_errors=[]`、
  `horizontal_overflow=false`，截图 SHA-256 为
  `3dced154e5c9a32000ce9fe3930a5bfcf0922d14edd6138af7e083d2ebbcafe9`。
- `scripts/smoke_desktop_plugin_lifecycle.py` 启动源码 production Wails，可执行文件
  SHA-256 为 `11D8391D1DDCE62BF731F6F1AB84E5298471DFCC1175725CE41CC28D144D31A1`
  （48,677,376 bytes）。它使用隔离 `REAMES_AGENT_HOME` 和本地 schema v1 合成包，
  经 UIA 与真实 Go 后端完成 stale install plan 拒绝、1.0.0 默认禁用安装、exact
  `skills.load` 授权、2.0.0 不同 digest 更新、恢复原 digest 的 1.0.0 回滚、doctor、
  两阶段移除和受管安装根清理；最终 15.2 秒运行结果为 `outcome=passed`、
  `boundary_changes=[]`、`errors=[]`、`temp_cleaned=true`。
- 原生调试先暴露两个真实缺口：无可访问语义的普通容器不会进入 WebView2 UIA 树，
  styled checkbox 的隐藏几何也不是可靠点击合同。插件 page/plan/row/banner 已补
  region/group/alert/status 语义，checkbox 改用聚焦后 Space 的标准键盘路径，并有
  React 与 UIA 驱动单测。
- 普通 CI 前端 job 运行真实 Chromium smoke 并保留七天 JSON/截图；Desktop candidate
  Windows job 在实际 NSIS 安装后运行原生插件 smoke，JSON 绑定 installer/executable
  SHA-256 并保留十四天。两条 workflow 变更只有远端运行成功后才构成远端证据。

## 远端基线与 candidate 诊断

commit `e9de895` 的普通 CI run `29378573077` 8/8、CodeQL run `29378573116` 3/3
全绿；其中真实 Chromium plugin lifecycle smoke 通过。Desktop candidate run
`29378899444` 的 Linux、macOS 和 Windows jobs 全部通过。已下载的 Windows 工件中，
installer SHA-256 为 `779706C1FA70D172912527E9130C4D9FDEFC1AD5C40885EF7BB719445438DF09`，
安装后 executable SHA-256 为 `4326C3B5DFC690DA584E8E1F20A8AD061CD03EFAB0780F8C9F6E2ECEE6DA394F`；
interaction、accessibility、native 和 plugin lifecycle 四份 JSON 均为 `outcome=passed`，
`boundary_changes=[]`、`errors=[]`。interaction 实际完成 19 请求、五类失败恢复、partial
stream 持久化、Stop 和重启恢复；plugin lifecycle 完成 stale plan、默认禁用、精确授权、
update/rollback、doctor 和 remove。该证据关闭先前 run `29376807221` 暴露的 partial
response 回滚及 installed Windows candidate 缺口。

当前跨宿主结构化审批批次的 root build/vet/internal 全测、M5 四包 race、
Desktop vet/full test、前端 `test:all`/production build/bundle budget、基线与工具/文档/公开/
部署/发布合同、119 项 Python 合同（2 skipped）、真实 Chrome plugin smoke、实际 upstream
scan、Go 品牌残留 0 和六目标 `CGO_ENABLED=0` 交叉编译均已通过；这些命令只构成本地
证据，提交与远端 CI/CodeQL 状态必须以当前 `git log` 和 GitHub Actions 单独核验。

## 未关闭边界

- GitHub 来源仍是 unsigned HTTPS；没有运营中的默认 registry、Reames 签名、
  provenance 或密钥轮换。
- 权限授权不等于 Hook/MCP 子进程受到 OS sandbox；插件崩溃、资源耗尽和恶意进程
  的隔离 E2E 尚未完成。
- controller 撤销阻止后续 Hook 事件，但不会强制终止已经启动的 Hook/MCP/插件进程；
  Skill fail-closed 会暂时暂停该 controller 的全部 Skill 入口，而不是原地热替换单个插件。
- generation + 原子状态指针保证进程内失败时 active 不丢失，但没有 durable
  lifecycle journal；突发断电 exactly-once、目录 metadata/ACL 和恶意本机并发组件
  在极窄系统调用窗口内的对抗不在完成声明内。
- 真实第三方插件和干净 clone 仍必须补齐；关闭 M5 时最新提交的远端 CI/CodeQL 必须全绿。
  在这些边界关闭前 M5 保持“进行中”。
