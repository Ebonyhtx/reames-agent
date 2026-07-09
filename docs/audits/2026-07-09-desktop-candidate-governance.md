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

修复后重新手动触发 `Desktop candidate`：

- Run: https://github.com/Ebonyhtx/reames-agent/actions/runs/29015200954
- Commit: `a860b32c3a0afa77cecfce481cfabc2408ebebd9`
- `linux/amd64`: success，artifact `reames-agent-desktop-candidate-linux-amd64-a860b32c3a0afa77cecfce481cfabc2408ebebd9`，28,712,860 bytes。
- `windows/amd64`: success，artifact `reames-agent-desktop-candidate-windows-amd64-a860b32c3a0afa77cecfce481cfabc2408ebebd9`，35,472,530 bytes。
- `darwin/universal`: success，artifact `reames-agent-desktop-candidate-darwin-universal-a860b32c3a0afa77cecfce481cfabc2408ebebd9`，83,293,412 bytes。

这证明三平台 candidate 打包流水线已经建立并能产出短期 artifact；仍不等价于稳定 release，也尚未证明普通用户下载安装体验。

## 明确不做

该 workflow 不做以下事情：

- 不创建 GitHub Release。
- 不写 updater metadata。
- 不上传 R2/S3/Pages/其他云存储。
- 不发布 npm/Homebrew。
- 不读取 signing secrets。
- 不声明产物已经适合普通用户安装。

## 风险与后续验证

Wails Desktop 是 CGO + 原生 webview，必须在真实 runner 上验证。当前 workflow 是 candidate pipeline，不是 stable release。三平台打包流水线已在 run `29015200954` 验证通过；面向用户发布前仍需以下证据：

1. Linux/Windows/macOS 至少各完成一次下载解包或安装冒烟。
2. 验证 artifact 文件内容与 `scripts/desktop-build.sh` 约定一致。
3. 若进入 canary/stable，再补签名、notarization、校验、更新元数据和人工审批。
