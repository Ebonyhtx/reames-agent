# Reames Agent — 参考项目移植路线图

> 创建：2026-07-08
> 基于对 8 个参考项目 + Reames Lite 的深度源码审计

## P0 — 立即移植（阻塞性缺失）

### 1. Tool Guardrails — 循环检测（源：Hermes）
- **文件**：`F:\code-reference\Hermes\agent\tool_guardrails.py`
- **内容**：SHA-256 哈希检测 exact-repeat、tool-flailing、no-progress 三类循环
- **决策模型**：allow / warn / block / halt 四级
- **移植**：~250 行纯逻辑，Go 直译即可
- **验收**：`go test ./internal/agent/... -run Guard`

### 2. 三层压缩 L1/L2/L3（源：Reames Lite）
- **文件**：`F:\Reames-Lite\packages\core\src\reames\features\compression\pipeline.py`
- **内容**：L1(50%警告)/L2(80%裁剪)/L3(80%+摘要)+安全网(90%)+断路器
- **关键**：head=2/tail=6 保留的缓存友好压缩区域
- **移植**：~500 行 Go，扩展 `internal/agent/compact.go`
- **验收**：`go test ./internal/agent/... -run Compact`

### 3. 事件类型补全（源：Reames Lite）
- **文件**：`F:\Reames-Lite\packages\core\src\reames\api\events.py`
- **缺失事件**：`CACHE_UPDATED`、`CACHE_IMPACT_RECORDED`、`MODE_CHANGED`、`COMPRESSION_UPDATED`
- **移植**：扩展 `internal/event/event.go` 的 Kind 枚举
- **验收**：`go test ./internal/event/...`

### 4. Intent Classifier（源：AgentArk）
- **文件**：`F:\code-reference\AgentArk\src\security\classification\intent_classifier.rs`
- **内容**：12 类固定意图词汇，LLM 只输出意图标签，确定性策略引擎裁决
- **移植**：Go struct + 策略引擎 ~300 行
- **验收**：单元测试覆盖全部 12 类意图

### 5. 密钥安全存储（源：AgentArk）
- **文件**：`F:\code-reference\AgentArk\src\crypto\mod.rs`
- **内容**：Zeroize 包装、AES-256-GCM、Argon2id(64MiB/3轮)、原子写入
- **移植**：Go crypto 标准库，~200 行
- **验收**：`go test ./internal/... -run Crypto`

### 6. apply_patch 工具（源：Reames Lite）
- **文件**：`F:\Reames-Lite\packages\core\src\reames\providers\tools\builtin\apply_patch.py`
- **内容**：原子 unified diff 补丁应用，最多 100 文件，含验证
- **移植**：Go 实现 ~400 行
- **验收**：`go test ./internal/tool/builtin/... -run Patch`

### 7. web_search 工具（源：Reames Lite）
- **文件**：`F:\Reames-Lite\packages\core\src\reames\providers\tools\builtin\web_search.py`
- **内容**：搜索引擎集成
- **移植**：Go HTTP client ~300 行
- **验收**：集成测试

### 8. 梦境引擎 — 记忆去重（源：Reames Lite）
- **文件**：`F:\Reames-Lite\packages\core\src\reames\features\memory\dream_engine.py`
- **内容**：LLM 驱动的后台记忆去重（发现→审查→合并）
- **移植**：Go ~400 行
- **验收**：`go test ./internal/memory/... -run Dream`

### 9. Trust Boundary + Output Sanitization（源：AgentArk）
- **文件**：`F:\code-reference\AgentArk\src\security\boundary\trust_boundary.rs`
- **内容**：HTML 清洗、不可信输出信封 `[UNTRUSTED_source_OUTPUT]`
- **移植**：Go ~150 行
- **验收**：单元测试 + 安全测试

---

## P1 — 重要增强（按计划移植）

### 环境/基础设施
- 统一 `RuntimeRequest`/`RuntimeResponse` 分发器（源：Reames Lite client.py）
- `PlatformAdapter` 接口实现（Feishu 适配器作为第一参考实现）
- 钩子事件通配符匹配 `command:*`（源：Hermes gateway/hooks.py）

### 工具
- `goal` 工具：三重预算 + 工作笔记（源：Reames Lite goal.py）
- `delegate` / `wolfpack_spawn` / `parallel_tasks`（源：Reames Lite）
- `hooks` 管理工具（源：Reames Lite hooks.py）
- 技能生命周期管理工具（源：Reames Lite）
- `session_search` 工具（源：Reames Lite）
- `checkpoint` 工具作为 LLM 可见工具（源：Reames Lite checkpoint.py）

### 记忆
- L0-L3 分层记忆与自动晋升（源：Reames Lite memory_manager.py）
- 双策略标签路由器：标签优先 → BM25 回退（源：Reames Lite tag_router.py）
- 置信度评分 + `confidence_floor` 驱逐（源：Reames Lite extractor.py）

### 安全
- 三层沙箱模型：Native / Docker / WASM（源：AgentArk sandbox.rs）
- Replay gate：证据聚合 + 最小样本/置信度/纠正率阈值（源：AgentArk replay_gate.rs）
- 声明式安全规则 `.local.md` 格式（源：Claude Code hookify 插件）

### 设计系统（源：MiMo Code + Scream Code + Impeccable）
- Seed→12阶→260 Token 主题引擎 → Go 后端生成，CSS custom property 前端消费
- 语义 ColorPalette 接口（~30 个 token）
- OKLCH 颜色工具库：hex↔oklch、gamut 裁剪、色阶生成
- Neo Kinpaku 完整调色板作为暗色主题基础

### 桌面/TUI（源：Kimi Code）
- "Thin shell + daemon" 桌面架构
- pi-tui 差分渲染引擎（三策略 + CSI 2026 同步输出）

---

## P2 — 后续增强

- 渠道集成（DingTalk/Slack/Telegram/Discord）
- 媒体工件元数据存储
- LSP 工具作为 LLM 工具暴露
- 凭据/密钥管理（`request_secret`）
- 工作区树/搜索/读取/变更客户端 API
- 成本跟踪显示单位（CNY）
- ACP 协议适配器（Zed/JetBrains IDE 集成）
- 自进化系统完整实现（Canaries, Replay, Promotion Gates）

---

## 参考项目速查

| 项目 | 本地路径 | 核心移植价值 |
|---|---|---|
| Hermes | `F:\code-reference\Hermes` | Tool Guardrails、PlatformAdapter 接口、MemoryProvider ABC、Cron Job 模型 |
| Codex CLI | `F:\code-reference\codex` | Hook 系统（10事件+matcher）、ExecPolicy、SandboxManager 模式、MCP Transport Recipes |
| MiMo Code | `F:\code-reference\MiMo-Code` | Seed→12阶主题引擎、OKLCH 颜色工具、FTS5 查询构建器 |
| Scream Code | `F:\code-reference\scream-code` | ColorPalette 语义 token、Goal Loop、Storm Breaker |
| Impeccable | `F:\code-reference\impeccable` | Neo Kinpaku 调色板、22条设计规则、45条反模式检测器 |
| AgentArk | `F:\code-reference\AgentArk` | Intent Classifier、Zeroize 密钥存储、三层沙箱、Replay Gate |
| Claude Code | `F:\code-reference\claude-code` | plugin.json 最小 manifest、hookify 规则格式、Agent/Command 定义 |
| Kimi Code | `F:\code-reference\kimi-code` | Thin shell + daemon 桌面架构、pi-tui 差分渲染、CSI 2026 |
| Reames Lite | `F:\Reames-Lite` | 三层压缩、事件类型、apply_patch/web_search/goal 工具、梦境引擎 |

---

## 验收总命令

```bash
# 每次 P0/P1 移植后运行
go build ./...                                    # 编译
go test ./internal/... -count=1 -timeout 300s     # 全量测试
grep -rn 'reasonix\|Reasonix' --include='*.go' -l | grep -v 'reames-agent' | wc -l  # 应为 0
```
