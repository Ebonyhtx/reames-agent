# Hermes Gateway 参考审计

> 日期：2026-07-09  
> 目的：校正 Reames 云端/社交通道形态，避免把用户想要的 Hermes 式部署误写成 serve/Web-first。

## 已阅读的参考文件

本次审计直接查看了 `F:\code-reference\Hermes` 中的以下实现：

- `AGENTS.md`：项目结构、gateway 与 CLI/Desktop/TUI 的关系说明。
- `hermes_cli/subcommands/gateway.py`：`hermes gateway run/start/stop/restart/status/install/uninstall/setup` 命令面。
- `hermes_cli/gateway.py`：gateway 进程发现、systemd/launchd 生命周期、状态和安装逻辑。
- `hermes_cli/gateway_windows.py`：Windows Scheduled Task + Startup folder fallback。
- `hermes_cli/service_manager.py`：systemd、launchd、Windows、s6 的抽象 service manager。
- `gateway/run.py`：消息网关 daemon 主循环、adapter 启动、Agent session/cache 管理。
- `gateway/platform_registry.py`：平台 adapter 注册表和 plugin adapter 扩展。
- `gateway/platforms/ADDING_A_PLATFORM.md`：新增平台 adapter 的完整契约。
- `plugins/platforms/feishu/adapter.py`：飞书/Lark adapter 的连接、鉴权、交互卡片、媒体和安全边界。
- `scripts/install.sh` / `scripts/install.ps1`：Linux/macOS/Termux/Windows 一键安装、配置、gateway service 引导。

## 关键结论

Hermes 的形态不是“CLI 上运行一个 bot 命令”，而是四个并列入口共享 Agent 能力：

```text
Hermes core agent
├─ CLI：人工终端交互
├─ Gateway daemon：Telegram/Discord/Slack/WhatsApp/Weixin/Feishu/QQ 等社交通道
├─ Desktop：本机桌面 UI，可连接后台 gateway/RPC 能力
└─ Serve/Dashboard/TUI：可选控制面，不是社交通道的前置条件
```

其中 gateway 是独立后台服务：

- `gateway run`：前台运行，适合 Docker、WSL、Termux、调试。
- `gateway install`：安装为系统后台服务。
- `gateway start/stop/restart/status`：管理已安装服务。
- Linux 使用 systemd 或容器内 s6。
- macOS 使用 launchd。
- Windows 使用 Scheduled Task，失败时 fallback 到 Startup folder。
- 安装脚本会引导用户配置 gateway，并可选择立即后台启动。

CLI 和 gateway 互不占用：

- 用户可以在服务器 SSH 里运行 CLI。
- gateway 同时在后台接收飞书、微信、QQ、Telegram 消息。
- Desktop 可作为本机 UI，但不是 gateway 的运行条件。
- gateway 内部为每个远端 chat/user/thread 创建或复用 Agent session。

## 对 Reames 当前状态的纠偏

Reames 当前已有：

- `internal/bot`：`BotGateway`，管理 controller、session、审批、事件渲染和 adapter。
- `internal/botruntime`：平台启用、连接配置、路由、session mapping。
- `internal/bot/feishu`、`internal/bot/qq`、`internal/bot/weixin`、`internal/bot/telegram` 等 adapter。
- `reames-agent gateway run`：前台启动 social gateway。
- `reames-agent bot start`：兼容旧命名的前台 bot gateway 入口。
- Desktop 内部也有 bot runtime 相关设置和状态。

本次纠偏后，Reames 已补上第一层 Hermes 式 gateway 命令面：

- `reames-agent gateway run`：前台运行 gateway daemon。
- `reames-agent gateway install/start/stop/restart/status/uninstall`：后台服务生命周期命令。
- `internal/gatewayservice`：跨 Linux systemd、macOS launchd、Windows Scheduled Task 的 service manager 计划渲染和用户级执行。

仍然缺的 Hermes 式产品闭环：

- 没有一键安装脚本把 CLI、Desktop 可选安装、gateway service 配置串起来。
- 还没有 `gateway setup` 向导把飞书/微信/QQ/Telegram 凭据、allowlist 和 workspace route 串起来。
- 文档之前把 `serve` 写得太靠前，容易误解成云端部署主入口；现在已改为 CLI/Gateway 并列入口。

## Reames 应采用的目标形态

Reames 的目标应改成：

```text
Reames Agent core
├─ CLI/TUI：本机或服务器 SSH 人工交互
├─ Gateway service：独立后台服务，承载飞书/微信/QQ/Telegram 等社交通道
├─ Desktop：电脑本地 UI，可多运行一个桌面壳
├─ Serve/Web/API：可选远程控制面
└─ Background workers：上游研究、遥测反馈、定时任务
```

命令语义建议：

- 当前保留：`reames-agent bot start --channels feishu` 作为前台调试/兼容入口。
- 当前新增：`reames-agent gateway run`：前台运行 gateway daemon。
- 当前新增：
  - `reames-agent gateway install`：安装 OS 后台服务。
  - `reames-agent gateway start|stop|restart|status|uninstall`：管理后台服务。
- 新增目标：
  - `reames-agent gateway setup`：配置平台、allowlist、工作区映射和凭据。
- 或者如果保持 `bot` 命名，则必须引入 `bot run` 与 `bot service ...`，避免 `bot start` 同时表示前台运行和服务启动。

从产品语义看，建议恢复 `gateway` 作为社交通道后台服务的正式概念；“bot” 是当前实现包名和前台命令，不应代表整个服务管理面。

## 下一步实施优先级

1. 完成 service manager 实战验证：
   - 在干净 Linux 服务器验证 user systemd；
   - 在 Windows 验证 Scheduled Task；
   - 在 macOS 验证 launchd；
   - 容器：后续 s6 或 supervisor。
2. 一键安装：
   - Linux/macOS：`install.sh` 安装单二进制、配置 home、可选安装 gateway service；
   - Windows：`install.ps1` 安装 CLI、可选 Desktop、可选 Scheduled Task gateway。
3. `gateway setup`：
   - 配置平台凭据；
   - 配置 allowlist / pairing；
   - 配置默认 workspace 和 routes。
4. 文档和 CI：
   - 部署文档必须把 CLI、Gateway service、Desktop、Serve 分开；
   - CI 增加 gateway service contract 检查；
   - 不再把 `serve` 当作社交通道运行前提。

## 本次文档修正原则

在代码实现 service manager 前，文档应明确区分：

- “当前可用”：`reames-agent gateway run` 前台运行 gateway；`reames-agent bot start` 保持兼容。
- “目标形态”：独立 gateway service 后台运行，与 CLI/Desktop/Serve 隔离。

这比继续把 `bot start` 写成云端主路径更接近用户需求，也更接近 Hermes 的真实架构。
