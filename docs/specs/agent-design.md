# AGENT 详细设计文档

> 版本：v0.1（草案）
> 基于 Hermes Agent 深度定制，方案B（深度定制）

---

## 目录

1. [架构总览](#1-架构总览)
2. [记忆系统](#2-记忆系统) ✅ 设计中
3. [MCP 集群](#3-mcp-集群)
4. [原生插件](#4-原生插件)
5. [钩子系统](#5-钩子系统)
6. [缓存优化](#6-缓存优化)
7. [熵管理系统](#7-熵管理系统)
8. [桌面端](#8-桌面端)
9. [开发路线](#9-开发路线)

---

## 1. 架构总览

详见 `agent-architecture-overview.md`（已确认方向）

---

## 2. 记忆系统

**方案：** 深度定制嵌入核心（方案B）

### 2.1 总体设计

```
Conversation Loop（深度修改版）
│
├─ [每轮开始] 记忆预取
│   ├─ 从 L3 Persona 获取用户画像摘要 → 注入系统提示词
│   ├─ 从 L2 Scenario 获取相关场景 → 注入上下文
│   └─ 从 L1 Atom 获取关键事实 → 按需检索
│
├─ [工具执行] 同步短期记忆
│   ├─ 工具输出过长时 → offload 到 refs/*.md
│   └─ 生成 Mermaid 符号图 → 保留在上下文中
│
├─ [每轮结束] 记忆同步
│   ├─ L0: 原始对话写入
│   ├─ L1: 原子事实提取（触发条件：每 N 轮或空闲时）
│   ├─ L2: 场景聚合（触发条件：新 L1 足够多）
│   └─ L3: 画像生成（触发条件：每 50 条新记忆）
│
└─ [后台] 记忆维护
    ├─ 碎片整理：合并相似 L2 场景
    ├─ 过期清理：超过 retention 期限的记忆归档
    └─ 画像更新：定时重新评估 L3 Persona
```

### 2.2 核心修改点

| 文件 | 修改内容 | 影响范围 |
|:---|:---|:---|
| `agent/conversation_loop.py` | 在主循环嵌入记忆预取/同步管道 | 核心运行逻辑 |
| `agent/system_prompt.py` | 将 L3 Persona 注入系统提示词 | 每轮对话 |
| `agent/tool_executor.py` | 工具输出过长时触发短期记忆 offload | 工具执行 |
| `agent/context_compressor.py` | 上下文压缩时保留符号化 Mermaid 图 | 压缩策略 |
| `agent/memory_manager.py` | 替换为 TencentDB 驱动 | 记忆管理 |
| `agent/prompt_caching.py` | 确保记忆注入不破坏 prefix-cache | 缓存优化 |

### 2.3 L0-L3 四层架构

```
L3: Persona（用户画像）
    存储：Markdown 文件 ~/.hermes/memory/persona.md
    内容：用户偏好、习惯用语、长期目标、技术栈偏好
    刷新：每 50 条新记忆或手动触发
    注入：系统提示词开头

L2: Scenario（场景块）
    存储：Markdown 文件 ~/.hermes/memory/scenarios/
    内容：常见问题模式、解决方案模板、项目上下文
    聚合：当积累足够多相关的 L1 原子事实时

L1: Atom（原子事实）
    存储：SQLite（FTS5 全文索引 + 向量索引）
    内容：用户说过的具体事实、决策、配置偏好
    提取：每 5 轮对话或空闲 10 分钟后
    检索：BM25 + 向量混合（RRF 融合）

L0: Conversation（原始对话）
    存储：SQLite（jsonl 格式）
    内容：完整的用户/助手消息历史
    保留：默认 30 天，之后可归档
```

### 2.4 符号化短期记忆（Mermaid）

```
触发条件：单轮工具输出超过上下文窗口的 35%
处理流程：
  1. 将冗长的工具输出（搜索/日志/代码）写入 refs/*.md
  2. 提取关键状态转换，生成 Mermaid 图
  3. 只在上下文中保留 Mermaid 图 + node_id
  4. 需要详细内容时通过 node_id 回溯到 refs/*.md

示例：
  graph LR
    A[搜索: "API 设计模式"] -->|结果1| B(factory pattern.md)
    A -->|结果2| C(singleton pattern.md)
    B --> D[代码分析完成]
    C --> D
```

### 2.5 混合检索策略

| 策略 | 算法 | 场景 |
|:---|:---|:---|
| **关键词检索** | BM25（SQLite FTS5） | 精确匹配、代码片段、技术术语 |
| **语义检索** | 向量嵌入（sqlite-vec） | 模糊查询、概念匹配、跨语言 |
| **融合排序** | RRF（Reciprocal Rank Fusion） | 合并两种检索结果，去重排序 |

### 2.6 关键配置参数（config.yaml）

```yaml
memory:
  provider: memory_tencentdb  # 深度定制版
  extraction:
    every_n_conversations: 5    # 每 N 轮提取 L1
    max_memories_per_pass: 20   # 每次提取最多条数
    idle_timeout: 600           # 空闲多少秒后触发提取
  persona:
    trigger_every_n: 50         # 每 50 条新记忆刷新画像
  recall:
    max_results: 5              # 每次召回最多条数
    strategy: hybrid            # keyword / embedding / hybrid
  offload:
    enabled: true               # 启用符号化短期记忆
    mild_ratio: 0.35             # 超过 35% 上下文触发轻度 offload
    aggressive_ratio: 0.85      # 超过 85% 触发激进 offload
  retention:
    l0_days: 30                 # 原始对话保留天数
    l1_days: 90                 # 原子事实保留天数
```

### 2.7 验证标准

- [ ] 5 轮对话后 L1 原子事实被正确提取
- [ ] 20 轮对话后 L2 场景开始聚合
- [ ] 50 条记忆后 L3 画像生成准确
- [ ] 符号化短期记忆节省 token 30%+
- [ ] 混合检索召回准确率 80%+
- [ ] 记忆注入不破坏 prefix-cache

---

## 3. MCP 集群

**目标：** 通过 MCP 协议接入外部服务，扩展 AGENT 的能力边界

### 3.1 服务选型与优先级

| 优先级 | 服务 | 项目 | 用途 | 部署方式 |
|:---:|:---|:---|:---|:---:|
| **P0** | 全网搜索 | searxng_http_mcp | 200+ 搜索引擎聚合，隐私保护 | Docker |
| **P0** | Git 开发 | github-mcp-server（30.7k⭐） | GitHub PR/Issue/Repo 操作 | Docker |
| **P1** | 代码分析 | ckb（102⭐）或 mcp-code-graph（396⭐） | AST/依赖图/影响分析 | Docker |
| **P1** | 高级爬虫 | webclaw（1.4k⭐） | 递归爬取、结构化数据提取 | Docker |
| **P2** | 文档解析 | 微软 MarkItDown（154k⭐） | PDF/Word/Excel/PPT/图片/音频 → Markdown，原生 Python，可做 Hermes 原生插件或 MCP 封装 | pip install |
| **P2** | 安全扫描 | agent-security-scanner-mcp（110⭐） | 漏洞/注入检测 | Docker |
| **P2** | 代码审查 | medusa（598⭐） | 79 个分析器，40000+ 规则 | Docker |

### 3.2 Hermes 配置模板

所有 MCP 服务统一在 `~/.hermes/config.yaml` 中配置：

```yaml
mcp_servers:
  # P0: 全网搜索
  searxng:
    url: "http://localhost:8888/mcp/"
    timeout: 120

  # P0: GitHub 开发
  github:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"
    timeout: 120

  # P1: 代码分析
  code-analysis:
    command: "docker"
    args: ["run", "--rm", "-i", "ckb-mcp-server"]
    timeout: 60

  # P1: 高级爬虫
  webcrawler:
    command: "docker"
    args: ["run", "--rm", "-i", "webclaw-mcp-server"]
    timeout: 180
```

### 3.3 部署架构

```
┌──────────────────────────────────────────────────┐
│                 你的 AGENT                         │
│  ┌────────────────────────────────────────────┐  │
│  │        MCP Client（tools/mcp_tool.py）      │  │
│  │   自动发现工具 → 注册到工具系统 → 模型可用    │  │
│  └──────────┬──────────┬──────────┬───────────┘  │
│             │          │          │              │
│    ┌────────▼──┐ ┌────▼────┐ ┌───▼────────┐     │
│    │ SearXNG   │ │ GitHub  │ │ 代码分析    │     │
│    │ (Docker)  │ │ (npx)   │ │ (Docker)   │     │
│    │ :8888     │ │ stdio   │ │ stdio      │     │
│    └───────────┘ └─────────┘ └────────────┘     │
│                                                 │
│    ┌───────────┐ ┌──────────┐ ┌────────────┐    │
│    │ 爬虫      │ │ 文档解析  │ │ 安全扫描    │    │
│    │ (Docker)  │ │ (Docker) │ │ (Docker)   │    │
│    └───────────┘ └──────────┘ └────────────┘    │
└──────────────────────────────────────────────────┘
```

### 3.4 技能封装

每个 MCP 服务需要一个对应 skill，教会 AGENT：
- 什么时候该用哪个 MCP 服务
- 怎么构造搜索/查询参数
- 怎么解析和利用返回结果

Skill 文件：`~/.hermes/skills/mcp-searxng/SKILL.md`、`~/.hermes/skills/mcp-github/SKILL.md` 等。

### 3.5 文档解析方案说明

文档解析推荐使用 **微软 MarkItDown**（154k⭐）：
- 支持格式：PDF、PowerPoint、Word、Excel、图片(EXIF+OCR)、音频、HTML、CSV、JSON、XML、ZIP、EPUB、YouTube
- 输出：纯 Markdown，Token 效率高
- 安装：`pip install markitdown[all]`
- 接入方式：
  方案A：封装为 MCP 服务器（Docker 部署）
  方案B：作为 Hermes 原生 Python 插件直接调用（更简单，推荐）
- MarkItDown 是 Python 库，与 Hermes（Python）同语言，推荐方案B

### 3.6 健康监控

- 每个 MCP 服务启动时自动探测（`mcp_tool.py` 的连接检测）
- 服务宕机时自动重连（指数退避，最多 5 次）
- 在熵管理仪表盘中显示 MCP 服务健康状态

### 3.7 部署顺序

```
第1步：SearXNG（搜索）— 最先部署，替代 Hermes 原生 web_search
第2步：GitHub MCP（开发）— 核心开发工作流
第3步：代码分析 MCP
第4步：高级爬虫 MCP
第5步：文档解析 + 安全扫描
```

### 3.7 验证标准

- [ ] SearXNG 搜索返回 200+ 引擎结果
- [ ] GitHub MCP 能完成 clone/commit/PR/review
- [ ] 代码分析 MCP 能正确解析项目结构
- [ ] 爬虫 MCP 能递归爬取指定深度
- [ ] 文档解析 MCP 能提取 PDF/Word 文本
- [ ] 所有 MCP 服务通过 Hermes 自动发现并注册到工具列表


---

## 4. 原生插件

**目标：** 将高频能力写成 Hermes 原生插件，按 `registry.register()` 模式接入工具系统

### 4.1 Git 开发工作流插件

插件名：`hermes-git`，目录 `plugins/hermes-git/`

| 工具名 | 功能 | 实现方式 |
|:---|:---|:---|
| `git_clone` | 克隆仓库，支持 HTTPS 和 SSH | 封装 git clone |
| `git_commit` | stage + commit，自动生成 message | git add + git commit |
| `git_branch` | 创建/切换/列出/删除分支 | git branch / checkout / switch |
| `git_pr` | 创建/列出/查看 PR diff | gh CLI 或 GitHub API |
| `git_review` | diff 分析 + 问题标记 + 审查意见 | git diff + AI 分析 |
| `git_log` | 查看历史 / 搜索 commit / changelog | git log / git show |
| `git_status` | 查看工作区状态 | git status / git diff --stat |

注册格式：
```python
registry.register(
    name="git_commit",
    toolset="git",
    schema=GIT_COMMIT_SCHEMA,
    handler=handle_git_commit,
    check_fn=lambda: shutil.which("git") is not None,
    emoji="🔀",
)
```

### 4.2 编译构建插件

插件名：`hermes-build`，目录 `plugins/hermes-build/`

项目类型自动检测逻辑：
```
检测顺序：
  1. 查找 go.mod → Go 项目
  2. 查找 package.json → Node.js 项目
  3. 查找 Cargo.toml → Rust 项目
  4. 查找 pyproject.toml / setup.py → Python 项目
  5. 查找 Makefile → make 项目
  6. 查找 CMakeLists.txt → CMake 项目
```

| 工具名 | 功能 |
|:---|:---|
| `build_project` | 自动检测类型 + 执行构建命令 |
| `test_project` | 自动检测类型 + 执行测试命令 |
| `lint_project` | 自动检测类型 + 执行 linter |
| `clean_project` | 清理构建产物 |
| `parse_build_error` | 解析编译错误 → 结构化输出 + 修复建议 |

### 4.3 代码分析插件

插件名：`hermes-code-analysis`，目录 `plugins/hermes-code-analysis/`

| 工具名 | 功能 | 技术 |
|:---|:---|:---|
| `analyze_complexity` | 圈复杂度、函数长度、嵌套深度 | tree-sitter AST |
| `find_duplicates` | 重复代码检测 | AST 指纹比对 |
| `analyze_dependencies` | 模块依赖图、循环依赖检测 | import 解析 |
| `suggest_refactor` | 基于分析结果出重构建议 | 规则引擎 + AI |
| `analyze_coverage` | 测试覆盖率分析 | coverage / gocov / nyc |

### 4.4 高级爬虫插件

插件名：`hermes-crawler`，目录 `plugins/hermes-crawler/`

| 工具名 | 功能 |
|:---|:---|
| `crawl_start` | 从入口 URL 开始递归爬取，控制深度/范围 |
| `crawl_extract` | CSS 选择器 / XPath 提取结构化数据 |
| `crawl_sitemap` | 从 sitemap.xml 爬取全站 |
| `crawl_status` | 查看爬取进度和结果 |

反封锁策略：
- 随机 User-Agent 轮换
- 请求间隔控制
- 代理轮换（可选）

### 4.5 插件开发规范

每个插件遵循 Hermes 标准结构：
```
plugins/<name>/
├── __init__.py      # 插件入口，注册工具
├── plugin.yaml      # 插件清单
├── README.md        # 使用说明
└── tests/           # 测试
```

工具注册统一走 `registry.register()`，与 Hermes 内置工具一致。

### 4.6 验证标准

- [ ] 每个插件单独可注册、可调用
- [ ] Git 插件能完成完整工作流（clone → branch → commit → PR → review）
- [ ] 构建插件能正确检测 3+ 种项目类型
- [ ] 代码分析插件 AST 解析准确率 90%+
- [ ] 爬虫插件能递归爬取 3 层深度
- [ ] 所有插件通过 Hermes 工具系统发现

## 5. 钩子系统

**目标：** 将 Reames 的三层钩子机制移植到 Hermes 核心

### 5.1 设计思路

Hermes 已有 `plugin.yaml` 支持 `post_tool_call` 和 `on_session_end` 钩子。
在此基础上扩展为三层：PreToolUse / PostToolUse / Stop + bypass 机制。

### 5.2 三层钩子定义

| 钩子 | 触发时机 | 类型 | 阻断能力 | 用途 |
|:---|:---|:---:|:---:|:---|
| **PreToolUse** | 工具执行前（权限检查后） | 同步 | ✅ exit 2 阻断 | 安全检查、资源检查、前置条件验证 |
| **PostToolUse** | 工具执行后（不论成功失败） | 同步 | ✅ exit 2 报警 | 结果验证、空架子检查、自动测试 |
| **Stop** | 每轮对话结束后 | 异步 | ❌ 仅通知 | 提醒用户检查、触发后台维护 |

### 5.3 钩子配置

```yaml
# config.yaml
hooks:
  PreToolUse:
    - match: "edit_file|write_file"
      command: "python3 ~/.hermes/hooks/pre-tool-check.py"
      description: "检查是否在删除真实代码换空壳"
      timeout: 5000
  PostToolUse:
    - match: "edit_file|write_file"
      command: "python3 ~/.hermes/hooks/check-empty-shell.py"
      description: "14 项空架子检查"
      timeout: 5000
    - match: "edit_file|write_file"
      command: "python3 ~/.hermes/hooks/run-tests.py"
      description: "自动跑测试"
      timeout: 120000
  Stop:
    - command: "python3 ~/.hermes/hooks/turn-notifier.py"
      description: "每轮结束提醒"
      timeout: 5000
```

### 5.4 移植的脚本（来自 Reames）

| 脚本 | 对应钩子 | 功能 |
|:---|:---|:---|
| `pre-tool-check.py` | PreToolUse | 改文件前检查是否大幅删代码 |
| `check-empty-shell.py` | PostToolUse | 14 项空架子检查（空函数/TODO/返回默认值/空catch/纯注释等） |
| `run-tests.py` | PostToolUse | 自动检测项目类型跑测试 |
| `turn-notifier.py` | Stop | Windows Toast 通知提醒 |

### 5.5 Bypass 机制

```python
# 每个钩子脚本开头检查
BYPASS_FILE = "~/.hermes/.bypass-hooks"
if os.path.exists(BYPASS_FILE):
    sys.exit(0)  # 绕过所有检查
```

创建/移除：
```bash
touch ~/.hermes/.bypass-hooks   # 绕过
rm -f ~/.hermes/.bypass-hooks   # 恢复
```

### 5.6 核心修改点

| 文件 | 修改内容 |
|:---|:---|
| `agent/tool_executor.py` | 工具执行前后插入 PreToolUse / PostToolUse 钩子调用 |
| `agent/conversation_loop.py` | 主循环末尾插入 Stop 钩子调用 |
| `agent/shell_hooks.py` | 扩展现有钩子系统支持三层 + bypass |
| `tools/registry.py` | 工具注册时可选关联钩子 |

### 5.7 与 Reames 现有钩子的关系

你在 Reames 上配的钩子（`pre-tool-check.js`、`check-empty-shell.js`、`run-tests.js`）可以直接移植为 Python 版本，逻辑不变。

### 5.8 验证标准

- [ ] PreToolUse 阻断：危险操作被拦截，模型收到错误
- [ ] PostToolUse 报警：空架子/TODO 被检测并反馈
- [ ] Stop 通知：每轮结束触发提醒
- [ ] Bypass：创建标志文件后所有钩子跳过
- [ ] 误报率 < 5%


## 6. 缓存优化

**目标：** 将 Reames 的字节稳定前缀缓存策略融入 Hermes

### 6.1 现状分析

Hermes AGENTS.md 明确指出「prompt caching is sacred」，现有策略：
- 不中途变更系统提示词
- 不中途变更工具集
- 仅在上下文压缩时改写历史

### 6.2 需要加强的地方

| 问题 | Reames 做法 | Hermes 现状 | 改进措施 |
|:---|:---|:---|:---|
| 消息序列化稳定性 | 确定性序列化，逐字节一致 | Python dict 序列化可能因键顺序变化 | 使用有序 dict + 规范 JSON 序列化 |
| 系统提示词注入 | 一次构建，永不修改 | 记忆/技能可能在运行时追加 | 将动态部分（Persona）固定在提示词末尾固定位置 |
| 工具集变更 | 会话内不切换 | 工具集可中途切换 | 确保切换只在会话边界发生 |
| 时间戳注入 | 不注入易变时间戳 | 部分场景有时间戳 | 使用固定占位符代替实时时间戳 |

### 6.3 核心修改点

| 文件 | 修改内容 |
|:---|:---|
| `agent/prompt_caching.py` | 实现字节稳定序列化 |
| `agent/system_prompt.py` | 固定动态部分的插入位置 |
| `agent/conversation_loop.py` | 禁止中会话切换工具集 |

### 6.4 验证标准

- [ ] 长会话缓存命中率 90%+
- [ ] 输入 token 成本降低 60%+
- [ ] 10 轮对话后缓存仍然有效

---



## 7. 熵管理系统

**目标：** 自动维护 AGENT 生态健康，防止技能/记忆/配置腐败（原创设计）

### 7.1 总体架构

```
熵管理调度器（cron job，每日/每周）
│
├─ 技能评估 → 遍历已安装技能 → 计算评分 → 低分标记 → 归档
├─ 记忆修剪 → 冷数据检测 → 过期归档 → 碎片整理
├─ 插件检查 → 冲突检测 → 版本扫描 → 健康报告
├─ 配置漂移 → 快照对比 → 标记变更 → 修复建议
│
└─ 汇总报告 → 桌面推送 / 消息平台通知
```

### 7.2 技能质量评估

| 指标 | 权重 | 数据来源 |
|:---|:---:|:---|
| 使用频率（近 30 天） | 30% | 使用日志 |
| 成功率 | 25% | 执行结果 |
| 代码质量（AST 检查） | 20% | tree-sitter 分析 |
| 最后使用时间 | 15% | 时间戳 |
| 用户反馈（点赞/踩） | 10% | 用户标记（无反馈时按 50% 折算） |

阈值：**< 40 分** → 标记为「待审查」，**连续 3 次评估 < 40 分 + 30 天未使用** → 自动归档

### 7.3 记忆修剪

| 策略 | 条件 | 操作 |
|:---|:---|:---|
| 时间衰减 | L0 超过 30 天 → 标记 | 归档到冷存储 |
| 频率衰减 | L1 超过 90 天未访问 → 标记 | 归档到冷存储 |
| 场景合并 | 相似度 > 85% 的 L2 场景 | 合并为一个 |
| 画像过期 | L3 超过 60 天未更新 | 触发重新评估 |

### 7.4 插件健康检查

| 检查项 | 方法 |
|:---|:---|
| 工具名冲突 | 扫描所有注册工具，检测重名 |
| 钩子冲突 | 检查多个插件注册了相同钩子 |
| 版本兼容 | 对比 Hermes 版本与插件声明的兼容版本 |
| 依赖缺失 | 检查插件所需的 Python 包是否安装 |

### 7.5 配置漂移检测

```yaml
# 初始快照（安装时记录）
config_snapshot:
  captured_at: "2026-06-15T00:00:00Z"
  sections: ["memory", "mcp_servers", "hooks", "skills"]

# 后续对比
# 新增项 → 标记为「未审阅」
# 修改项 → 标记为「已变更」
# 废弃项（存在但未被引用）→ 标记为「可清理」
```

### 7.6 定时清扫者

```yaml
cron:
  jobs:
    entropy_janitor:
      schedule: "0 3 * * 0"  # 每周日凌晨 3 点
      command: "hermes entropy --run"
      description: "熵管理清扫：技能评估 + 记忆修剪 + 插件检查 + 配置漂移"
```

### 7.7 验证标准

- [ ] 技能评估能正确识别低质量技能
- [ ] 记忆修剪不误删重要记忆
- [ ] 插件冲突检测准确率 100%
- [ ] 配置漂移能标记新增/修改/废弃项
- [ ] 定时清扫者按计划自动执行

---


## 8. 桌面端

**目标：** 基于 Hermes Electron 桌面端定制品牌

### 8.1 定制内容

| 项目 | 说明 |
|:---|:---|
| 名称 | 你的 AGENT 的专属名称 |
| Logo | 自定义图标 |
| 主题色 | 品牌色系 |
| 熵管理仪表盘 | 技能健康度 / 记忆使用率 / 插件状态 / MCP 健康 |

### 8.2 待设计

桌面端定制在设计阶段后期进行，先完成核心功能。

---


## 9. 开发路线（完整版）

```
阶段1 ✅ 学习 — Hermes/TencentDB/SearXNG/MCP 源码完成
阶段2 🔄 设计 — 当前进行中（620 行设计文档）
阶段3 📅 记忆系统 — TencentDB 4 层集成 + 符号化短期记忆
阶段4 📅 MCP 集群 — SearXNG → GitHub → 代码分析 → 爬虫 → 文档
阶段5 📅 原生插件 — Git → 编译 → 代码分析 → 爬虫
阶段6 📅 钩子系统 — Reames PreToolUse/PostToolUse/Stop 移植
阶段7 📅 缓存优化 — 字节稳定前缀策略
阶段8 📅 熵管理系统 — 设计已完成，等待实施
阶段9 📅 桌面端 — 品牌定制 + 仪表盘
```
