# 仓库 legacy tree 清洁收口审计

> 日期：2026-07-17
>
> 范围：当前 checkout、Git 索引、运行/CI/发布引用、许可证与品牌边界

## 结论

初始迁移曾把一整套 Hermes/Python runtime、Electron Desktop、旧 Ink TUI、plugins、skills、
Python tests、Python/Node package 元数据、Astro site 和 Cloudflare workers 留在主仓库，
并通过 `2026-07-09-legacy-tree-quarantine.md` 暂时隔离。当前 Go/Wails 主产品已经完成 M0–M5
所有可由仓库和 CI 验证的闭环；旧树不再被当前 CLI、Controller、Desktop、部署、发布或 CI
调用，继续保留只会扩大依赖、品牌、许可证和维护噪声。

本批因此关闭临时隔离：从当前树删除 4,049 个 tracked legacy 文件。删除不重写 Git 历史；
历史提交仍可恢复精确字节，后续机制研究使用 `F:\code-reference\Hermes`。

## 删除边界

删除的产品/运行面包括：

- Python runtime 与入口：`agent/`、`reames_cli/`、`gateway/`、`tools/`、`providers/`、
  `cron/`、`tui_gateway/`、`cli.py`、`run_agent.py`、`mcp_serve.py` 等；
- 旧 UI：`apps/` Electron/Tauri shell、`ui-tui/` Ink TUI、旧 root assets/locales；
- 旧扩展与研究资产：root `skills/`、`optional-skills/`、`plugins/`、`optional-mcps/`、
  `datagen-config-examples/`、旧 tests 和旧 hooks；
- 旧包与发布面：`pyproject.toml`、`uv.lock`、root `package.json`/`package-lock.json`、Nix、
  Homebrew、SignPath、旧 Docker s6 tree 与安装 wrapper；
- 无所有权服务：`site/`、`workers/` 及四个旧部署 workflow；
- 误导入口：`README.ur-pk.md`、`REAMES_AGENT.md`、`INTEGRATION_REFERENCE.md`。

## 保留边界

当前产品继续保留：

- `cmd/`、`internal/` 和 `desktop/` 的 Go/Wails/React 产品；
- `deploy/`、root Dockerfile/compose、当前 installer、candidate、CodeQL 与 upstream watch；
- `docs/` 历史审计、`benchmarks/`、`third_party/` 法律文件和 `.reames-agent/commands/`；
- DeepSeek Reasonix 的 MIT 归属、go-tuf Apache-2.0 LICENSE/NOTICE 和参考治理记录。

## 防回归

`scripts/check_public_readiness.py` 现在：

- 拒绝所有已删除 legacy 根目录、旧根入口和 worker/site workflows 再次被跟踪；
- 拒绝活跃代码、Desktop、部署、脚本和 workflow 中的 Hermes/Nous runtime 品牌；
- 只允许 `REASONIX.md` / `REASONIX.local.md` 作为明确的旧文件名兼容；
- 保持 build binary、telemetry、installer、Node.js action runtime 和发布边界门禁。

项目记忆的新默认和用户提示统一为 `AGENTS.md`；旧 `REASONIX.md` 仍可读取和迁移，但不再是
新项目推荐名称。

## 提交前本地验证

清洁后的保留树已按低并发顺序通过：

- root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`；
- `internal/control`：隔离单次和 `-count=10` 稳定性复验，排除并发门禁负载造成的 goleak 时序噪声；
- Desktop Go：`go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 300s`；
- React frontend：`corepack pnpm test:all`、`corepack pnpm build`，bundle budget 通过；
- 真实 Chromium plugin lifecycle smoke：install、enable、update、rollback、doctor、remove 全通过，
  `console_errors=[]`、`page_errors=[]`；
- 仓库脚本：124 个 Python 单元/合同测试、upstream issue reconciliation、`actionlint`、docs、
  public-readiness、deploy、release 和内置工具文档合同均通过；
- 运行与并发：无凭据 headless Gateway smoke 通过，`pluginregistry` 与 `cli` race test 通过；
- 发布形态：Linux、macOS、Windows 的 amd64/arm64 六个 `CGO_ENABLED=0` 目标全部编译通过；
- 依赖与差异：root/Desktop `go mod tidy -diff` 无差异，`git diff --check` 通过。

## 证据边界

删除前审计确认：

- 只有 `main` 一个本地/远端分支和一个 worktree；
- 旧树约占当前 checkout 的 70% 文件数和 80% tracked 字节；
- 当前 Go、Desktop、CI 和发布入口没有生产引用；
- 旧 worker/site workflow 仅自引用旧目录，并被固定在非当前 `main-v2` 分支；
- Git 历史和外部 Hermes 参考仓库可替代 checkout 内的参考副本。

上述本地门禁仍不能替代独立 clean clone 和最新远端 CI/CodeQL；二者必须基于本批最终提交
继续复验。本页不把本地删除或单一窄测试冒充最终交付证据。
