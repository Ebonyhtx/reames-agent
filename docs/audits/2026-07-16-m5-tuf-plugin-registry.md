# M5 TUF 插件 Registry 信任链审计

> 日期：2026-07-16
> 范围：可选 registry 的客户端信任、安装绑定、CLI/Desktop 发现、证据持久化和运营边界
> 结论：客户端信任机制已实现并有确定性攻击回归；真实运营 registry 仍未完成

## 问题

此前插件可从本地目录或 GitHub HTTPS 安装，虽记录 commit revision、重算
`sha256-tree-v1` 并强制两阶段审批，但没有项目拥有的签名发现信任链。被删除的旧
`pluginpkg.Registry` 只是下载普通 JSON；它既没有调用方，也不能防止镜像篡改、metadata
rollback/freeze、密钥轮换失误或签名发布者被冒充，保留会形成危险的“registry 已有”错觉。

参考审查确认：DeepSeek Reasonix 与 Reames Lite 没有可直接继承的强 registry 信任链；
Claude Code marketplace 的发现体验可参考，但不能替代签名验证。实现采用官方
[go-tuf/v2](https://github.com/theupdateframework/go-tuf) 与
[TUF 规范](https://theupdateframework.github.io/specification/draft/)，而不是自制签名协议。

## 已实现合同

1. 新增 `internal/pluginregistry`。没有默认 endpoint 或 TOFU；metadata URL、targets URL、
   bootstrap root 和 index target 只能来自用户全局配置。相对 root 位于 Reames home；endpoint/
   bootstrap-root 字段只从进程环境展开，`index_target` 按字面值使用，项目 TOML 和项目 `.env`
   都不能替换 endpoint/root。
2. bootstrap root 必须带外取得。client cache namespace 同时绑定 metadata URL 与 bootstrap
   root SHA-256；rotated root/timestamp/snapshot/targets 持久化并由进程 mutex + OS file lock
   串行。删除 cache 会重置已学习 rollback 状态，因此不是普通恢复步骤。
3. 官方 updater 顺序验证连续 root rotation、签名阈值、metadata expiry、rollback、freeze、
   mix-and-match 和 consistent snapshot target hash/length。metadata/target 响应有上限，
   HTTPS 为默认要求，HTTP 仅允许 loopback；重定向不能改变单次请求 origin。
4. `plugins.json` 使用严格 schema v1、拒绝未知字段/重复名字/超量条目。签名 entry 只接受
   canonical `https://github.com/<owner>/<repo>`、完整 40 字符 commit、精确
   `sha256-git-tree-v1`、已知权限和 portable subpath；display/path 字段有长度边界，并拒绝终端控制、
   Unicode bidi/format 与行段分隔字符。registry 索引、state 与安装请求复用可移植 ASCII 名称
   身份，大小写别名、尾点和 Windows 设备保留名在内容物化前拒绝。
5. registry provenance assertion 必须与 entry 的 source/subpath/revision/digest 完全一致。
   可选 attestation target 由 TUF 验证 hash/length；实现准确标记为
   `tuf-attestation-target-integrity-verified`，不解析或声称验证 DSSE signer、SLSA predicate policy、
   transparency log 或 SLSA level。
6. `registry:<name>` 只进入 plugin 安装路径。客户端隔离 system/global Git config、filters、
   replace refs 和交互凭据后精确 fetch full commit、detached checkout 并核对 HEAD；从 raw Git
   blobs、portable paths 与 Git executable intent 重算跨平台 source digest，再核对 manifest
   name/version/permissions；安装 generation 另算本机 `sha256-tree-v1`。
   preview 与 apply 各自重新 refresh/clone/inspect；planId 绑定 root、认证 entry digest、
   source revision、canonical source digest、本机 tree digest、权限、provenance/attestation
   evidence；任何规范化后的签名声明或 release 内容语义变化都要求新预检。
7. registry 名、metadata URL、root version、bootstrap-root digest、认证 entry digest、
   provenance status 和 attestation digest 随 active/previous generation 落盘，在 update 和
   rollback 后保留。CLI `registry refresh/search/show/digest`、CLI/模型 install_source、Desktop
   signed-registry 搜索/选择/预检共享同一 resolver；未配置 registry 时仅 registry 路径
   fail closed，不破坏本地/GitHub 直接安装。
8. 删除未使用且无签名的 `internal/pluginpkg/registry.go`，避免安全能力重复和降级入口。
9. 保存 go-tuf Apache-2.0 LICENSE/NOTICE；CLI GoReleaser archive 与 Desktop 的 macOS app、
   Windows portable/installer、Linux tar/deb 都携带项目和 go-tuf legal files，发布合同与
   Desktop artifact checker 防止后续漏包。
10. 根模块与 Desktop 模块固定 Go 1.26.5 build toolchain；它包含本轮 `govulncheck` 命中的
    GO-2026-5856 与 GO-2026-4970 标准库修复，候选不得由更旧的 1.26 patch 构建。

## 威胁回归

确定性 TUF fixture 覆盖：

- 有效 refresh/resolve 与 attestation target hash；
- 被篡改 index target 的 hash 拒绝；
- persistent metadata 下的 rollback 拒绝；
- 顺序 root rotation 与 rotated root cache 重用；
- expired timestamp 拒绝；
- provenance 不绑定、未知字段、控制/bidi 字符与非 canonical GitHub URL 拒绝；
- registry/state 中的大小写名称碰撞、尾点和 Windows 设备保留名拒绝，且安装在物化前失败；
- HTTP 非 loopback、项目 TOML/`.env` trust override 与未配置 registry 拒绝；
- manifest name/version/permission、canonical source digest 不一致拒绝；
- preview 后 root version 或认证 entry/source/provenance 的语义变化使 planId/apply 失效；
- registry evidence 经安装、更新、previous generation 和 rollback 双向保持；
- Desktop/CLI 未配置边界与前端 search/select/evidence/preview 交互。

## 当前证据

本批最终候选已在本地 clean clone 通过 root build/vet/internal 全测、Desktop
build/vet/full test、前端 frozen install/`test:all`/production build/bundle budget，以及
文档/公开/部署/发布合同。clean clone 同时暴露并关闭了 Windows checkout 下 `dist/.gitkeep`
的 CRLF/LF 伪修改：占位文件和 Vite 恢复内容现在均为 0 字节，构建后工作树保持干净。
远端 CI/CodeQL 仍以集中 push 后的实际结果为准。依赖采用 `go-tuf/v2 v2.4.2`；stripped Windows CLI
相对前一提交增加约 2.41 MiB（5.65%），这是官方 TUF metadata/updater 及其签名依赖的
成本；本批已记录并评估为可接受，仓库目前没有 CLI 二进制大小的硬阈值门禁。

核心复核命令：

```text
go test ./internal/config ./internal/pluginregistry ./internal/installsource ./internal/pluginpkg -count=1
go test ./internal/cli -run PluginRegistry -count=1
go test ./internal/... -count=1
desktop: go test . -run "PluginRegistry|DecodePluginOperationPreserves" -count=1
frontend: capabilities-panel-actions + typecheck
```

## 未完成与不扩大声明

- 本仓库不提供或默认信任公开 registry endpoint；没有生产 HSM、私钥托管、离线
  root/targets threshold ceremony、online role 凭据隔离、透明 monitor 或 compromise drill。
- TUF 认证 registry operator 的发布决定，不证明签名插件无恶意；用户仍必须审核权限，
  package sandbox 仍允许网络且无三平台统一硬 CPU/RSS quota。
- TUF cache 假设 Reames home 不被本机用户、管理员、调试器或其他高权限进程篡改；这类
  本地特权攻击不在 registry client 能单独解决的范围。
- 可选 attestation 当前只是 TUF-authenticated bytes。只有增加独立 identity/predicate
  policy verifier 并取得真实发布证据后，才可声明 DSSE/SLSA provenance 验证。
- M5 仍需最新远端 CI/CodeQL 证据；长期 GOAL 继续 active。

运营仓库、角色阈值、发布顺序、轮换和泄露恢复程序见
[插件 Registry 运维](../PLUGIN_REGISTRY_OPERATIONS.zh-CN.md)。
