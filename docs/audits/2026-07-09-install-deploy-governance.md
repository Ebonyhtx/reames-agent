# 安装与部署统一性审计

> 日期：2026-07-09  
> 范围：DeepSeek Reasonix 官方上游、Hermes 参考项目、Reames 当前安装/部署入口  
> 结论：Reames 不能简单复制任一参考项目的安装方式；需要以“单 Go 二进制 + CLI/Gateway/Desktop/Serve 并列入口”为主线，分别吸收 Reasonix 的发布链路和 Hermes 的后台 gateway 机制。

## 检查过的来源

- Reames 当前仓库：
  - `README.md` / `README.zh-CN.md`
  - `docs/DEPLOY.md`
  - `Dockerfile`
  - `docker-compose.yml` / `docker-compose.windows.yml`
  - `deploy/systemd/reames-agent.service`
  - `scripts/install.sh` / `scripts/install.ps1` / `scripts/install.cmd`
  - `.github/workflows/ci.yml`
- DeepSeek Reasonix 官方上游：
  - `F:\code-reference\DeepSeek-Reasonix`
  - 已 fast-forward 到 `origin/main-v2`：`0e0cb63c`
  - `README.md`
  - `docs/SPEC.md`
  - `docs/RELEASING.md`
  - `docs/index.html`
- Hermes 参考项目：
  - 见 [Hermes Gateway 参考审计](2026-07-09-hermes-gateway-reference.md)

## Reasonix 官方安装/部署形态

Reasonix 上游现在的公开安装主线是成熟发布分发：

- `npm i -g reasonix`：跨平台安装入口，拉取预构建原生 Go 二进制。
- `brew install esengine/reasonix/reasonix`：macOS Homebrew 入口。
- GitHub Releases：发布 `darwin|linux|windows × amd64|arm64` 预构建归档和 `SHA256SUMS`。
- Desktop 发布：独立 `desktop-v*` 标签，R2 latest/canary 指针和桌面更新网关。
- 构建契约：`CGO_ENABLED=0` 单二进制，6 平台交叉编译。
- Server/Web：`reasonix serve`，配置中 `[serve]` 默认 loopback，远程暴露必须启用 token/password。
- IM Bot：仍是 `reasonix bot start` 前台长期进程；没有 Hermes 式 `gateway install/start/status` service manager 命令面。

因此 Reasonix 的可吸收点主要是：

- 预构建 release 产物；
- npm/Homebrew 包装层；
- stable/canary/tag 发布治理；
- 桌面 R2 分发与更新通道；
- `serve` 安全默认值。

不应直接照搬的点：

- `reasonix bot start` 作为唯一 IM 长期运行入口；
- Reasonix 官方域名、npm 包名、Homebrew tap、R2/CDN 路径；
- 已发布稳定渠道的假象。Reames 目前仍处于 pre-stable。

## Hermes 参考项目可吸收点

Hermes 在社交通道部署上更接近用户目标：

- CLI 与 gateway daemon 进程隔离；
- `gateway run` 前台调试；
- `gateway install/start/stop/restart/status/uninstall` 后台服务生命周期；
- Linux systemd、macOS launchd、Windows Scheduled Task；
- 一键安装时可引导配置并安装 gateway service。

Reames 已吸收第一层：

- `reames-agent gateway run`
- `reames-agent gateway install/start/stop/restart/status/uninstall`
- `internal/gatewayservice`
- `scripts/install.sh` / `scripts/install.ps1` 可选安装 gateway service

仍需继续吸收：

- `gateway setup` 向导；
- 干净 Linux/Windows/macOS 实机服务验证；
- installer 与未来 release 二进制下载结合，而不是永久源码构建。

## Reames 当前统一部署原则

Reames 的正式入口应保持四条线并列，而不是互相依赖：

| 入口 | 当前命令 | 部署定位 |
|---|---|---|
| CLI/TUI | `reames-agent` / `reames-agent run` | 本机或服务器 SSH 主入口 |
| Gateway | `reames-agent gateway run` / `gateway install` | 飞书/微信/QQ/Telegram 后台社交通道 |
| Serve/Web/API | `reames-agent serve` | 可选远程控制面，默认 loopback + 鉴权 |
| Desktop | Wails 桌面应用 | 电脑本地 UI，不是服务器 gateway 的前置条件 |

安装方式当前统一为：

- 源码构建：`go build ./cmd/reames-agent`
- 一键源码安装：`scripts/install.sh` / `scripts/install.ps1`
- 容器部署：`Dockerfile` / `docker-compose*.yml`
- 服务部署：`deploy/systemd/reames-agent.service` 用于 `serve`；gateway service 由 `reames-agent gateway install` 渲染/执行

未来统一为：

- Reasonix-like：发布预构建 6 平台二进制、校验和、签名、npm/Homebrew 包装；
- Hermes-like：安装器可选配置并安装 gateway service；
- Reames-specific：所有命令、路径、文档、CI、环境变量使用 `reames-agent` / `REAMES_AGENT_*`。

## 本次发现的问题

1. `README.zh-CN.md` 仍写着旧命令 `reames-agent gateway start --channels feishu`，已改为 `gateway run` 和 `gateway install --dry-run`。
2. `README.zh-CN.md` 仍使用 `DEEPSEEK_API_KEY=sk-xxx` 示例，已改为 `replace-with-your-key` 并补 `REAMES_AGENT_SERVE_TOKEN`。
3. Reames installer 已从旧 Hermes installer 替换为 Reames 源码构建安装器，但还需要未来接入 release artifact 下载。
4. Reasonix 上游已经比本地参考仓库前进 5 个提交，已 fast-forward 到 `0e0cb63c`。

## 下一步

1. 给 Reames release 流程补“预构建二进制下载优先、源码构建 fallback”的 installer 设计。
2. 清理或隔离剩余旧 Hermes/Python 辅助脚本，避免它们被误认为正式 Reames 入口。
3. 把 Reasonix 的 release/npm/Homebrew 治理拆成可执行 Reames 里程碑，但在 public stable 前保持发布关闭。
4. 在干净 Linux 服务器上验证：
   - `scripts/install.sh --gateway --channels feishu`
   - `reames-agent gateway status`
   - `reames-agent run`
   - `reames-agent serve --health-check`
