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

## 明确不做

该 workflow 不做以下事情：

- 不创建 GitHub Release。
- 不写 updater metadata。
- 不上传 R2/S3/Pages/其他云存储。
- 不发布 npm/Homebrew。
- 不读取 signing secrets。
- 不声明产物已经适合普通用户安装。

## 风险与后续验证

Wails Desktop 是 CGO + 原生 webview，必须在真实 runner 上验证。当前 workflow 是 candidate pipeline，不是完成声明。M0 的“native Desktop candidate”应在以下证据齐备后才能勾选：

1. 手动触发 `Desktop candidate` workflow。
2. 三个平台 job 都成功。
3. artifact 文件名与 `scripts/desktop-build.sh` 约定一致。
4. Linux/Windows/macOS 至少各完成一次下载解包或安装冒烟。
5. 若进入 canary/stable，再补签名、notarization、校验、更新元数据和人工审批。
