# Reames Agent

> 基于 Hermes Agent 深度定制的 AI 编码助手 — 由 Reasonix + Hermes 融合而成。

## 功能概览

| 模块 | 说明 | 状态 |
|:---|:---|:---:|
| **记忆系统** | L0-L4 渐进式披露 + Mermaid Offload + TencentDB | ✅ |
| **MCP 集群** | SearXNG 搜索 + GitHub + MarkItDown 文档解析 | ✅ |
| **原生插件** | Git(7) + Build(4) + CodeAnalysis(2) + Crawler(1) | ✅ |
| **钩子系统** | PreToolUse / PostToolUse / Stop 三层校验 | ✅ |
| **缓存优化** | 字节稳定前缀 + CacheStats + 实时状态栏 | ✅ |
| **熵管理** | 技能评估 + 记忆修剪 + 插件检查 + 配置漂移 | ✅ |

## 安装

### 前置条件

- [Hermes Agent](https://hermes.ai) v0.16+ 已安装
- Python 3.12+
- Node.js 18+（用于部分 MCP 服务器）

### 快速安装

```bash
# 1. 克隆仓库到 Hermes 安装目录
git clone <你的仓库链接> /path/to/hermes-agent
cd /path/to/hermes-agent

# 2. 运行安装脚本
chmod +x reames-setup.sh
./reames-setup.sh

# 3. 重启 Hermes
```

### 手动安装

```bash
# 1. 备份原有的配置文件
cp ~/.hermes/config.yaml ~/.hermes/config.yaml.bak

# 2. 安装 Python 依赖
pip install markitdown[all] searxng-mcp

# 3. 复制自定义文件
cp agent/*.py /path/to/hermes-agent/agent/
cp -r plugins/reames_* /path/to/hermes-agent/plugins/
cp -r hooks/* /path/to/hermes-agent/hooks/

# 4. 配置 MCP 服务器（见 .hermes/config.yaml）
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
# 1. 安装 Hermes Agent
# 2. 克隆仓库到 Hermes 安装目录
git clone <仓库链接> /path/to/hermes-agent
# 3. 运行安装脚本
./reames-setup.sh
# 4. 开始开发
```

## 技术栈

- **基础框架**: Hermes Agent v0.16
- **模型**: DeepSeek V4 Flash（默认）
- **向量数据库**: TencentDB Agent Memory
- **搜索**: SearXNG
- **文档解析**: Microsoft MarkItDown
