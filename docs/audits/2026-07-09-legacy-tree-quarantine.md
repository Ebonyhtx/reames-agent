# Legacy tree quarantine audit

Date: 2026-07-09

> 状态：历史决策，已由 `2026-07-17-repository-cleanup.md` 关闭。下述“短期不删除”仅描述
> 当时迁移阶段，不再代表当前仓库策略。

## 背景

`scripts/` 公开入口清理后，仓库根目录仍保留一整套继承自 Hermes/Python 时代的历史代码树。它们包含有价值的机制参考，例如多渠道 gateway、旧 TUI、skills、browser tools、Python tests 和旧打包元数据；但当前 Reames Agent 的权威产品路径已经迁移到 Go runtime：

- CLI：`cmd/reames-agent`
- 核心 runtime：`internal/`
- Gateway service：`internal/bot`、`internal/botruntime`、`internal/gatewayservice`
- Desktop：`desktop/`
- 安装/部署/上游追踪：`scripts/install.*`、`scripts/check_*`、`.github/workflows/*`

因此旧 Python/Hermes 树不能再作为当前官方运行入口出现。

## 当前发现

本轮审计确认以下目录或文件仍属于 legacy/reference 面：

- Python runtime/package：`agent/`、`tools/`、`gateway/`、`reames_cli/`、`cron/`、`tui_gateway/`
- Python/Node 旧入口：`cli.py`、`run_agent.py`、`mcp_serve.py`、`mini_swe_runner.py`、`batch_runner.py`
- 旧测试树：`tests/`
- 旧 TUI/前端 workspace：`ui-tui/`、`apps/`
- 旧 skill/plugin 资产：`skills/`、`optional-skills/`、`plugins/`、`providers/`
- Python package metadata：`pyproject.toml`、`uv.lock`
- Node root workspace metadata：`package.json`、`package-lock.json`

其中 `package.json` 仍显示 `hermes-agent`、指向 `NousResearch/Hermes-Agent`，并通过 `postinstall` 提示运行旧 Python 入口。这个属于公开入口误导，本轮已改为 Reames 私有 root workspace 元数据，并加入 public-readiness 门禁。

## 决策

短期不直接删除整棵 legacy tree。原因：

1. 用户明确希望吸收 Hermes 和其他参考项目的优点，旧树中仍可能有可迁移机制。
2. 旧树内部引用密集，一次性删除会制造大量无关差异，降低 CI 和审查可读性。
3. 当前首要目标是让 Go 版 Reames 的官方入口可信，而不是把历史参考一次性清空。

短期规则：

- 旧 Python/Hermes 目录只能作为参考或待迁移资产。
- 新文档、README、安装器、CI、部署说明不得把旧 Python 入口描述为当前官方运行方式。
- 任何新的官方能力都应落到 `cmd/`、`internal/`、`desktop/`、`deploy/`、`docs/` 和当前 `scripts/check_*` 面。
- root `package.json` 只能作为 private workspace 元数据，不得指向旧 Hermes 仓库或推荐 `python run_agent.py`。
- Git 不得跟踪 `reames-agent.exe`、`bin/`、`dist/` 等构建产物；发布产物只能通过 release candidate / GoReleaser / CI artifact 产生。

## 后续迁移队列

建议按以下顺序逐步处理 legacy tree：

1. Gateway：对照 `F:\code-reference\Hermes` 和旧 `gateway/`，把仍缺失的渠道 envelope、错误分类、重连、媒体处理迁移到 Go `internal/bot*`。
2. TUI：对照 `ui-tui/` 和当前 Bubble Tea CLI，决定是否保留旧 Ink TUI 的交互机制，避免双 TUI 长期并存。
3. Skills/plugins：从 `skills/`、`optional-skills/`、`plugins/` 提炼 manifest、权限、市场和诊断模型，迁移到 Go `internal/skill` / `internal/pluginpkg`。
4. Browser/tools：从 Python `tools/` 中提炼浏览器、文件、媒体和外部服务能力，逐项映射为 Go builtin tools 或 MCP 插件。
5. Tests：把仍有价值的旧 Python 行为测试改写为 Go/desktop/frontend 契约测试；其余删除。
6. Packaging：移除或隔离 `pyproject.toml`、`uv.lock`、旧 Node workspace 中不再需要的发布路径。

每一步都必须满足：先证明无当前官方入口依赖，再迁移或删除；迁移后补契约测试；提交前跑相应 CI 子集。
