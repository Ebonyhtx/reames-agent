# P0 移植项 — 源码验证报告

> 创建：2026-07-08
> 每项均对照 Reames Agent 实际 Go 源码验证

## 验证结果总览

| # | 功能 | 原判 | 实际 | 说明 |
|---|------|------|------|------|
| 1 | Tool Guardrails 循环检测 | P0 移植 | ❌ 不需要 | **已存在且更完善** |
| 2 | 三层压缩 L1/L2/L3 | P0 移植 | ⚠️ 部分 | 换了个架构，功能等价 |
| 3 | 事件类型补全 | P0 移植 | ⚠️ 部分 | 20 种已有，仅缺 4 种 |
| 4 | Intent Classifier | P0 移植 | ⚠️ 部分 | 有 task/chat 分类器，缺安全意图分类 |
| 5 | 密钥安全存储 | P0 移植 | ❌ 确实缺失 | .env 明文，无 zeroize/AES/Argon2 |
| 6 | apply_patch 工具 | P0 移植 | ❌ Go 缺失 | Python 层有，Go 核心无 |
| 7 | web_search 工具 | P0 移植 | ❌ Go 缺失 | Python 插件有，Go 核心无 |
| 8 | 梦境引擎记忆去重 | P0 移植 | ⚠️ 部分 | memorycompiler 存在，规则驱动非 LLM 驱动 |
| 9 | Trust Boundary 输出清洗 | P0 移植 | ❌ 确实缺失 | 无 HTML 清洗，无输出信封 |

## 逐项详析

### 1. Tool Guardrails — ❌ 不需要移植

**Reames Agent 已有（agent.go + guardian.go）：**

| 机制 | 位置 | 功能 |
|------|------|------|
| Storm Breaker | `agent.go:2110-2242` | 签名检测 (tool+error) + 连败检测，阈值 3 |
| Repeat Success Guard | `agent.go:2548-2597` | 同参数写工具成功 >2 次即阻断 |
| Loop Guard | `agent.go:1339-1364` | 循环触发时豁免 final readiness |
| Stale Anchor Block | `agent.go:2562-2577` | 文件被修改后禁止基于旧 anchor 编辑 |
| Guardian Circuit Breaker | `guardian.go:345-376` | 连续拒绝 3 次或滑动窗口 10/50 即熔断 |
| MaxSteps 最终防线 | `agent.go:2110` | 硬上限 |

**结论**：比 Hermes 的 ToolGuardrails 更完善。Hermes 的 SHA-256 精确参数去重这里用签名替代了，覆盖更广。**此项从 P0 移除。**

---

### 2. 三层压缩 — ⚠️ 架构不同，功能等价

**Reames Agent compact.go 已有**：

| 阶段 | 比率 | 动作 |
|------|------|------|
| Soft | 0.5 | 发出 Notice 警告 |
| Snip | — | 裁剪过长 tool result 的头尾 |
| Prune | — | 将过期 tool result 替换为占位符 |
| Compact | 0.8 | LLM 摘要化中间区域 |
| Force | 0.9 | 强制压缩（即使折叠价值低） |
| Circuit breaker | — | compactStuck 检测，连续压缩无效时暂停 |

- head=2 / tail=6 保留：✅ `pinnedPrefixLen()` + `tailStart()`
- 断路器：✅ `compactStuck`
- 机械回退：✅ `mechanicalFoldDigest()`

**与 Reames Lite 的区别**：Reames Lite 标为 "L1/L2/L3" 三级，Reames Agent 是连续 pipeline。概念全部覆盖，只是命名不同。**降为 P2——补充 CompactionUpdated 事件即可。**

---

### 3. 事件类型 — ⚠️ 20 种已有，仅缺 4 种

Reames Agent 已有 20 种 event.Kind。与 Reames Lite 对照：

| Reames Lite 事件 | Reames Agent | 状态 |
|---|---|---|
| TURN_STARTED | TurnStarted | ✅ |
| TURN_COMPLETED | TurnDone | ✅ |
| MESSAGE_DELTA/COMPLETE | Text / Message | ✅ |
| REASONING_DELTA | Reasoning | ✅ |
| TOOL_STARTED/COMPLETED | ToolDispatch / ToolResult | ✅ |
| APPROVAL_REQUESTED | ApprovalRequest | ✅ |
| ASK_REQUESTED | AskRequest | ✅ |
| USAGE_UPDATED | Usage | ✅ |
| COMPRESSION_UPDATED | CompactionStarted/CompactionDone | ⚠️ 部分 |
| SUBAGENT_STARTED/COMPLETED | — | ❌ 缺 |
| CACHE_UPDATED | — | ❌ 缺 |
| CACHE_IMPACT_RECORDED | — | ❌ 缺 |
| MODE_CHANGED | — | ❌ 缺 |

**真正缺失的只有 4 种事件类型**，其余全部有对应。

---

### 4. Intent Classifier — ⚠️ 有分类器，非安全意图

**存在的**：`task_classifier.go` — LLM 驱动的 task vs chat 分类，含启发式回退（固定词汇列表：问候语、聊天短语、任务短语、动作关键词）。

**缺失的**：AgentArk 风格的 **安全意图分类**（12 类固定词汇：override-instructions, extract-system-prompt, role-hijack 等）。当前 guardian 是"事后审查"（工具调用前的安全评估），不是用户输入"事前分类"。

---

### 5. 密钥安全存储 — ❌ 确实缺失

`internal/config/credentials.go`：API key 存入明文 `.env` 文件。无零化、无 AES 加密、无 Argon2 派生。

---

### 6. apply_patch — ❌ Go 核心缺失

Go builtin tools 19 个，不含 `apply_patch`。Python 层有实现，但 Python 不在我们的 Go 产品路径上。

---

### 7. web_search — ❌ Go 核心缺失

仅 `web_fetch` 工具。Go skill 文档明确说 "There is no dedicated web-search tool"。Python 插件有但不在 Go 路径。

---

### 8. 梦境引擎 — ⚠️ 有 memorycompiler，不是 LLM 去重

`internal/memorycompiler/`（2900+ 行）已有：因果图压缩、执行轨迹分析、策略评分、各种字符串去重函数。但明确声明 "deliberately local and rule-driven: the model never rewrites code"。

区别：Reames Lite 梦境引擎是 **LLM 驱动的合并/去重**（发现→审查→合并三步），memorycompiler 是**规则驱动的编译**。两者目标相似但机制不同。

---

### 9. Trust Boundary — ❌ 确实缺失

Guardian 是执行前门控，不是输出清洗。无 bluemonday 等 HTML 清洗库，无输出信封包装。

---

## 修正后的 P0 清单

| # | 功能 | 工作量 |
|---|------|--------|
| P0-1 | web_search 工具（Go 原生） | ~300行 |
| P0-2 | apply_patch 工具（Go 原生） | ~400行 |
| P0-3 | 密钥零化存储（zeroize + AES-GCM + Argon2） | ~200行 |
| P0-4 | Trust Boundary（HTML清洗 + 输出信封） | ~150行 |
| P0-5 | 4 种缺失事件类型 | ~100行 |

从 9 项缩减为 **5 项真正的缺失**。
