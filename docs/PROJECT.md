# Reames Agent 项目说明

> 状态：当前产品方向的权威说明
>
> 更新：2026-07-17

## 一句话定位

Reames Agent 是一个以 DeepSeek Reasonix 为工程底座、面向本地与远程工作的多平台通用 Agent。编程助手是第一条成熟主线，长期能力覆盖研究、文件与数据处理、自动化、记忆、插件和多渠道协作。

## 产品形态

同一套 Agent runtime 支撑四个入口：

| 入口 | 定位 |
|---|---|
| Desktop | 主产品；完整会话、工作区、审批、变更、设置和恢复体验 |
| CLI | 终端中的高效编程与自动化入口 |
| Server/Web | 本机、局域网和云端的 HTTP/SSE 服务 |
| Gateway | 飞书、QQ、微信、Telegram 等消息渠道 |

入口只负责交互和传输。会话、Agent loop、模型、工具、权限和事件语义应逐步收敛到 `internal/control` 后面的统一运行边界。

云端部署形态见 [CLOUD_AGENT_PLAN.md](CLOUD_AGENT_PLAN.md)：目标是在自有服务器上同时支持 SSH/CLI、HTTP/SSE、飞书等 IM 通道、后台上游研究 Worker，以及隐私保护的遥测反馈闭环。

## 项目来源

- [DeepSeek Reasonix](https://github.com/esengine/DeepSeek-Reasonix) `main-v2` 是源码主上游。
- `F:\Reames-Lite` 是前身，只保留公共边界、缓存纪律、中文体验和接口契约等思想。
- `F:\code-reference` 中的其他官方项目提供机制和体验参考，不作为可直接拼接的代码集合。
- 详细来源、许可证和升级规则见 [REFERENCE_GOVERNANCE.md](REFERENCE_GOVERNANCE.md)。

## 核心原则

1. **单一运行语义**：Desktop、CLI、Server 和 Gateway 不各自实现 Agent 行为。
2. **缓存稳定**：system prompt 与 tool schema 保持稳定；UI、渠道和诊断状态不得污染模型前缀。
3. **副作用受控**：文件、命令、网络和凭据操作经过权限、沙箱、脱敏与证据记录。
4. **状态可恢复**：会话、检查点、任务和后台作业必须有清晰的持久化与恢复语义。
5. **桌面优先但不绑死桌面**：桌面是主产品，核心能力仍保持传输无关。
6. **上游人工决策**：自动发现、分类和建单；由维护者审查、移植和接受版本。
7. **证据先于完成声明**：构建、测试、真实交互或发布证据齐备后，事项才算完成。

## 当前事实

- Go Agent 内核、CLI、HTTP/SSE、IM Gateway 和 Wails/React Desktop 已有较完整实现。
- 核心、Desktop 和前端已建立本地与远端 CI 基线，并有六目标 CLI candidate、三平台 Desktop candidate 及原生安装 smoke 记录。
- M1 真实任务闭环已关闭：真实 Provider、原生会话/工作区/停止、文件审批/落盘/回退、重启恢复以及五类原生失败恢复均有分层证据。
- 24 个内置工具，具备权限、沙箱、检查点、记忆、技能、插件、定时任务、LSP 和证据账本等模块。
- M3 Desktop 日用化已关闭：关闭态/次级界面与简中/繁中词典按需拆包并受真实产物硬预算保护，模态隔离、Transcript 语义和严格 Windows UIA 可访问性 smoke 已交付。commit `68218d6` 的 CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows 全链路均通过。Windows native cold/warm 为 7.031/2.016 秒；interaction 完成 19 请求、五类失败恢复、停止和同 project/workspace/session 恢复，跳转前 marker present 但 offscreen、assistant 不在 UIA，严格 InvokePattern 定位问题 1 后 user/assistant 均 present + onscreen，`recovery_verified=true`；strict accessibility 随后实际执行并通过。三份 smoke 的 `boundary_changes=[]` 且无 errors。自动证据不等于 NVDA/Narrator 实际听感或 Windows High Contrast 人工验证。
- M6 的无界面配置与 credential-free 运维预检已形成纵向链路：`gateway setup` 支持飞书/Lark、QQ、微信，secret 只引用大写环境变量名，访问控制 fail closed，dry-run 零落盘，更新原子、幂等并保留其他连接、route、access 和 session mappings；同一隔离 home 的真实 CLI 二进制覆盖 setup → doctor → service-plan、localhost Provider 一次性 `run`/会话落盘、反馈脱敏/去重/维护草稿。WSL2 真实 systemd user manager 进一步通过带空格 binary/home/workspace 的 install、同名重装生效、status、restart、stop/start、journal、loopback webhook readiness 与 uninstall `LoadState=not-found`，但 `Linger=no`，不替代 logout/reboot、干净云节点、真实 Provider 或真实 IM 回环证据。
- M6 本地恢复基线已补齐三条确定性链路：Linux user-scope `gateway install` 会先执行 `systemd-analyze --user verify`，并对旧 unit bytes/mode、enabled/active 状态做快照和分层回滚；`backup create/verify/restore` 支持 home/state 分根、已知凭据排除、内嵌哈希自洽验证和仅恢复到不存在的新目标；CLI updater 会实际执行候选与安装后 `version` 健康检查、保留 `<executable>.previous`，并支持互斥锁保护的 `upgrade --rollback`。这些证据来自本地故障注入、race/vet 和跨平台编译，不等于跨根崩溃原子性、Windows 目标目录 ACL 保护、macOS/Windows Gateway 安装事务，也不替代干净云节点或公开签名 release 的实际演练。
- M4 Agent 可靠性已按路线图门槛关闭：所有 Goal completion 统一通过 Todo/project checks，v2 sidecar 持久化 Goal/Plan/Todo、最小 root 项目检查引用和 child journal cursor；委派树共享预算，可写 child receipt/checkpoint 归并给祖先，持久 subagent 在 Provider/tool/compaction 边界保存 transcript 并以 `interrupted` + `continue_from` 显式恢复。Previewable built-in writer/checkpoint restore 使用 `os.Root` resolve-beneath I/O；每个 visible/synthetic turn 都有独立恢复 checkpoint，in-flight commit anchor 让冷启动在“完整提交则保留、否则 workspace/runtime/transcript 一起回滚”之间 fail closed。Conversation/RewindBoth 另有 `prepared -> resources_applied` journal，checkpoint 只在资源提交后退休。`AtomicWriteFile` 已移除 Windows 原地复制降级，跨设备 rename fail closed，并补 write-through/父目录同步。该完成声明严格限于可预览文件 writer 与会话本地资源：完整 evidence/预算仍非跨进程账本，child-only bash 不是 durable root proof，shell/MCP/external API 和后台 opaque side effect 不具备逐文件门禁或 exactly-once，ACL/xattr/硬链接身份也不恢复。
- M5 插件生命周期信任机制已关闭所有可由仓库、clean clone 和 CI/CodeQL 验证的事项：原生 schema v1、精确权限、不可变 generation、默认禁用、`preview/planId/apply`、跨进程状态锁和 `os.Root` 受管路径已有故障注入与完整生命周期测试。Desktop、CLI、Bot、Serve/event wire 和 ACP 共用 fresh-human 结构化审批；generation 变化或禁用会原子阻止新 work 起跑，串行 rebuild，并撤销旧 MCP/Hook/Skill runtime。package-owned Hook/MCP 使用最小环境、独立 state/tmp、严格 OS sandbox、敏感读取阻断和进程树回收；真实 `obra/superpowers@d72560e462a74e10d161b7f993d5fc3282bfa1e2` 已完成 Windows sandbox E2E。commit `13016c6` 加入无默认 endpoint/TOFU 的官方 `go-tuf/v2` registry client，绑定 full commit、canonical tree digest、manifest 权限和 provenance assertion。commit `9295f8b` 又加入显式带外 root 的只读生产策略审计，验证连续 root 旧/新双阈值、角色 key 隔离、到期窗口和完整 metadata/index/attestation 字节，并在成功报告中保留人员仪式、HSM、endpoint/monitor 与 DSSE/SLSA policy 等 `externalRequired`。该提交的 clean clone、本地全量门禁、普通 CI `29510215514` 8/8 与 CodeQL `29510215449` 3/3 全绿，Node.js 24 action majors 的远端日志不再出现 Node.js 20 弃用告警。M5 唯一未关闭项保持 `external-blocked`：直接 GitHub 未签名，以及真实运营公开 registry 的生产 endpoint、人员/HSM 密钥仪式、实际轮换/compromise drill 和独立 DSSE/SLSA policy verifier。package process 允许网络、跨平台硬 CPU/RSS 配额不统一仍是明确安全限制，但不以 mock 冒充生产 registry 证据，也不重新打开已验收的 M5 仓库内合同。
- 2026-07-17 Reasonix 再同步已采用 MCP schema/凭据/Provider 加固，并关闭 identity-bound MCP trust P0、writer worktree P1 与 offline Guard/Safe Mode P2。独立 Guard 在普通 runtime 之前运行，使用五分钟三次失败的进程所有权启动账本、30 秒健康观察期、配置健康快照和完整安装单元 pending transaction；自动回滚必须同时满足 crash 归因、版本、同安装目录与全量备份 SHA-256，歧义或 mixed install fail closed。Safe Mode 不读取用户/项目 TOML 或 dotenv，不启动 Provider、MCP、插件、Hook、Bot、LSP、planner、Guardian、subagent 或 Memory Compiler。CLI/Serve/Desktop/Gateway 共享 `repair.Report`；三平台 Desktop 包均通过 Guard 启动，Gateway service 在加载运行时前执行同一 credential-free preflight。
- P3 Desktop Recovery Center 的仓库内实现和本地 Windows 安装态证据已收口：普通模式与 recovery-only Safe Mode 都只投影同一 `repair.Report`，所有修复经 `repair.ExecuteAction -> control.Controller.RunRecoveryAction -> Desktop RunRecoveryAction` 受控执行；支持配置隔离、快照恢复、精确 undo、已验证更新回滚、tabs/projects/window/zoom 派生状态重建和插件全量禁用。界面按需加载，确认/执行/反馈留在独立 chunk，路径、密钥和诊断文本在 Wails 边界脱敏，并以请求序号保证最后操作优先。三平台 candidate 已接入安装后 recovery smoke；最新本地 Windows Wails Desktop/Guard 真实 smoke 通过。远端 Linux/macOS/Windows candidate 尚待本批 push 后运行，真实签名/notarization、公开 release 升级失败与断电点回滚保持 `external-blocked`。
- Reasonix `dae65e25` 的会话可靠性机制已按 Reames 架构吸收：非成功 HTTP body 有硬读取边界；verified save 的 snapshot no-op 仍以 transcript/event-log 文件戳和 ledger revision/digest 做磁盘 fencing；损坏 event-log 尾部只有在取证 sidecar 成功落盘后才截断；Desktop navigation 以 monotonic epoch 保证最后点击优先。最新 `d3cfa5c2` 的 DeepSeek reasoning-only `finish_reason=stop` 又以显式 Provider capability 吸收，避免模型已结束后重复昂贵推理，同时不削弱其他网关的空答案重试。Reasonix `7f00d2c2` Theme Pack V2 已完成机制审查，但不复制其品牌、图片、marketplace 或 1.36 万行实现；下一内部方向是在 P3 远端收口后推进 P4“受控 Theme Pack”，先实现不可执行 manifest、semantic token 白名单、ZIP/path/symlink/图片限制、内容寻址资产与原子存储，再做按需 Gallery 和可撤销预览。Reasonix 删除 Memory Compiler 的产品取舍仍不直接跟随。
- 参考项目最新增量已按锁文件人工接受：Kimi 的 Auto/YOLO 文案准确性已转化为三语权限契约测试；Hermes 的 session-state 单一投影、profile prewarm 和 best-effort stream fence、Codex 的集中 MCP runtime、MiMo 的 CLI-only 演示脚本路径均只作为架构回归信号，不引入 Python/Electron/Rust 或第二套 runtime。Reasonix 新增的多套生产 release workflow 不继承；Reames 反而增加全 workflow 发布写权限/动作棘轮，继续只允许无 secrets、`contents: read` 的候选构建。外部风险仍是公开 registry 运营、干净云节点 logout/reboot、真实 IM 回环和生产签名链。
- 继承自早期迁移的 Hermes/Python runtime、Electron/TUI、旧 plugins/tests/package 元数据以及 `site/`、`workers/` 已在完成运行引用和替代实现审计后从当前树删除；参考机制只保留在 Git 历史和 `F:\code-reference`，不得重新整套 vendor。

代码与测试驱动的初始接管结论见 [audits/2026-07-09-takeover.md](audits/2026-07-09-takeover.md)。

## 近期产品标准

桌面端应让用户完成一个完整任务闭环：

```text
创建/恢复会话
→ 选择工作区和模型
→ 提交任务与附件
→ 查看推理、工具进度和文件变更
→ 批准或拒绝副作用
→ 停止、继续、回退或恢复
→ 获得带证据的结果
```

主界面使用中文优先、低噪音的桌面产品语言。工程术语、迁移阶段和上游名称不应进入普通用户主流程。

## 明确不做

- 不自动合并或自动升级上游源码。
- 不用旧 Reames Lite 的 Python provider/cache 覆盖 Reasonix 现有管线。
- 不把桌面端做成 CLI 日志壳或工程状态仪表盘。
- 不为追求“全能”而复制整套参考项目。
- 不在没有行为测试或真实验证时宣称功能已经完成。

## 文档治理

- 产品方向只更新本文。
- 优先级、里程碑和验收只更新 [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)。
- 架构事实更新 [ARCHITECTURE.md](ARCHITECTURE.md)。
- 上游来源和接受规则更新 [REFERENCE_GOVERNANCE.md](REFERENCE_GOVERNANCE.md)。
- 一次性调查、审计和验证记录放在 `docs/audits/`，不得冒充当前计划。
- 已被替代或无法反映当前代码的文档直接删除；Git 历史承担归档职责。
