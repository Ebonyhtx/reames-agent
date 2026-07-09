# Reames Agent 发布流程

> 状态：候选工件验证已启用；生产发布暂未启用
> 版本策略：单一 SemVer 标签，人工批准发布

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

## 版本号来源

Reames Agent 使用一个项目级版本号，格式为 SemVer 标签：

```text
vMAJOR.MINOR.PATCH
vMAJOR.MINOR.PATCH-rc.N
```

规则：

- CLI、Desktop、server、bot gateway 和插件示例共享同一个项目版本号。
- Go 二进制版本由构建时 `-ldflags "-X main.version={{ .Tag }}"` 注入。
- 普通开发构建显示 `dev`；候选 snapshot 只用于验证，不代表稳定版本。
- 只有 `v*` 标签可作为正式版本来源；`npm-v*`、`desktop-v*` 等继承标签不再使用。
- 发布前必须把 `CHANGELOG.md` 的 `Unreleased` 内容移动到对应版本段。
- 如果未来拆出 npm、Homebrew、Desktop updater 或插件市场版本，它们必须在本页单独声明映射关系，不能隐式复用继承流程。

## 变更日志

`CHANGELOG.md` 是人工维护的用户可读变更日志，不由提交历史自动替代。

每个版本至少包含：

- 安全修复；
- 破坏性变化；
- 用户可见功能；
- 部署、配置、升级和回滚变化；
- 重要修复与已知问题。

合并到 `main` 的工程性变更可以先记录在 `Unreleased`。发布候选前，维护者把该段归档到新版本号，并确认发布说明与候选工件一致。

## 签名与校验策略

当前候选工件只生成 `SHA256SUMS`，用于维护者下载后校验，不作为公开供应链保证。

启用生产发布前必须完成：

1. 对 CLI archives 和 `SHA256SUMS` 建立项目自有签名。优先使用 GitHub OIDC + Sigstore/cosign 的 keyless signing；如果改用长期私钥，私钥只能放在受保护 environment。
2. Desktop 平台签名单独处理：
   - macOS：Developer ID signing + notarization；
   - Windows：代码签名证书；
   - Linux：包校验和与可选仓库签名。
3. 发布说明必须包含校验方式和回滚方式。
4. 升级器只能信任 Reames Agent 自有 release endpoint、签名和校验和，不能继续使用 Reasonix/Hermes 继承基础设施。
5. 签名失败、校验失败或 updater 元数据不一致时，客户端必须 fail closed。

## 启用生产发布前的门槛

1. 确认 GitHub Releases 为当前仓库 `Ebonyhtx/reames-agent` 所有。
2. 按本页版本号来源和变更日志规则准备候选版本。
3. 创建并验证项目自己的签名策略；私钥只进入受保护 environment，或使用 OIDC keyless signing。
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
