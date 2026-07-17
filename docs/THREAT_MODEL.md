# Reames Agent 威胁模型

> 状态：描述当前代码边界，不把路线图当作已实现能力
> 更新：2026-07-17

## 范围与假设

Reames Agent 是单用户本地/自托管 Agent。模型输出、网页/工具返回、IM 消息和插件内容均视为不可信；本机用户显式安装的 MCP、Hook、LSP 和 shell 命令属于高权限扩展。`yolo`、关闭 sandbox、`serve.auth = "none"` 或 `allow_all` 都会主动降低防护，不能作为默认安全证据。

主要边界：

```text
本机用户 / Desktop / CLI
          │
          ▼
Controller ── Agent loop ── Provider API
    │              │
    │              ├─ built-in tools / shell sandbox
    │              └─ MCP / Hook / LSP 子进程
    ├─ session / checkpoint / credential state
    └─ Serve / Gateway ── 浏览器或 IM 用户
```

## 控制现状

| 领域 | 状态 | 当前实现 | 尚未覆盖或限制 |
|---|---|---|---|
| Provider 凭据 | 部分实现 | Provider key 名写入配置，值从 Reames Agent 全局 `.env` 或进程环境解析；项目 `.env` 不作为 Provider key 来源；Unix 写入权限收紧为 `0600`；有可选 AES-256-GCM 存储原语 | 进程环境仍是有效来源；加密文件使用机器属性派生密钥，不等同 OS keyring/硬件保护；日志与第三方错误的脱敏是 best-effort，需持续回归 |
| 工具审批 | 已实现核心路径 | `internal/permission` 按工具、subject、只读属性和 allow/ask/deny 规则决策；文件写入审批可携带 diff，拒绝/超时路径有不落盘测试 | `yolo`/显式 allow 会绕过交互；所有新工具、远程入口和扩展必须持续做集成覆盖，不能只依赖工具自报 `readOnly` |
| MCP reader 信任 | 已实现核心路径 | 宿主本地 `mcp-security.json` receipt 绑定 workspace、配置来源、transport、真实 executable/content 或规范化 HTTPS endpoint、args、env/header 键名、launcher exact content 以及 tool input/output schema/read-only/destructive 指纹；跨进程锁和原子写保护状态。Legacy `trusted_read_only_tools` 只迁移一次；identity drift 在进程/网络启动前阻断，capability drift 撤销变化工具，cache/lazy/Plan Mode/普通执行/只读子代理共用评估。Desktop 显式 reverify；destructive 每次 fresh-human，Auto/YOLO/Guardian 不代答 | MCP annotation 和远端 schema 仍由 server 提供，不能证明实现没有隐藏副作用；HTTPS endpoint 背后的运营内容可变化，因此 live capability 检查仍是必要的第二道门。宿主本地 state 不抵抗同机高权限篡改，也不是跨机器 portable allowlist；MCP 副作用不具备 exactly-once |
| Shell/子进程隔离 | 部分实现 | `sandbox.mode = enforce` 时 shell 使用 Linux bubblewrap、macOS Seatbelt、Windows AppContainer/低完整性 token 与 Job Object；后端不可用时 enforce 模式 fail closed。已安装包拥有的 Hook/MCP 另由 `processpolicy` 强制使用同一 OS backend、core-only wrapper env、显式 child env、workspace/state 写根、敏感 read barrier 和进程树回收，且不能申请 unconfined escape。显式 child env 不进入 wrapper argv；隐藏 helper 在隔离边界内取走并清除编码环境后才启动 child，缺失/损坏或宿主未注册 helper 时 fail closed | sandbox 可被配置为 off，零值 shell、用户手工 Hook/MCP 与 LSP 不自动隔离；平台能力不完全等价。package process 当前允许网络，且没有跨三平台统一硬 CPU/RSS 配额。helper 环境编码不是加密，不对同机高权限进程、调试器或进程转储提供独立密钥保护 |
| 不可信内容 | 部分实现 | 有 untrusted envelope、HTML 文本化和常见 token 正则脱敏；system prompt/tool schema 采用稳定前缀约束 | 不能保证模型不受 prompt injection 影响；正则无法识别所有私有凭据；工具/插件/网页内容仍必须依赖权限边界限制副作用 |
| Prompt/展示数据边界 | 已实现核心路径 | `MessagesForRequest` 在 Provider interface 前剥离 citation/edit/original 等本地 metadata；OpenAI/Anthropic wire-byte 与 Agent cache-prefix 回归覆盖该纪律。`control.TranscriptMessage` 隐藏 system、合成恢复指令、compose 控制块和 referenced-context payload；Serve/ACP history、ACP metadata title、Desktop memory suggestions/history/pagination/planner sidecar 均消费安全投影。Desktop sidecar correlation hash 与安全 replay text 标记为 `json:"-"`，不跨远程 transport；rebuild 通过 opaque history snapshot 刷新已有 system prompt，同时保留 legacy system-less transcript | 新增 Provider、传输适配器或本地 metadata 字段时仍必须加入剥离/投影合同；模型本来就需要看到的用户正文和工具结果不属于该隐藏边界 |
| HTTP Serve | 部分实现 | 支持 `none`/token/password，token 常量时间比较、密码 session HMAC、登录速率限制、JSON-only POST CSRF guard、默认无 CORS、显式单 origin CORS；版本化 command 校验与服务端 `remote` scope 阻止客户端选择 trusted submit，旧 WS submit 也不再绕过 `!shell` 限制；history 使用展示安全 transcript 而不输出 system/注入上下文；真实 WebSocket 握手有回归测试 | WebSocket `CheckOrigin` 当前放行并依赖外层鉴权；`auth=none` 依赖 loopback/same-origin 部署假设；请求体、WS frame 和全局请求速率限制仍需系统化审计 |
| IM Gateway | 部分实现 | 用户/群 allowlist、admin/approver 角色、operator 身份检查和各渠道传输适配已存在；connection/domain/chat/user/operator/message ID 只用于路由，不进入 Provider prompt。`gateway setup` 只接受常规大写 secret 环境变量名，新连接必须显式 pairing/名单/角色或有意 `allow_all`，损坏配置与缺失 access 均 fail closed；dry-run 不落盘，正式更新原子且幂等。Linux systemd user service 已用随机 token 的 loopback webhook challenge 验证 install/reinstall/restart/stop/start/uninstall，token 不进入 unit、命令输出或报告；Linux user-scope install 还会验证 unit、快照旧定义与 enabled/active 状态，并在定义恢复失败时停止后续 manager 操作 | CLI 无法从字符串本身证明一个大写值绝不是误传 secret，运维仍须使用命名明确的环境变量；当前没有通用飞书 webhook HMAC/重放验证实现；install 事务不覆盖 system scope、macOS launchd、Windows Scheduled Task 或 uninstall；WSL 证据为 `Linger=no` 的登录会话内生命周期，不证明 logout/reboot 常驻；真实飞书/QQ/微信回环需要外部应用凭据与网络环境，未验证前不得声明完成 |
| 插件与 Hook | 部分实现 | 原生 manifest schema v1 强制语义版本和精确权限；兼容 manifest 按能力推导权限。复制安装以 `sha256-tree-v1` 发布不可变 generation，状态原子选择 active/previous，新安装默认禁用，启用绑定精确 digest/权限；update/rollback/uninstall 使用 preview/planId/apply，并在状态锁内比较完整 lifecycle state token。插件 state、安装请求和 registry 索引共享可移植 ASCII 名称身份，大小写别名、尾点和 Windows 设备保留名在物化前拒绝。状态 mutation 有进程内/跨进程锁；受管 staging 从复制、摘要到发布保持目录句柄身份，创建/发布/删除使用 `os.Root`。runtime/doctor 使用 digest-before/parse/digest-after，遇到 tamper、link 漂移或 grant 不足会拒绝加载。插件 MCP owner 绑定 controller 实际加载的配置并随用户同名接管清除；更新、回滚、卸载和禁用串行化同步 rebuild、取消旧状态的迟到 startup build，并在所有 live/detached controller 断开旧插件 MCP、撤销/取消运行中旧插件 Hook、暂停旧 Skill 入口到重建/新会话。package Hook/MCP 有最小环境、严格 sandbox、独立 state/tmp、敏感读取阻断、超时/输出上限、诊断脱敏和后代回收；真实 `obra/superpowers` 固定 revision 已完成 Windows sandbox E2E。可选 registry 没有默认 endpoint/TOFU：用户级 endpoint 与带外 bootstrap root 不可被项目 TOML/`.env` 替换，官方 `go-tuf/v2` 持久验证 root 轮换、过期、rollback/freeze/mix-and-match；严格索引绑定 full commit、跨平台 raw-Git `sha256-git-tree-v1`、manifest 版本/权限和 registry provenance assertion，apply 在隔离 ambient Git 配置后重解析重算并持久化 root/provenance/attestation 证据。只读 operator audit 必须显式提供带外 root，重放连续 root 的旧/新双阈值，强制四角色 canonical key 隔离、root/targets 2-of-3 与到期窗口，并验证完整 metadata/index/attestation 字节 | 直接 GitHub 仍只有 HTTPS + commit revision；没有已运营公开 registry、生产私钥仪式或实际 compromise drill。registry GitHub source 当前必须可匿名无交互拉取；可选 attestation target 只由 TUF 认证字节，不验证 DSSE signer identity、SLSA predicate policy 或等级；签名恶意插件仍是恶意代码。operator audit 的合成 Ed25519 轮换和成功 JSON 不证明人员 quorum、HSM custody、原子 HTTPS 发布、在线 freshness monitor 或真实 compromise drill，`externalRequired` 不得从证据摘要移除。TUF 本地 cache 假设 Reames home 未被同机用户/高权限进程篡改，删除 cache 会重置已学习 rollback 状态；Windows `os.Chmod` 不等于 DACL 收紧，因此自定义共享 home 不在 owner-private 声明内。legacy 安装需重装迁移；没有跨进程 durable lifecycle journal，断电后的不可达 orphan 只可清理而不能证明 exactly-once。package process 允许网络且没有统一硬 CPU/RSS 配额；用户手工 Hook/MCP 与 LSP 仍是高权限进程。任何候选仍须以干净 clone、最新远端 CI/CodeQL 和公开发布链的实际证据为准 |
| 可写子代理隔离 | 已实现核心路径 | 产品装配中的 writer `task`/Skill/Subagent 绑定 persisted parent、独立 Git branch/worktree、workspace/ref 跨进程锁和重建后的 workspace tool registry；非 Git writer fail closed，只读 child 不分配 worktree。Child mutation 不进入父 evidence/checkpoint，父会话经 Controller preview/apply/merge/rollback/reject 才改变 source。取消/崩溃、lost/orphaned、trash/restore/purge 有持久状态。Acceptance 在 Git mutation 前写 intent；unchanged pre-state 与 completed merge 可证明恢复，rollback 要求 exact post-state | child Git worktree 不是通用 OS sandbox；MCP/LSP/memory/source live service 不继承，opaque shell/external API 不 exactly-once。Dirty source 不复制。apply 后崩溃若无法排除随后人工编辑会停在 `acceptance_interrupted` 并要求人工 Git 处理；系统刻意不以 `reset --hard` 猜测归属。同机高权限进程、用户直接编辑 worktree/repo 或 Git 元数据损坏不在信任边界内 |
| 状态与恢复 | 已实现核心路径 | session JSONL、lease/recovery 和 control persistence 边界复用同一语义。v2 runtime sidecar 持久化 Goal/Plan/Todo、continuation 安全计数、transcript anchor/revision、最新 writer epoch 的最小 root 项目检查引用和 child effect cursor。Previewable built-in writer/checkpoint restore 以 `os.Root` resolve-beneath handle 执行，checkpoint/runtime/in-flight/journal 任一写前持久化失败均阻断 child/root writer；multi-file writer 全量预览后再执行。每个 visible/synthetic turn 的 checkpoint 与 in-flight commit anchor 在冷启动时执行“完整提交则保留，否则 workspace/runtime/transcript 同步回滚”；Conversation/RewindBoth 使用 `prepared -> resources_applied` journal，checkpoint 只在资源 barrier 后退休。`AtomicWriteFile` 以 fsync + atomic replace + directory/write-through flush 发布，cross-device rename fail closed。持久 subagent/job 在 Provider、tool 与 compaction 边界保存 transcript，冷启动转为 `interrupted` 且不自动重放。所有 Goal completion 仍经过 Todo/project checks | 完整 Evidence ledger 与委派预算仍非跨进程账本；child-only bash 只可作为当前 turn 证据，不能恢复为 root proof。崩溃中的 opaque tool 可能未执行、部分执行或已完成，系统不提供 arbitrary side effect exactly-once；后台副作用仍须从 job 产物、磁盘和外部系统核验。`bash`/MCP/external API 没有逐文件 checkpoint；ACL/xattr/硬链接身份、无 lease 嵌入方单写者和跨根备份 journal 仍未关闭 |
| 进程级启动恢复 | 已实现核心路径 | 独立 Guard 在 config/i18n/boot/Provider 之前运行；带 live-PID ownership 的五分钟三次 crash-loop 账本、30 秒健康观察期、五份配置快照、repair/undo、installer failure marker 和完整安装单元 pending transaction 共用 OS 跨进程锁与原子写。自动回滚必须同时证明失败版本=`toVersion`、同安装目录、transaction identity 和全量备份 SHA-256；补偿后无法证明一致时 mixed install fail closed。pending 未清算时不能被下一更新覆盖，Windows helper 缺失与 event-log 取证失败均 fail closed。Safe Mode 不读用户/项目 TOML 或 dotenv，不恢复旧 tab/session，Desktop 只建立 recovery-only shell，`boot.Build` 拒绝 Provider/Controller/Agent 装配，并禁用 MCP/plugin/Hook/Bot/LSP/planner/Guardian/subagent/Memory Compiler 等运行面。CLI/Serve/Desktop/Gateway 只投影同一 `repair.Report` | 同机管理员可改程序或状态，不在应用层信任边界；Safe Mode 不恢复 opaque external side effect。真实断电、签名安装包、Windows installer、macOS notarized bundle 和 Linux package manager 的三平台升级失败/crash-loop 回滚仍需安装态证据；来源不明时必须人工可信重装 |
| 构建与发布 | 部分实现 | Go 依赖哈希、六目标 candidate、SHA256SUMS、三平台 Desktop candidate、CodeQL 和发布契约检查已建立；CLI updater 已锁定官方仓库和精确资产名，实际执行候选/安装后 `version`，保留 `.previous`，并以同目录锁保护自动恢复和 `upgrade --rollback`。Desktop 包现同时交付 Guard/Desktop/launcher/helper，Windows/Linux/macOS 入口默认经过 Guard，Linux/Windows helper 和 macOS bundle rollback 保留完整安装单元 | 生产发布仍禁用；公开 release 实际升级/失败回滚、CLI/Windows/macOS 工件签名、notarization、provenance attestation 和可信 updater 发布链未完成。Guard transaction 收口了应用层 crash journal，但不证明文件系统/包管理器在任意断电点的全局原子性 |

## 优先风险

1. **远程副作用**：Serve/IM 一旦暴露到非 loopback，鉴权、Origin、CSRF、角色与审批必须同时成立。
2. **扩展供应链**：权限 manifest、内容摘要、默认禁用、package-owned 进程 sandbox 和可选 TUF registry 降低了静默漂移、镜像篡改与宿主密钥泄露风险，但 registry operator/targets signer 仍能授权恶意代码，用户 MCP/Hook 和 LSP 仍能执行本机代码；生产 registry、密钥仪式和 provenance policy 未有外部证据前，只应安装固定 revision、可审计来源。
3. **凭据外泄**：Provider/IM token 可能经错误正文、工具输出、日志或第三方扩展泄露；证据脚本不得保存原始 HTTP 错误正文。
4. **沙箱误解**：package-owned Hook/MCP 已强制 OS 隔离不代表所有子进程都隔离；用户手工 Hook/MCP、LSP 与显式关闭的 shell sandbox 仍属于高权限执行。
5. **恢复一致性**：session、Goal、Todo、checkpoint、lease、备份根和版本二进制跨文件更新时必须防止旧状态复活或终态丢失；进程内回滚不得冒充断电原子性。

## 外部阻塞与可本地推进

没有真实 API key、IM 应用或云服务器时，仍可使用 localhost Provider harness、假凭据、隔离 home、原生安装包和本地反向代理完成确定性合同与失败路径。以下证据必须保持 `external-blocked`，不能用 mock 替代完成声明：

- 真实 Provider 的鉴权、计费/用量和供应商网络行为；
- 真实 IM 平台的身份、回调/WebSocket、审批与重连回环；
- 真实运营公开 registry 的人员 quorum、离线 root/targets 仪式、HSM 或等价托管、生产
  HTTPS 发布与 freshness 监控、实际轮换/compromise drill，以及独立 provenance policy；
- 公网 TLS、反向代理、安全组、linger-enabled logout/reboot 常驻，以及干净云节点上的备份/恢复和公开签名 release 升级/回滚实启；
- Windows/macOS 代码签名、Apple notarization、OIDC provenance 与公开 updater 链。

漏洞报告流程和支持边界见 [SECURITY.md](../SECURITY.md)。发布启用门槛见 [RELEASING.md](RELEASING.md)。
