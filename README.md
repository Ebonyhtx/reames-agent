# Reames Agent

> 基于 Hermes Agent 深度定制的 AI 编码助手 — 由 Reames 深度定制。

## 功能概览

| 模块 | 说明 | 状态 |
|:---|:---|:---:|
| **记忆系统** | L0-L4 渐进式披露 + Mermaid Offload + 记忆后端 | ✅ |
| **MCP 集群** | SearXNG 搜索 + GitHub + MarkItDown 文档解析 | ✅ |
| **原生插件** | Git(7) + Build(4) + CodeAnalysis(2) + Crawler(1) | ✅ |
| **钩子系统** | PreToolUse / PostToolUse / Stop 三层校验 | ✅ |
| **缓存优化** | 字节稳定前缀 + CacheStats + 实时状态栏 | ✅ |
| **熵管理** | 技能评估 + 记忆修剪 + 插件检查 + 配置漂移 | ✅ |

## 安装

### 前置条件

- Python 3.12+
- Node.js 18+（可选，用于 GitHub MCP 等）

### 快速安装

```bash
git clone <你的仓库链接>
cd reames-agent

# 创建虚拟环境
python -m venv .venv
.venv\Scriptsctivate  # Windows
# source .venv/bin/activate  # macOS/Linux

# 安装依赖
pip install -e .

# 启动
hermes
```

## 使用

```bash
hermes  # 启动对话
```

看到状态栏即表示 Reames 加载成功：

```
deepseek-v4-flash | 77.27% | avg 77.27% | 5.5K | ¥0.0250 | 3 turns
```

## 项目结构

```
📦 reames-agent
├─ agent/                     # 核心模块
│  ├─ entropy.py              # 熵管理系统
│  ├─ status_bar.py           # 实时状态栏
│  ├─ mermaid_offload.py      # 大输出卸载
│  ├─ deepseek_cache.py       # 缓存优化
│  └─ ... (Hermes 原核心)
├─ plugins/
│  ├─ reames_git/             # Git 开发工具 (7)
│  ├─ reames_build/           # 构建工具 (4)
│  ├─ reames_code_analysis/   # 代码分析 (2)
│  └─ reames_crawler/         # 网页爬虫 (1)
├─ hooks/                     # Hermes 钩子脚本
├─ docs/
│  └─ specs/                  # 设计文档
├─ reames-setup.sh            # 安装脚本
└─ README.md
```

## 在新电脑上无缝开发

```bash
git clone <仓库链接>
cd reames-agent
python -m venv .venv
.venv\Scriptsctivate
pip install -e .
hermes
```

## 技术栈

- **基础框架**: Hermes Agent v0.16
- **模型**: DeepSeek V4 Flash（默认）
- **记忆后端**: 多后端记忆系统
- **搜索**: SearXNG
- **文档解析**: Microsoft MarkItDown
