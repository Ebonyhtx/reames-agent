# Reames Agent

Go 1.25+ 多平台 AI 编程助手。基于 DeepSeek Reasonix (MIT)，融合 10 个参考项目优点。

## 项目结构

```text
cmd/reames-agent/              # CLI 入口（Bubble Tea TUI）
cmd/reames-agent-plugin-example/ # MCP 插件示例
internal/
  agent/          # 核心 Agent loop、Session、Compaction、Task 子代理
  control/        # 传输无关 Controller（CLI/serve/Desktop 共享）
  provider/       # LLM Provider 接口 + OpenAI/Anthropic 实现
  tool/builtin/   # 24 个内置工具（bash, read_file, web_search, apply_patch 等）
  serve/          # HTTP/SSE 服务（Web/Cloud 前端）
  bot/            # IM Bot（飞书/QQ/微信/Telegram）
  cli/            # Bubble Tea TUI 实现
  config/         # TOML 配置加载
  plugin/         # MCP 插件宿主（stdio + HTTP）
  skill/          # Playbook 技能系统
  memory/         # 层级化记忆
  hook/           # 生命周期 Hooks
  sandbox/        # OS 沙箱
  permission/     # 权限门控
  crypto/         # AES-256-GCM + Argon2id 加密
  trust/          # HTML 清洗 + 输出信封 + 密钥脱敏
  cron/           # 持久化定时任务
  board/          # 统一工作台状态投影
  lsp/            # LSP 客户端（诊断/定义/引用/悬停/Delta基线）
  evidence/       # 证据账本（complete_step 验证）
  i18n/           # 国际化（zh/en）
  checkpoint/     # 文件检查点/回退
  guardian/       # 安全审查子代理
  planmode/        # Plan 模式只读门控
desktop/          # Wails v2 桌面应用
  app.go          # Go 后端（Wails 绑定）
  frontend/       # React 19 + Vite 8 + Zustand 前端
docs/             # 项目方向、发展计划、架构、用户与运维文档
```

## 开发约束

- **Go 1.25+**，CGO_ENABLED=0 单二进制，6 平台交叉编译
- **GOPROXY**：中国大陆用 `https://goproxy.cn,direct`
- **先读代码再改**：`internal/` 下每个包有 package comment 说明职责
- **缓存优先**：system prompt 不可变，tool schemas 按稳定顺序导出，UI 状态不注入 prompt
- **传输无关**：`control.Controller` 驱动 CLI/serve/Desktop 三个前端，新功能加在 controller 不是前端
- **每步验证**：`go build ./...` → `go vet ./...` → `go test ./internal/...`
- **同步写测试**：新模块必须有对应 `_test.go`
- **不提交二进制**：`bin/` 目录不提交

## 常用命令

```bash
# 构建
go build -o bin/reames-agent.exe ./cmd/reames-agent

# 全量测试（跳过慢的 control 测试）
go test ./internal/crypto/... ./internal/trust/... ./internal/cron/... \
  ./internal/board/... ./internal/pluginpkg/... ./internal/config/... \
  ./internal/agent/... ./internal/tool/builtin/... ./internal/provider/... \
  ./internal/hook/... ./internal/skill/... ./internal/lsp/... -count=1

# 交叉编译
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reames-agent-linux-amd64 ./cmd/reames-agent

# 公开、品牌和 legacy-tree 清洁棘轮
python scripts/check_public_readiness.py

# 工具契约验证
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v

# 官方上游追踪机制验证
python -m unittest scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
```

## 代码风格

- Go 标准风格，gofmt 格式化
- 包名小写单数，无下划线
- 接口以 `er` 结尾或用名词
- 错误处理用 `fmt.Errorf` 包装上下文
- 零值初始化，避免构造函数重载
- 单元测试用 `t.Fatal` 而非 `assert` 库

## 参考项目

项目源流分三级：`esengine/DeepSeek-Reasonix` 的 `main-v2` 是一级主源码上游；OpenAI Codex 与 Claude Code 是二级战略代码上游，分别跟进 GPT/OpenAI 与 Claude/Anthropic 的原生模型协议和代码级能力；`F:\code-reference` 下其余仓库只提供机制与体验参考。`F:\Reames-Lite` 是项目前身和契约参考。详细规则见 `docs/REFERENCE_GOVERNANCE.md`。

| 项目 | 路径 | 复用方向 |
|---|---|---|
| DeepSeek Reasonix | `F:\code-reference\DeepSeek-Reasonix` | 源码基座（Go, Wails, Bubble Tea） |
| Hermes | `F:\code-reference\Hermes` | 频道/社交集成、错误分类器 |
| Codex CLI | `F:\code-reference\codex` | 二级战略代码上游；GPT/Responses、App-Server、插件、Hook、LSP、CDP/自动化能力 |
| MiMo Code | `F:\code-reference\MiMo-Code` | 设计系统、OKLCH 颜色工具 |
| Impeccable | `F:\code-reference\impeccable` | 品牌设计语言 |
| Scream Code | `F:\code-reference\scream-code` | 主题系统、Goal Loop |
| AgentArk | `F:\code-reference\AgentArk` | 安全架构、Intent Classifier |
| Claude Code | `F:\code-reference\claude-code` | 二级战略代码上游；Claude/Messages、Thinking、工具/视觉/缓存与插件生态 |
| Kimi Code | `F:\code-reference\kimi-code` | 桌面 Shell 设计 |
| Grok Build | `F:\code-reference\Grok-Build` | 权限/沙箱、持久会话与子代理、TUI/ACP/headless 交互 |
| Reames Lite | `F:\Reames-Lite` | 旧版 Python 项目（合约参考） |

## 当前状态

- 产品方向以 `docs/PROJECT.md` 为准，执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。
- M0、M1、M2、M3、M4 已按路线图门槛关闭；M5 所有可由仓库、clean clone 和 CI/CodeQL 验证的事项已收口。
- 当前唯一未关闭的 M5 项是需要真实运营主体的公开 registry 密钥仪式、生产 endpoint、实际轮换/compromise drill 与 provenance policy，保持 `external-blocked`。
- P1 writer worktree 隔离与 P2 Offline Guard/Safe Mode 的仓库内机制已关闭；Guard、CLI/Serve/Desktop/Gateway 共用 `internal/repair`，Safe Mode 为 recovery-only 且不装配 Provider/Controller/Agent。M6 Linux user-scope Gateway install/uninstall 均已进入可故障回滚事务；代码提交 `a6d6fd07` 的完整 clean clone、12 个 CLI/Guard 交叉目标、credential-free smoke、CI `29754127548` 8/8 与 CodeQL `29754135162` 3/3 已通过。下一交付门槛是干净节点和 macOS/Windows service transaction 证据。
- Reasonix 一级上游已代码级审至 `43993f5a`；插件 Skill 的 MCP package provenance、唯一 canonical alias 解析、runtime binding 与权限/Hook/Evidence 身份已吸收，Provider schema 不暴露别名。
- 主分支只保留 Go/Wails 产品；旧 Hermes/Python/Electron/TUI/plugin/test/package 树已删除，参考机制只从 `F:\code-reference` 按治理规则吸收。
- 已有 24 个内置工具和 6 目标交叉编译；完成声明必须以测试、真实交互或发布证据为准。
