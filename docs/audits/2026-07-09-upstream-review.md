# 上游更新初审（2026-07-09）

## 更新结果

| 上游 | 更新前 | 更新后 | 新提交 |
|---|---:|---:|---:|
| DeepSeek Reasonix | `07c65c22` | `7cbc435d` | 133 |
| Hermes | `1c473bc6a` | `88a58ff13` | 163 |
| OpenAI Codex | `f6e251c3ac` | `dc23c7bcc8` | 71 |
| MiMo Code | `ba87f5d` | `9fb861a` | 23 |
| Impeccable | `7190295f` | `3e38e595` | 20 |
| Scream Code | `946eda6` | `e807673` | 8 |
| AgentArk | `4249c32` | `4249c32` | 0 |
| Claude Code | `7930e1c` | `be02c39` | 3 |
| Kimi Code | `f30781bb` | `735922c2` | 11 |

Reames Lite 的工作树包含未跟踪调研产物，未更新。

## Reasonix 优先候选

### P0：安全补丁族

当前 Reames Agent 的 `internal/trust.RedactSecrets` 只覆盖少量固定 token 格式；Reasonix 新增的 `internal/secrets` 进一步覆盖：

- 基于变量名和 `KEY=value` 的凭据识别；
- Authorization scheme；
- OpenAI、GitHub、Slack、AWS、JWT 等 token；
- tool output、持久化会话和后台工件；
- 可选的子进程环境变量过滤；
- 敏感文件读取保护。

同一批次还新增：

- `internal/doctor/session_redact.go`：历史会话/诊断归档脱敏；
- `internal/tool/configwrite.go`：配置写入必须由新鲜人工决定；
- `internal/tool/builtin/managed_config.go`：受管理配置文件写入保护。

建议将这组能力作为一个安全批次整体审查，避免只复制正则而遗漏持久化和审批链路。

### P1：ACP 与客户端能力

新增 ACP client I/O、客户端文件系统/终端能力、location、plan 和 session mode。Reames Agent 已有 ACP 包，但缺少这些新文件，应先做协议差异测试，再决定是否完整跟随。

### P1：桌面稳定性与交互

候选包括：

- 手动滚动意图与 tail-follow 修复；
- Context Window Ring；
- 恢复副本历史展示；
- Composer 运行条和上下文菜单；
- 审批、AskCard、PromptShelf 的细节修复。

这些适合在 Wails 真实点击验证建立后分批吸收，不应直接覆盖当前 Reames 中文与品牌改造。

## 其他参考信号

- Hermes：桌面 Electron 主进程 TypeScript 化、trace upload、memory/prompt 组织。
- Codex：可靠消息、线程 rollout 持久化、Provider HTTP client factory 和审批演进。
- MiMo Code：文件 Hook 热更新与多 system message 编排。
- Impeccable：新增 Web/iOS/Android/adaptive 平台设计轴。
- Scream Code：强化“每频道单会话”纪律，移除旧迁移胶水。
- Kimi Code：状态感知浏览器通知和 Provider 鉴权错误显式化。

这些目前作为设计信号记录，不直接进入 Reames 主线。
