# Reames Agent

多平台 AI 编程助手。终端原生 CLI、桌面应用、云端部署服务、IM 机器人网关 — 全部来自单个 Go 二进制文件。

**基于 DeepSeek Reasonix (MIT)，融合 8 个参考项目和 Reames Lite 的优点。**

## 快速开始

```bash
# 从源码构建（需要 Go 1.25+）
git clone https://github.com/reames-agent/reames-agent.git
cd reames-agent
go build -o bin/reames-agent ./cmd/reames-agent

# 配置
./bin/reames-agent setup

# 启动交互会话
./bin/reames-agent
```

## 功能

- **多模型**: DeepSeek、OpenAI 兼容、Anthropic — 配置驱动，无需硬编码
- **缓存优先**: DeepSeek 前缀缓存优化，目标 95%+ 命中率
- **三端统一**: CLI(Bubble Tea TUI)、桌面(Wails+React)、云端(HTTP/SSE)
- **IM 网关**: 飞书、QQ、微信、Telegram 机器人适配
- **插件/MCP**: MCP stdio+HTTP 双传输，技能 playbook 系统
- **单二进制**: CGO_ENABLED=0，6 平台交叉编译

## 使用

```bash
reames-agent                        # 交互式 CLI
reames-agent run "修复 auth 的 bug" # 单任务执行
reames-agent serve                  # 启动 Web UI (localhost:8787)
reames-agent gateway start --channels feishu  # 启动 IM 机器人
```

## 云端部署

```bash
docker build -t reames-agent .
docker run -p 8787:8787 -e DEEPSEEK_API_KEY=sk-xxx reames-agent
```

详见 [docs/DEPLOY.md](docs/DEPLOY.md)

## 文档

- [架构设计](docs/ARCHITECTURE.md)
- [部署指南](docs/DEPLOY.md)
- [产品路线图](docs/PRODUCT_ROADMAP.md)
- [参考移植路线](docs/REFERENCE_PORTING_ROADMAP.md)

## 许可证

MIT。基于 [DeepSeek Reasonix](https://github.com/esengine/DeepSeek-Reasonix)。
