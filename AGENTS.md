# Reames Agent

Go 1.25+ 多平台 AI 编程助手。基于 DeepSeek Reasonix (MIT)，融合 9 个参考项目优点。

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
site/             # Astro 文档站点（遗留，非核心产品）
workers/          # Cloudflare Workers（遗留，非核心产品）
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

# 品牌残留检查（必须为 0）
grep -rn 'reasonix\|Reasonix' --include='*.go' -l | grep -v 'reames-agent' | wc -l

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

项目源流：`esengine/DeepSeek-Reasonix` 的 `main-v2` 是主源码上游；`F:\code-reference` 下其他仓库只提供机制与体验参考；`F:\Reames-Lite` 是项目前身和契约参考。详细规则见 `docs/REFERENCE_GOVERNANCE.md`。

| 项目 | 路径 | 复用方向 |
|---|---|---|
| DeepSeek Reasonix | `F:\code-reference\DeepSeek-Reasonix` | 源码基座（Go, Wails, Bubble Tea） |
| Hermes | `F:\code-reference\Hermes` | 频道/社交集成、错误分类器 |
| Codex CLI | `F:\code-reference\codex` | App-Server 协议、Hook 系统、LSP Delta |
| MiMo Code | `F:\code-reference\MiMo-Code` | 设计系统、OKLCH 颜色工具 |
| Impeccable | `F:\code-reference\impeccable` | 品牌设计语言 |
| Scream Code | `F:\code-reference\scream-code` | 主题系统、Goal Loop |
| AgentArk | `F:\code-reference\AgentArk` | 安全架构、Intent Classifier |
| Claude Code | `F:\code-reference\claude-code` | 插件生态/市场 |
| Kimi Code | `F:\code-reference\kimi-code` | 桌面 Shell 设计 |
| Reames Lite | `F:\Reames-Lite` | 旧版 Python 项目（合约参考） |

## 当前状态

- 产品方向以 `docs/PROJECT.md` 为准，执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。
- 当前处于 M0“基线可信”：核心、Desktop、前端本地基线已恢复，等待远端 CI 和干净 clone/发布验证。
- 下一里程碑是 M1“真实任务闭环”：真实 API、原生 Desktop、工具审批、文件变更和会话恢复。
- 已有 24 个内置工具和 6 目标交叉编译；完成声明必须以测试、真实交互或发布证据为准。
