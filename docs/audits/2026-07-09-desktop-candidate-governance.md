# Desktop candidate governance audit

Date: 2026-07-09

## 背景

M0 “基线可信”仍缺一项：三平台 native Desktop candidate 打包。仓库已有 Wails 桌面实现、`desktop/wails.json` 和 `scripts/desktop-build.sh`，但此前只有 CLI 的 `Release candidate` workflow；桌面只能本地手动打包，没有统一的 GitHub Actions candidate 入口。

## 本轮变化

新增 `.github/workflows/desktop-candidate.yml`：

- 手动触发：`workflow_dispatch`。
- 权限：`contents: read`。
- 目标：
  - `linux/amd64` on `ubuntu-24.04`
  - `windows/amd64` on `windows-latest`
  - `darwin/universal` on `macos-latest`
- 构建入口：统一调用 `scripts/desktop-build.sh`。
- 产物：只上传 GitHub Actions artifact，保留 14 天。

本轮同时把 `desktop/wails.json` 的 author 从继承上游的 `esengine` 改为 `Reames Agent Contributors`。

后续远端验证发现 Windows runner 虽然通过 Chocolatey 安装了 NSIS，但 `makensis` 未进入 PATH，Wails 在创建 installer 阶段失败。workflow 已显式把 `C:\Program Files (x86)\NSIS` 写入 `GITHUB_PATH` 并运行 `makensis -VERSION` 作为早失败探针；`scripts/desktop-build.sh` 也会在 Windows 下探测常见 NSIS 路径，避免本地或 CI 环境出现同类漂移。

## 远端验证结果

最终修复后重新手动触发 `Desktop candidate`：

- Run: https://github.com/Ebonyhtx/reames-agent/actions/runs/29015844761
- Commit: `ee34223e0b4d69422fb0c9de7e27a222b9326972`
- `linux/amd64`: success，artifact `reames-agent-desktop-candidate-linux-amd64-ee34223e0b4d69422fb0c9de7e27a222b9326972`，28,712,191 bytes。
- `windows/amd64`: success，artifact `reames-agent-desktop-candidate-windows-amd64-ee34223e0b4d69422fb0c9de7e27a222b9326972`，36,433,945 bytes。
- `darwin/universal`: success，artifact `reames-agent-desktop-candidate-darwin-universal-ee34223e0b4d69422fb0c9de7e27a222b9326972`，83,294,895 bytes。

内容级 artifact smoke：

- Windows portable zip 包含 `Reames Agent.exe` 和 `reames-agent-update-helper.exe`。
- Linux tarball 包含 `reames-agent-desktop`；Linux `.deb` 包含 `debian-binary`、`control.tar.gz`、`data.tar.gz`。
- macOS arm64/amd64 zip 均包含 `Reames Agent.app/Contents/MacOS/reames-agent-desktop`、`Info.plist` 和 `iconfile.icns`；universal `.dmg` 存在。

该检查已固化为 `scripts/check_desktop_artifacts.py`，并用 `scripts/test_check_desktop_artifacts.py` 覆盖 Windows update helper 缺失等回归。

这证明三平台 candidate 打包流水线已经建立并能产出结构正确的短期 artifact；仍不等价于稳定 release，也尚未证明普通用户安装/启动体验。

Windows portable 启动 smoke：

- Artifact: `Reames Agent-windows-amd64.zip` from run `29015844761` / commit `ee34223e0b4d69422fb0c9de7e27a222b9326972`。
- 环境：Windows，本机临时目录，隔离 `REAMES_AGENT_HOME`，并设置 `REAMES_AGENT_DESKTOP_DISABLE_WEBVIEW2_GPU=1`。
- 结果：`Reames Agent.exe` 启动后 12 秒仍存活，未立即崩溃；随后结束 smoke 进程。
- 证据：临时 Agent home 写入了 `desktop-tabs.json`、`desktop-window.json`、环境探测 cache 和初始 session metadata。
- 边界：这不是安装器 smoke，也没有验证真实用户点击路径、模型调用或自动更新。

## 明确不做

该 workflow 不做以下事情：

- 不创建 GitHub Release。
- 不写 updater metadata。
- 不上传 R2/S3/Pages/其他云存储。
- 不发布 npm/Homebrew。
- 不读取 signing secrets。
- 不声明产物已经适合普通用户安装。

## 风险与后续验证

Wails Desktop 是 CGO + 原生 webview，必须在真实 runner 上验证。当前 workflow 是 candidate pipeline，不是 stable release。三平台打包流水线已在 run `29015844761` 验证通过；面向用户发布前仍需以下证据：

1. Linux/macOS 至少各完成一次安装/启动冒烟。
2. Windows NSIS installer 完成一次安装/启动冒烟。
3. 若进入 canary/stable，再补签名、notarization、校验、更新元数据和人工审批。

## 原生安装/启动 smoke 扩展

2026-07-10 在同一手动 candidate matrix 中加入了安装后 smoke，等待下一次远端运行取证：

- Linux runner 安装本轮实际 `.deb`，检查二进制、desktop entry 和图标，再通过 `dbus-run-session` + Xvfb 启动安装后的 `/usr/bin/reames-agent-desktop`；`xdotool` 必须重复发现可见的 Reames Agent 窗口。
- macOS runner 挂载本轮实际 universal `.dmg`，把 `Reames Agent.app` 复制到 runner 临时 Applications，验证 ad-hoc 签名和 `x86_64` / `arm64` 双架构，再启动 bundle 内二进制。
- Windows runner 对本轮构建产物继续运行窗口消息泵、状态围栏和关闭路径 smoke。
- 三个平台都使用显式隔离 home、关闭更新检查、记录 artifact/executable SHA-256，并围栏检查默认 home、默认 cache 和 legacy OS support 路径未变化，最后上传 `desktop-*-native-smoke.json`。

这些自动化只证明候选的安装结构、启动存活、隔离状态和有界清理；不替代真实模型、审批或用户点击证据。Linux/macOS 条目在远端原生 runner 成功前保持未完成。

首次扩展 run `29070266501` 的 Linux 与 Windows jobs 成功：Linux JSON 记录实际 `.deb` 和安装后二进制 SHA-256、15 次可见窗口检查、`window-close` 与三类边界零变化；Windows JSON 记录 12 秒消息泵响应、`wm-close` 与边界零变化。macOS 已完成 DMG 挂载、app 复制和签名验证，但 `lipo` 的输入文件参数顺序错误导致在启动前失败；workflow 已按 Xcode CLI 语法改为 `lipo <binary> -verify_arch x86_64 arm64`，等待重跑。

修复后的 run `29070605386` 三个平台 jobs 全部成功。macOS JSON 记录实际 DMG SHA-256 `9EC1C64DD9581EA7AC1E069BF9E64DADCF220CBD229FA33A3565AC3A2F100364`、安装后二进制 SHA-256 `0D35D19F40514FA882101CF110F8E1203AF39F95A8389AC40895471B69F50EBE`、12 秒存活、状态落盘、三类边界零变化和退出码 0；Linux 重跑同样成功。Linux/macOS 安装/启动条目据此完成。

Windows job 在上述 runs 中验证的是本轮 Wails build 产物，而不是 NSIS 安装后的文件。workflow 随后扩展为静默安装实际 NSIS 到 runner 临时目录，验证 per-user `InstallLocation`、主程序、update helper 与 uninstaller，针对安装后二进制运行同一隔离 smoke，最后静默卸载并检查文件和注册清理；该条目等待远端重跑。
