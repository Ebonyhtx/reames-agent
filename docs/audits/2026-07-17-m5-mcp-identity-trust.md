# 2026-07-17 M5 MCP 身份绑定信任审计

## 结论

本批关闭 `docs/DEVELOPMENT_PLAN.md` 中由 Reasonix
`3212786e`..`60fc21f9` 暴露的 M5 P0：用户配置、项目 `.mcp.json`、legacy JSON、
session 注入和 plugin package 提供的 MCP 不再仅凭 raw tool name 获得长期 reader authority。

Reames 采用的是按现有 `control`、`plugin.Host`、cache/lazy 和权限模型重构的实现，不合并
Reasonix 分支，也不引入其品牌、发布、遥测或依赖体系。授权事实保存在宿主本地
`<REAMES_AGENT_HOME>/mcp-security.json`；用户配置中的 `trusted_read_only_tools` 只作为一次性
兼容迁移输入。

## 信任模型

### Server identity

Receipt identity 绑定：

- workspace fingerprint 与配置来源类别：`toml`、`mcp_json`、`legacy_json`、
  `plugin_package:<owner>`、`session`；
- transport；
- stdio 的真实 executable 路径与 SHA-256；
- interpreter 参数中存在的 script/archive 文件路径与 SHA-256；
- args、工作目录、package owner/root；
- env/header 键名，不保存凭据值；
- HTTP URL 的 scheme/host/port/path/query 结构；userinfo 和 credential-like query value
  规范化为固定脱敏值，因此 token 轮换不改变 identity；
- mutable launcher 的 locator、exact resolved version 和 content/integrity digest。

有 receipt 时，identity drift 在 `newTransport` 之前阻断，因此不会先启动子进程或建立网络连接。
Desktop 将这类失败投影为 `identityChanged`，普通 reconnect 与 bulk retry 不得绕过；恢复入口是
显式 **Reverify identity**。连接仍存活的 capability drift 会显示 changed tool 和
**Reverify trust**，用于清除已删除或变成 writer/destructive 的旧 selection。详情视图同时显示
trust-store error，避免把持久化故障误判为普通连接错误。identity/capability reverify 保留原 receipt
的 session/workspace scope，不会把一次 session trust 静默升级为 workspace trust。

### Tool capability

每个 tool receipt 绑定 raw/model name、input/output schema、read-only 和 destructive 状态。
Schema fingerprint 忽略 `description`、`title`、`examples`、`$comment` 等展示字段，但保留影响
输入/输出和安全语义的结构。

Capability drift 的处理：

- reader 变 writer/destructive：立即失去 reader authority；
- schema 或模型可见名变化：该 tool 失去 authority；
- tool 被删除：记录 `removed` drift，显式 reverify 时从 selected readers 中安全移除；
- 新增 tool：默认未授权；
- 未变化且仍是非 destructive reader 的 tool：可继续使用；
- lazy/cache 命中后，live handshake 若与 cached trust snapshot 不一致，会在 `tools/call` 前阻断，
  不把旧 adapter 的 reader 标记带入 dispatch；若 cached reader 在 live handshake 中变为 destructive，
  首次调用也在 dispatch 前 fail closed；lazy 安全状态由并发锁保护。

### Legacy migration and revocation

`trusted_read_only_tools` 在没有 receipt 且没有 import marker 时，只能在一次成功 live handshake 后
导入实际存在、read-only 且非 destructive 的 raw names。随后写入 import marker：

- Desktop 的 trust/untrust 与 Plan Mode 的 “always allow” 只更新 `mcp-security.json`；
- 不改写 `reames-agent.toml`、全局 `config.toml` 或项目 `.mcp.json`；
- 显式 workspace trust/untrust 会清除同一身份的旧 session receipt，避免低层撤权被高优先级旧授权遮蔽；
- 首次 legacy receipt 与 import marker 在一次原子写中落盘，进程崩溃不会留下“receipt 已导入但 marker
  未写入”或反向的半状态；
- receipt 被撤销后，旧 raw-name list 不会再次授权；
- receipt 保存失败会明确失败，不再降级为修改 legacy 配置。

## Mutable launcher lock

Workspace trust 对 `npx`、`bunx`、`uvx` 和 `git+https` launcher 执行只读 preflight：

- npm：解析 exact version 与 `dist.integrity`；
- PyPI：解析 exact version 与发布文件 SHA-256 集合；
- Git：ref 解析为完整 commit；
- 即使配置已写 exact npm/PyPI version，仍解析并绑定 package content/integrity；
- 所有可变 launcher 都必须在授予 workspace trust 前绑定内容摘要；只固定版本字符串不够；
- Git launcher 只允许 `git+https`，`git+ssh` 在联网前拒绝；
- receipt 保存后，重放加入 offline/no-install 参数；identity 计算会消除仅由宿主注入的 offline flag，
  但保留 exact locator 和 launcher digest，因此 preflight 与后续启动身份一致。

Persistent remote trust 只允许 HTTPS；HTTP 测试端点只能使用 session trust。

## Approval semantics

- 未有匹配 receipt 的 `readOnlyHint` 在普通执行中按 writer 审批；Plan Mode 可发起一次 fresh reader
  trust prompt。
- `auto`、`yolo`、Guardian、已批准计划窗口和记忆 permission rule 不能代答 MCP reader trust。
- `destructiveHint` 永远不能进入 reader receipt，也不能在 Plan Mode 进入 reader-trust 通道；每次调用
  都忽略旧 session grant 并必须经过 `FreshHumanApprovalGate`。没有交互式 fresh-human host 时 fail closed。
- Desktop bulk/per-tool trust 只显示给 schema 有效、声明 read-only 且非 destructive 的工具。

## Shared Host 与多标签页

Desktop 同一 workspace 的多个 controller tab 共享一个 `plugin.Host`，但各自拥有 tool registry。
本批给 Host 增加带引用计数的 registry attachment：

- live add/reconnect 向所有未 suspend 的 registry 发布新 adapter；
- remove/reverify 从所有 registry 清理旧 prefix；
- attach 与并发 remove 在同一同步边界内发布/清理 adapter，不留下新 registry 接到 stale adapter 的窗口；
- 单 tab disable 仍使用 registry `SuspendPrefix`，不移除共享 client，也不会被 sibling reconnect
  意外恢复；
- controller close 会 detach registry，避免共享 Host 长期持有已销毁 tab。

这关闭了“活动 tab 重验身份后，兄弟 tab 仍保留指向已关闭 client 的 stale adapter”窗口。

## 状态文件保护

`mcp-security.json` 使用跨进程文件锁与原子写入。可执行文件与已识别 script/archive 使用流式
SHA-256；参数指向的受识别文件无法读取或计算摘要时 fail closed。Boot 在文件尚未创建前就把目标状态路径加入
模型文件读取与 Bash sandbox 的敏感拒绝列表；该文件不是可复制到其他机器的 portable allowlist。

## 主要测试证据

- `internal/mcptrust`：identity/capability fingerprint、drift、session/workspace precedence/scope 保持、
  legacy 原子导入、launcher lock、并发持久化与跨进程锁；
- `internal/plugin`：pre-spawn drift block、credential URL rotation、script hash、cached/live drift、
  exact launcher content lock、offline replay identity、persistent HTTPS 限制；
- `internal/agent` / `internal/control`：untrusted reader 的 writer posture、Plan Mode fresh trust、
  destructive 不进入 reader 通道且每次 fresh-human，Auto/YOLO/Guardian/旧 session grant 不代答；
- `desktop`：receipt 不改写 `.mcp.json`、identity drift reverify、selected reader 保留、共享 Host
  sibling registry 刷新；
- `desktop/frontend`：identity drift 显示 reverify、bulk retry 排除 drift、writer/destructive/schema-invalid
  工具不显示 reader-trust action。

完成声明仍以 root、Desktop、frontend、发布合同、六目标交叉编译、clean clone 与远端
CI/CodeQL 为准。

## 剩余边界与下一步

本批不声明：

- HTTPS endpoint 背后的运营方内容永不变化；live capability drift 仍是必要的第二道门；
- MCP tool side effect exactly-once；reader/destructive annotation 只是信任输入，不是远端行为证明；
- 真实公开 plugin registry、HSM/人员仪式、生产签名 release 或跨平台统一硬 CPU/RSS 配额完成；
- 可写 child 已有并发 workspace 隔离。

下一内部优先级转为 P1：在现有 `task`/Skill/Subagent、durable child effect journal 和 checkpoint
之上增加 workspace lease、独立 worktree、取消/崩溃回收与父会话交付，不建立第二套 Agent runtime。

## 验证结果

工作树本地门禁：

| 门禁 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `go vet ./...` | 通过；先捕获并修复 `mcpManager` 构造时复制 `sync.Mutex` 的问题 |
| `go test ./internal/... -count=1 -timeout 300s` | 全部通过 |
| `go test -race ./internal/mcptrust ./internal/plugin ./internal/control -count=1 -timeout 600s` | 全部通过 |
| Desktop `go build ./...` / `go vet ./...` / `go test . -count=1 -timeout 300s` | 全部通过；最终完整测试约 212 秒 |
| Frontend `corepack pnpm test:all` | 通过，包含新增 identity/capability drift、destructive 与 writer trust 合同 |
| Frontend `corepack pnpm build` | 通过；bundle budget 通过；工作树 initial/localized initial JS 为 873,435 / 992,288 B，clean clone 为 873,447 / 992,300 B |
| `python scripts/check_public_readiness.py` | 通过 |
| `python scripts/check_release_contracts.py` | 通过 |
| Upstream Watch unit/Issue/deep scan | 通过；Reasonix up-to-date，Hermes/Codex 增量完成分类并接受 reviewed SHA，最终 `changed_count=0` |
| 六目标 `CGO_ENABLED=0` CLI 交叉编译 | linux/darwin/windows × amd64/arm64 全部通过，产物写入系统临时目录 |
| 长历史 privacy-safe benchmark | 10,000 turns / 30,000 messages：convert 6.32 ms、reducer 9.26 ms、transcript compute 15.23 ms（单次本机样本，不作为跨机器 SLA） |

独立 clean clone `dbf323a0eddd56807ddd930391a71da5d3059135` 另行通过：root build/vet/internal
全测、Desktop build/vet/full test、前端 frozen-lock 安装 + `test:all` + production build、public/release
合同、Upstream Watch unit/Issue/final scan、工具契约以及 linux/darwin/windows × amd64/arm64 六目标
交叉编译；验证后 tracked worktree 保持干净。远端 CI/CodeQL 仍必须在最终 amend/push 后实际通过，
本地或 clean-clone 表格不提前代替该证据。

Wails 原生 smoke 已确认当前构建可启动，API key onboarding 提供“稍后设置”，并非强制输入 key。
本轮原生窗口中的 MCP 按钮点击被人工按 `Esc` 中止，因此不声称完成该按钮的原生交互验证；
对应 **Reverify trust** / **Reverify identity** 点击路径已由前端 JSDOM 回归测试实际执行覆盖。
