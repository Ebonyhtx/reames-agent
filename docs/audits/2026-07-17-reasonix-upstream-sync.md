# 2026-07-17 Reasonix 与参考项目增量同步审计

## 结论

本轮将 DeepSeek Reasonix `main-v2` 从 Reames 已审查点
`07c65c22226e4886004215168230e1e1edad734b` 审查到
`9b54b9f8937b9878d9052833bff4ab99ba7638de`。该区间包含 525 个提交、392 个非 merge
提交，覆盖 `desktop-v1.17.10` 至 `desktop-v1.17.14` 附近的安全、Provider、MCP、恢复、
Desktop 和交付机制变化。

本轮没有合并上游分支，也没有复制 Reasonix 的发布、品牌、遥测或依赖体系。实际采用的是
经过 Reames 缺口证明、按现有 `control`、cache-first 和 `processpolicy` 边界重构的纵向闭环：

- MCP tool schema 严格验证、无效工具隔离和 Provider 400/422 定位；
- 保存凭据对子进程环境和模型可读文件路径的隔离；
- MCP schema cache 身份不再绑定凭据值；
- Anthropic 兼容流的累计 usage 合并；
- 仅对官方 MiMo 端点进行 draft 2020-12 tuple schema 转换；
- CLI 与 Desktop 显示具体 MCP schema 故障，不让单个坏工具拖垮整个 server；
- Windows 上游扫描显式使用 UTF-8，避免中文提交标题触发 GBK 解码崩溃。

下一阶段不继续横向堆 UI 功能。内部优先级确定为：

1. MCP server/tool 的身份绑定 trust receipt 与可变 launcher 精确锁定；
2. 可写子代理的 workspace lease、独立 worktree、回收与交付投影；
3. 在现有 updater/recovery 事务之上评估离线 Guard 与 Safe Mode；
4. 最后再吸收历史消息时间、外部打开器、Subagent profile 等体验项。

## 上游证据

Reasonix 本地镜像：`F:\code-reference\DeepSeek-Reasonix`

```text
reviewed: 07c65c22226e4886004215168230e1e1edad734b
latest:   9b54b9f8937b9878d9052833bff4ab99ba7638de
commits:  525 total / 392 non-merge
tag:      desktop-v1.17.14 / v1.17.14 vicinity
```

与本轮直接实现相关的上游提交包括：

| 提交 | 上游变化 | Reames 决策 |
|---|---|---|
| `9be6e1e8` | MCP schema 根类型必须为 object | 采用，并扩展为缺失根类型规范化和严格编译 |
| `ed113b49` | 隔离无效 MCP 工具 | 采用；坏工具不再阻断同 server 的其他工具 |
| `bd149dc8` | schema cache identity 排除凭据值 | 采用；env/header 只绑定排序后的键名 |
| `f5179b4d` / `fad0933b` | 保存凭据不进入工具子进程或读取路径 | 按 `processpolicy` 重构采用，覆盖 Bash/Hook/LSP/MCP/探测/外部 helper |
| `f5179b4d` | Anthropic 兼容流累计 usage | 采用，按非负累计计数取最大值防重复计数 |
| `db44434d`..`18d4ed9e` | MiMo 专用 JSON Schema 方言转换 | 采用；未知 dialect 和非 MiMo Provider 保持原始字节 |
| `3212786e`..`60fc21f9` | MCP identity、trust receipt、launcher pin | 延后为下一 P0；现有 raw-name trust 缺少 identity 绑定 |
| `03b39a65` | workspace lease、worktree、并发 writer 隔离 | 延后为下一 P1；Reames 只有 session lease 和 child-effect journal，没有可写 child worktree |
| `3baf0b3c` / `1b9ff514` | 离线 Guard、Safe Mode、恢复锁 | 暂缓；先完成 identity trust 和 worktree 隔离，再接入现有 updater/recovery 事务 |
| `656e983d` 等 | CLI/Desktop Subagent profiles | 暂缓；Reames 已有 model/effort/skill profile 基础，先避免形成第二套配置模型 |
| `9b54b9f8` / `0afcea40` | 外部打开器、面板偏好、历史消息时间 | 体验候选；不高于当前安全与并发 writer 风险 |

## 已采用实现

### 1. MCP schema 安全与故障隔离

`internal/provider` 现在会：

- 将缺失根 `type` 的 MCP 参数 schema 明确为 object；
- 通过 `jsonschema/v6` 编译 schema；
- 禁止编译器从 `file://` 或网络 URL 解析外部 `$ref`；
- 拒绝非 object 根 schema；
- 将 Provider 的 400/422 schema 错误映射回具体 MCP server/tool。

`internal/plugin` 在握手与旧 cache 读取时逐工具净化：无效工具进入 server 状态诊断，其他合法工具继续可用。
CLI 与 Desktop 的 capabilities 视图展示 schema error，并禁止把隔离工具记为可信只读。

### 2. 凭据环境与文件隔离

`Config.CredentialEnvNames()` 返回当前配置引用的 Provider/Serve/Bot key，以及全局凭据文件中已经不再
被配置引用的旧 key。`processpolicy` 保存进程生命周期并集，避免多个并发工作区互相缩小过滤集合。

Boot 在启动任何工具进程前注册凭据 key。下列子进程使用过滤后的 ambient env，再叠加自身显式 env：

- MCP stdio 与登录 shell PATH probe；
- Bash、ripgrep、环境探测；
- Hook 与 LSP；
- Bot 项目检索、附件/PDF helper、通知 helper；
- cc-switch 导入、CLI git status、受限 install-source git。

Reames 全局 `.env` 在存在时同时加入模型读取和 Bash OS sandbox 的拒绝路径；项目自己的 `.env` 不受此
规则影响，维持既有工作区行为。真实 child-process 测试证明注册凭据不可见，而 MCP/LSP/Hook 所需的
显式变量仍可受控回注。

### 3. Provider 与 cache 兼容

- `SpecFingerprint` 仍绑定 transport、command、URL、args、目录、package policy、env/header 键名和
  read-only trust，但不再绑定 env/header 值。密钥轮换不失效 schema cache，键名或权限变化仍失效。
- Anthropic SSE usage 会合并 `message_start` 与 `message_delta` 的所有累计字段，兼容 LongCat 等把完整
  usage 放在最终 delta 的网关。
- MiMo 官方域名才转换旧 tuple `items: []` 为 2020-12 `prefixItems`；未知 `$schema` resource 不改写，
  非 MiMo 请求保持输入 bytes 不变，避免影响其他 Provider 的 prompt/tool cache。

## 已有等价或更强机制

下列上游变化没有重复移植：

- Reames 已有 session lease、跨进程 session removal guard、runtime sidecar revision、原子替换、
  visible/synthetic turn commit anchor 和 Rewind journal；因此不复制 Reasonix 的会话恢复实现。
- Reames 已有结构化 `control.Command`、`eventwire`、展示安全 transcript 和五入口依赖棘轮；不回退到
  Desktop/CLI 各自实现 runtime 语义。
- Reames 已有共享委派预算、持久化 Subagent transcript、child effect journal 和 checkpoint 回收；
  MiMo/Kimi 的基础 Goal/后台 child 生命周期只作为测试与交互参考。
- Reames 已有签名 plugin registry、不可变 generation、package-owned Hook/MCP 严格沙箱和运行时撤销；
  Reasonix MCP catalog 不直接替换该供应链，但其 user-MCP identity receipt 是明确缺口。

## 明确暂缓或拒绝

- 不采用 Reasonix 自有 GitHub Release、R2、npm/Homebrew、crash worker、遥测 endpoint 和品牌资产。
- 不跟随 Reasonix 将 Windows native sandbox 整体退役；Reames 的 AppContainer/package sandbox 已有
  独立测试和交付边界，除非出现本项目证据证明应替换。
- 不在本批提升到 Go 1.26；项目继续保持 Go 1.25+ 与既有 CI/交叉编译合同。
- 不复制 Creation/classic 主题、侧栏和 Composer 的整套 UI 变化；仅吸收能证明 Reames 缺口的交互。
- 不在缺少真实生产 registry、HSM/人员仪式和公开签名 release 时宣称供应链或恢复已生产完成。

## 其他参考项目增量

本轮安全快进了干净的本地参考仓库，并审查锁定基线后的提交主题和相关源码：

| 参考项目 | 新信号 | 决策 |
|---|---|---|
| Hermes | profile-scoped Gateway 凭据、重复状态修复、stale backend exit、并发 git status | 记录为 M6 多 profile/channel 运维候选；当前不移植 Python/Electron runtime |
| OpenAI Codex | session state/I/O 分离、跨会话 MCP catalog cache/opt-out、并发 stdio、cache-write usage | Reames 已部分等价；将 cache opt-out 和 session world-state 刷新列为后续诊断候选 |
| MiMo Code | worktree 原子创建/回收、child liveness/stall、跨进程 grant 继承、fan-in | 与 Reasonix workspace lease 合并为同一个 P1，不新增第二套 orchestrator |
| Impeccable | platform design axis、light-mode 对比度、blocker-age sheriff | 设计 lint 候选；当前主题/对比度合同已存在，不新增产品里程碑 |
| Scream Code | session switch 清屏和单 channel 单 session 纪律 | Reames 已有 tab/session epoch 和 scoped reducer 测试，记为回归用例信号 |
| AgentArk | 本区间只有文档链接修复 | 无机制变化 |
| Claude Code | 本区间为 changelog/feed 更新 | 只保留版本信号 |
| Kimi Code | interrupted tool-call closure、Goal deadline/continuation、transport facade | 对照 Reames M4 恢复测试；transport facade 不替代现有 control 边界 |

## 后续门槛

### P0：identity-bound MCP trust

完成前不得把 `trusted_read_only_tools` 视为跨配置变化的长期授权。目标证据：

1. receipt 绑定 canonical transport、真实 executable/hash 或规范化 HTTPS endpoint、args、env/header 键名、
   package/launcher digest 与配置来源；
2. tool receipt 绑定 raw/model name、input/output schema、read-only/destructive 语义；
3. identity 或 capability drift 自动失去 reader authority，要求 fresh-human 复核；
4. 可变 npm/npx/uvx/git launcher 固定 exact version/content，凭据轮换不改变 identity；
5. cache、lazy connect、plan mode、read-only subagent 和普通执行共享同一评估结果。

### P1：可写 child workspace isolation

目标不是新增一个后台任务系统，而是给现有 `task`/Skill/Subagent 补齐：workspace lease、独立 worktree、
可见的 branch/worktree 身份、取消与崩溃回收、父会话交付/合并策略，以及 Windows 可移植锁测试。

### P2：离线恢复入口

只有在 P0/P1 关闭后，再评估独立 Guard、Safe Mode 工具面、crash-loop 计数和 updater rollback lock；
必须复用 Reames 现有 recovery transaction，不能建立第二套不相容的恢复状态机。

## 验证

本批在提交前至少执行：

```powershell
python -m unittest scripts.test_check_upstreams -v
python scripts/check_upstreams.py --deep --out-dir artifacts/upstream-watch
go test ./internal/provider/... ./internal/plugin/... ./internal/config ./internal/processpolicy ./internal/boot -count=1
go test ./internal/tool/builtin ./internal/environment ./internal/hook ./internal/lsp ./internal/bot ./internal/control ./internal/installsource ./internal/cli -count=1
Push-Location desktop; go test . -count=1; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
```

`artifacts/upstream-watch/` 是本地审查产物，不纳入 Git；正式接受点由
`docs/upstreams/upstreams.lock.json` 保存。
