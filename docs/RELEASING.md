# Reames Agent 发布流程

> 状态：候选工件验证已启用；生产发布暂未启用

## 当前安全边界

仓库从 Reasonix 继承的发布流程曾绑定上游维护者、npm、Homebrew、R2、域名和签名凭据。这些目标不属于当前项目的已确认发布基础设施，因此已经移除自动 tag 发布 workflow。

在完成所有权配置前：

- 推送 `v*`、`npm-v*` 或 `desktop-v*` tag 不会发布任何内容；
- 不向 npm、Homebrew、Cloudflare R2 或第三方更新服务写入；
- 不创建 GitHub Release；
- 只允许手动运行不含发布权限的候选工件构建。

## CLI 候选工件

Actions → **Release candidate** → **Run workflow**。

该 workflow 使用 GoReleaser snapshot 构建并上传 14 天保留的候选工件：

```text
darwin/amd64
darwin/arm64
linux/amd64
linux/arm64
windows/amd64
windows/arm64
SHA256SUMS
```

候选工件只用于检查文件名、可执行性、校验和与跨平台构建，不进入任何公开分发渠道。

## 启用生产发布前的门槛

1. 确认 GitHub Releases 为当前仓库 `Ebonyhtx/reames-agent` 所有。
2. 决定版本号来源和 CLI/Desktop/npm 是否共享版本。
3. 创建并验证项目自己的签名密钥；私钥只进入受保护 environment。
4. 明确 npm package、Homebrew tap、下载域名和对象存储的所有者。
5. Desktop updater、崩溃报告和遥测不得继续指向上游基础设施。
6. 先完成一次不发布的 native Desktop 三平台打包。
7. 建立 canary environment 和人工审批，再允许稳定发布。
8. 对发布后的安装、升级、回滚和校验失败执行端到端验证。

## 计划中的发布通道

| 通道 | 作用 | 当前状态 |
|---|---|---|
| Candidate | CI 工件；不公开发布 | CLI 已启用 |
| Canary | 维护者/测试者主动安装 | 未启用 |
| Stable | 面向普通用户 | 未启用 |

生产发布启用后仍遵循“小范围 canary → 验证 → 人工批准 stable”，不得仅凭 tag 自动把未经验证的构建推给用户。
