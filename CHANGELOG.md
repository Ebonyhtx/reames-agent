# Reames Agent — Changelog



## [v0.16.0-reames.2] — 2026-06-16

### 🏗 Hermes 配置完全隔离

- 28 处硬编码 `~/.hermes` 路径改为 `get_hermes_home()`（返回 `~/.reames`）
- `get_hermes_home()` 忽略指向 `.hermes` 的旧 `HERMES_HOME` 环境变量
- `REAMES_HOME` 环境变量可自定义路径（优先级最高）
- `file_safety.py`、`backup.py` 安全检查兼容 `.reames`

### 📦 首次启动自动配置

- 首次启动自动生成 `~/.reames/config.yaml`（13 个完整配置段）
- 覆盖：model、providers、agent、terminal、memory、display、approvals、hooks、cron、gateway、web、browser、compression
- `${DEEPSEEK_API_KEY}` 和 `${MEMORY_EMBEDDING_API_KEY}` 自动从环境变量解析
- 缺失配置段由 `DEFAULT_CONFIG` 运行时补全

### 🚀 Windows 一键安装

- 新增 `install.ps1`：`irm ... | iex` 一行命令安装
- 自动检测 Python/Git → 克隆 → venv → pip → PATH → 配置向导
- 安装向导引导设置 `DEEPSEEK_API_KEY` 和 `MEMORY_EMBEDDING_API_KEY`
- 自动检测并隔离旧 Hermes 安装

### 🐛 Bug 修复

- 修复 `env_loader.py` 缺 `get_hermes_home` import 导致启动崩溃
- 修复旧 `HERMES_HOME` 指向 `.hermes` 时仍被 Reames 读取的问题

## [v0.16.0-reames.1] — 2026-06-16

### 🔧 记忆系统完善与 Bug 修复（7 轮手术刀）

**向量检索修复：**
- 修复 config.yaml 中 `${MEMORY_EMBEDDING_API_KEY}` 不会被环境变量替换
- 修复 `configure_embedding` 每次只补 20 条 → 全量补嵌
- 嵌入数量 e=40 → e=69 ✅

**关键词搜索修复：**
- 修复 LIKE 后备代码在 with conn 块外执行（conn 已关闭），静默失败
- LIKE 后备改为独立 with 块，中文搜索恢复 ✅

**新鲜度权重完善：**
- 从天衰减改为周衰减（`/86400` → `/86400/7.0`）
- 新增小时内衰减（同日数据也有区分）
- 新鲜度查询增加 messages 表 fallback（之前只查 memories）

**搜索注入优化：**
- L2 scenes.md 从无条件注入改为按标题匹配注入
- 2-gram 中文分词匹配（复合查询也能命中标题）
- L3 persona 从 200 字缩减到 150 字

**数据清理：**
- 删除旧矛盾 L1 事实（"L2/L3 未实现" 等）
- 重建 scenes.md（下次 L2 触发自动回归）

### 🧹 项目整理
- 删除 Hermes 残留文件：`setup-hermes.sh`、`.hermes-bootstrap-complete`、`hermes-already-has-routines.md`
- 清理根目录 `__pycache__/`

## [v0.16.0-reames] — 2026-06-16

> 基于 Hermes Agent v0.16.0 深度定制，融合 Reasonix 缓存优化、DeepSeek 适配、
> 自研记忆引擎、信息栏系统、品牌全面独立。

---

### 🧠 记忆系统（最大改动）

**删除 Hermes 原生 MemoryStore + 9 个记忆插件 + MemoryProvider ABC + MemoryManager 编排层。**

- ❌ 删除 `tools/memory_tool.py`（MemoryStore: MEMORY.md + USER.md）
- ❌ 删除 `agent/memory_provider.py`（ABC 抽象基类）
- ❌ 删除 `agent/memory_manager.py` 中的 MemoryManager 类
- ❌ 删除 `plugins/memory/` 全部 9 个提供者
- ❌ 删除 `plugins/memory/memory_tencentdb/`（插件式 TencentDB）
- ✅ 新建 `agent/reames_memory.py`（415 行纯 Python，SQLite 直连）

**ReamesMemory 架构：**

| 层级 | 说明 | 实现 |
|---|---|---|
| **L0** | 原始对话存储 | SQLite `messages` 表 + FTS5 全文索引 |
| **L1** | 原子事实提取 | DeepSeek 每 10 轮提取 + SQLite `memories` 表 |
| **L1 检索** | 混合搜索 | FTS5 关键词 + 用户可配嵌入 API 向量语义搜索 |
| **L2** | 场景聚合 | DeepSeek 聚合同主题事实 → `scenes.md` |
| **L3** | 用户画像 | DeepSeek 合成 → `persona.md` → 注入系统提示词 |
| **去重** | 三重保护 | set 内存去重 + UNIQUE 索引 + IntegrityError |
| **向量** | 可选 | 配置 `MEMORY_EMBEDDING_API_KEY` 即启用语义搜索 |

**L3 注入机制：** 会话内只注入一次，字节完全稳定，不破坏 DeepSeek prefix-cache。

**净减少代码：** 删除 17,429 行旧代码，新增 ~500 行。零外部依赖。

---

### 🔄 缓存优化（Reasonix 移植）

- ✅ 创建 `agent/deepseek_cache.py` — `deterministic_json(sort_keys=True)` + `ensure_cache_stable()` + `CacheStats`
- ✅ 系统提示词仅会话开始时构建一次（`_cached_system_prompt`），跨轮字节稳定
- ✅ 时间戳仅精确到日期（不精确到分钟），`invalidate_system_prompt()` 在压缩后保持注释
- ✅ `ensure_cache_stable()` 已接入 `conversation_loop.py` 消息构造管道
- ✅ DeepSeek prefix-cache 命中率 98%+（状态栏实时显示）

---

### 📊 信息栏系统

- ✅ 创建 `agent/status_bar.py` — 实时状态栏
  - 本轮命中率 + 多轮滑动平均（来自真实 API 数据，非 CacheStats 猜测）
  - 用户对话轮次（非 API 调用轮次）
  - CTX 进度条 `[████░░░░░░] X%`（压缩后自动回落）
  - 本轮费用 + 会话总费用（DeepSeek 官方人民币定价）
  - DeepSeek 余额自动查询
- ✅ 状态栏接入 `conversation_loop.py`、`cli.py` 渲染
- ✅ 定价：`deepseek-v4-flash` 输入 ¥1/百万、输出 ¥2/百万、缓存命中 ¥0.02/百万
- ✅ 定价：`deepseek-v4-pro` 输入 ¥3/百万、输出 ¥6/百万、缓存命中 ¥0.025/百万

---

### 🎨 CLI 品牌改造

- ✅ 启动画面 "Reames Agent v0.16.0"
- ✅ 所有错误提示 `hermes setup` → `reames setup`
- ✅ `KawaiiSpinner` → `ReamesSpinner`
- ✅ 删除首次运行设置向导
- ✅ 去除启动 Tip 提示
- ✅ 命令行入口点 `reames.exe`

---

### 🐛 Bug 修复

- 状态栏 `CostResult.cost` → `CostResult.amount_usd`（AttributeError 根因）
- 压缩阈值显示硬编码 80% → 50%（匹配实际配置）
- `_cache_stats` 每轮被销毁重建 → `agent._cache_stats` 持久化
- 平均命中率 `CacheStats.hit_rate` 永为 0 → 改用 API 真实数据滑动平均
- CTX 进度条数据源错误 → `context_compressor.last_prompt_tokens`
- 轮次计算错误（75轮≠13轮） → `user_turn_count` 独立计数
- DeepSeek 余额 API 解析错误 → 正确读 `balance_infos[0].total_balance`
- `print_status_line()` stderr 输出污染回复框 → 已移除

---

### 🗑️ 清理

- 删除 44 个文件（9 个记忆插件 + memory_provider + memory_tool + .bak + __pycache__）
- 精简 `memory_manager.py`（只保留 `sanitize_context` 等工具函数）
- 移除项目 `~/.hermes` 目录下的过时 .bak 文件

---

### 📋 文件变更统计

| 操作 | 数量 |
|---|---|
| 新建文件 | `agent/reames_memory.py`、`agent/deepseek_cache.py`、`agent/status_bar.py` |
| 删除文件 | 44 个（plugins/memory/* 等） |
| 修改文件 | `agent_init.py`、`system_prompt.py`、`conversation_loop.py`、`cli.py`、`run_agent.py` 等 15 个 |
| 净代码变化 | -16,343 行 |

---

*以上改动由 Reasonix (deepseek-v4-flash) 辅助完成。用户：阿波。*


## [v0.16.0-reames.4] — 2026-06-17

### 🔥 缓存优先压缩模式（`cache_first`）

**核心思路：** Reasonix 的缓存优化理念融入 Hermes 压缩管道。

```
旧压缩流程（Hermes 遗留）：
  Phase 1: 工具结果修剪  ← 保留对话结构
  Phase 2: 边界计算
  Phase 3: LLM 摘要生成  ← 改变消息结构 → DeepSeek 缓存 MISS ❌

新 cache_first 模式：
  Phase 1: 工具结果修剪  ← 对话结构原封不动
  Phase 2: 边界计算
  [跳过 Phase 3]          ← 前缀字节稳定 → DeepSeek 缓存命中 ✅
```

**依据：** DeepSeek 磁盘 KV 缓存无固定大小限制，缓存命中的唯一条件是前缀字节稳定性。
LLM 摘要改变消息结构 → 缓存失效。工具修剪不改变对话结构 → 缓存持续命中。
（来源：DeepSeek 官方文档 — Context Caching 章节）

| 文件 | 改动 |
|------|------|
| `agent/context_compressor.py` | 新增 `cache_first: bool = False` 参数；`compress()` 中 Phase 1 后判断跳过 Phase 3 |
| `agent/agent_init.py` | 从 `config.yaml` 读取 `compression.cache_first`，传递给 ContextCompressor 构造器 |
| `agent/agent_init.py` | config 自动生成模板新增 `# cache_first: false` 配置项 |

### 🌡️ 压缩后缓存预热（Cache Warming）

压缩后新前缀的 KV cache 未被计算，下一轮必定 MISS。预热提前触发 DeepSeek 计算 KV cache。

- daemon 线程，`max_tokens=1`，`temperature=0.0`
- **仅对 DeepSeek 模型触发**（检查 `model.startswith("deepseek")`）
- 全 `try/except` + `timeout=10`，失败无害
- 费用 ≈ `max_tokens=1` 输出 ≈ $0.000001

| 文件 | 改动 |
|------|------|
| `agent/conversation_compression.py` | 新增 `_fire_cache_warmup()` 函数；`compress_context()` 末尾调用 |

### 🧪 待测试项

- [ ] `cache_first=True` 时 `compress()` 跳过 Phase 3
- [ ] `cache_first=True` 仍然执行工具修剪
- [ ] 缓存预热仅 DeepSeek 模型触发
- [ ] 非 DeepSeek 模型不触发预热
- [ ] 无 `_cache_stats` 时不触发预热
- [ ] `cache_first` 默认 False 不改变现有行为

---

*以上改动由 Reasonix (deepseek-v4-flash) 辅助完成。*
