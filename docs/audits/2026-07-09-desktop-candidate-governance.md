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

1. Linux/Windows/macOS 至少各完成一次安装/启动冒烟。
2. 若进入 canary/stable，再补签名、notarization、校验、更新元数据和人工审批。
