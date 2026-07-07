# Reames Agent — 架构文档

> Reames Agent 是基于 Hermes Agent v0.16.0 的定制分支，深度融合了 TencentDB 记忆系统设计理念和 Reames Agent 的缓存/压缩机制，专门适配 DeepSeek 大模型。

---

## 一、项目定位

Reames Agent 是一个**命令行 AI 助手**，运行在终端环境中。它通过工具调用（shell、文件读写、搜索等）直接在用户的开发环境里工作。

**核心特性：**
- 多层级记忆系统（L0-L3），纯 SQLite 本地存储，零外部依赖
- DeepSeek prefix-cache 优先的对话压缩机制
- 超大工具结果的 Mermaid Offload 分流
- 国产大模型优先（DeepSeek），兼顾 OpenAI/Anthropic 等标准协议

---

## 二、记忆系统架构

### 数据流总图

```
用户输入
  |
  +---> Hermes 骨架 --> state.db（完整录音：user/assistant/tool_call/tool_result）
  |
  +---> ReamesMemory --> reames_memory.db（精简记忆引擎）
         +-- L0: 只存 user+assistant（精简对话）
         +-- L1: 原子事实（每 10 轮 LLM 提取）
         +-- L2: 场景聚合（>=20 条事实触发）
         +-- L3: 用户画像（>=50 条事实时，启动/结束刷新）

  超大工具结果（>5000 字符）
         +---> offload --> refs/*.md（完整原始文件，60 天后自动清理）
                       --> Mermaid 标签替换进对话（省 token）
                       --> capture_offload 存进 messages 表 role=tool（可检索）
```

### 各层级详述

#### L0 — 原始对话（messages 表）

写入时机：每轮 run_conversation() 结束时
触发链：conversation_loop.py -> _sync_reames_memory_for_turn -> run_agent.py sync_all() -> reames_memory.capture_turn()

写入内容：INSERT INTO messages (session_id, role, content) VALUES (?, 'user', ?), (?, 'assistant', ?)
只存 user+assistant 两条，不存 tool_call/tool_result

关键代码：agent/reames_memory.py:164-198

#### L1 — 原子事实（memories 表）

写入时机：每 10 轮对话后（_l1_pending >= _l1_interval）
触发链：capture_turn() -> _l1_pending++ -> _extract_l1() -> _get_recent() -> LLM 提取

提取 prompt：
  从以下对话中提取关键事实（用户偏好、项目信息、技术决策）。
  每行一条事实，不要评论。

去重机制：
  1. set 去重（_extract_l1 中维护 existing set）
  2. UNIQUE index（idx_mem_content）
  3. IntegrityError 后 pass

关键代码：agent/reames_memory.py:456-491

#### L2 — 场景聚合（scenes.md）

写入时机：每 10 轮检查 memories 表计数 >= 20 条
触发链：capture_turn() -> 检查 memories 计数 -> _aggregate_l2() -> LLM 聚合成 2-4 个场景

内部守卫：cur - last_l2_count >= l2_interval（防止重复聚合）

注入方式：不注入 system prompt，仅在 recall() 搜索时按标题命中注入

关键代码：agent/reames_memory.py:524-541

#### L3 — 用户画像（persona.md）

写入时机：仅两处
  1. initialize() 时 memories >= 50 条
  2. on_session_end() 时 memories >= 50 条
  不在每轮检查中触发（已移除）

读取内容：最新 100 条 L1 事实
注入方式：通过 system_prompt_block() -> system prompt volatile 层
  当前会话看不到新画像（daemon thread 在 initialize() 返回后才运行）
  新画像只在下个会话生效

关键代码：agent/reames_memory.py:245-256, 547-556

### 检索路径

当 LLM 调用 reames_memory_search 工具时：

```
recall(query)
  |
  +-- L3 --> persona.md（匹配则注入）
  +-- L2 --> scenes.md（匹配标题则注入）
  +-- L1 --> memories FTS5 全文搜索 + LIKE 回退
  +-- L0 --> messages FTS5 全文搜索 + LIKE 回退（只搜 user/assistant）
  +-- 向量搜索（如果配置了 embedding_api_key）
  +-- RRF 融合排序 --> 取前 recall_count 条
```

关键代码：agent/reames_memory.py:200-256

---

## 三、缓存与压缩机制

### 3.1 DeepSeek prefix-cache 稳定性

为了最大化 DeepSeek 的 prefix-cache 命中率，Reames Agent 保证：
1. System prompt 在一轮会话中不变（build once, replay verbatim）
2. 外部记忆注入到用户消息末尾，不修改 system prompt
3. _get_recent() 过滤 <memory-context> 块，不影响 L1 提取质量
4. 确定性 JSON 序列化（deterministic_json），确保相同内容的 JSON 表示一致

### 3.2 cache-first 压缩模式

当 compression.cache_first 为 true（或使用 DeepSeek 模型时自动启用）：
- 跳过 LLM 摘要：压缩时不调用辅助模型生成摘要（避免破坏 prefix-cache）
- 仅裁剪工具调用：只删除最早的 tool_call/tool_result 对
- 创建新会话：压缩时会创建 continuation session，保持缓存前缀不变

配置：
```yaml
compression:
  cache_first: true   # DeepSeek 模型自动设为 true
```

### 3.3 状态栏缓存命中率

显示字段：
- 本次命中：当前 API call 的 cache_read_tokens / total_tokens
- 平均命中：滑动平均（历史 70% + 当前 30%）
- CTX 进度条：当前上下文占用百分比
- 压缩比例：已进行的压缩节省比例

---

## 四、Mermaid Offload

当工具返回结果 > 5000 字符时：

```
maybe_offload(content, tool_name, tool_use_id)
  +-- 保存完整内容 --> refs/{tool_use_id}.md
  +-- 替换为 Mermaid 标签 --> <mermaid-offload>...</mermaid-offload>
  +-- capture_offload --> messages 表 (role=tool, 前 3000 字符)
```

清理：仅启动时执行一次，默认 60 天自动清理
配置：
```yaml
offload:
  retention_days: 60    # 0 = 永不过期
```

关键代码：agent/mermaid_offload.py
