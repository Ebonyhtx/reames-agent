# M5 Registry 运维审计与轮换演练

> 日期：2026-07-16
> 范围：公开 TUF metadata 的只读生产策略审计、轮换/泄露恢复合成演练与外部证据边界

## 结果

新增 `reames-agent plugin registry audit <repository> --root <root.json>`。它只读取已经装配的
TUF 仓库和从带外渠道取得的 bootstrap root，不访问网络、不生成私钥、不写仓库，也不读取
用户的 registry 配置。`--at` 可固定参考时间，输出 schema v1 JSON 以便仪式归档。

审计沿用 runtime client 相同的官方 `go-tuf/v2 trustedmetadata` 转移：

- bootstrap root 必须自签名；仓库内相同版本 root 必须与带外字节完全一致；
- bootstrap root 的解析路径必须在 repository 外，且不能与 `repository/metadata` 下任一
  文件是同一物理对象（包括外部硬链接）；root 由单次稳定文件句柄读取并用同一 `FileInfo`
  做 alias 比较，避免检查路径与实际验证字节之间的替换窗口；
- 后续 `<N>.root.json` 必须连续，并同时满足可信旧 root 和新 root 的阈值；
- final root 必须启用 consistent snapshots，root/targets 至少 2-of-3，snapshot/timestamp
  至少 1-of-1；四个顶层角色不得复用 key，metadata key ID 必须等于公钥 canonical ID；
- root/targets/snapshot/timestamp 分别限制最短剩余时间与最长未来到期窗口；
- timestamp → versioned snapshot → versioned targets 依次验证签名、version、length/hash 和
  expiry；metadata 拒绝重复 JSON key；
- `plugins.json` 和每个被引用 attestation target 必须以 TUF SHA-256 前缀文件存在、路径
  受 `os.Root` 约束且不穿越 symlink，并通过真实 length/hash；
- registry index 继续使用严格 schema、portable name/path、canonical Git source 与完整
  provenance 绑定，并限制 future/stale `updated` 时间。
- 成功报告记录排序后的四角色 public key IDs、final root/targets/snapshot/timestamp SHA-256、
  index/attestation 摘要、版本、到期和实际签名数，用于绑定精确仪式证据。

## 演练证据

合成演练使用进程内短生命周期 Ed25519 私钥，不把密钥写入仓库或测试输出：

1. v1 使用彼此独立的 root 3 keys、targets 3 keys、snapshot 1 key、timestamp 1 key，
   root/targets 阈值为 2；
2. v2 同时更换 root、targets、snapshot、timestamp key，root metadata 由旧 2-key quorum
   和新 2-key quorum 双签；最终 repository 由新 online/offline role keys 签名；
3. 从 v1 带外 root 审计 v1→v2，验证连续轮换、完整 metadata/target 链和 attestation；
4. 删除一个旧 root quorum 签名后审计 fail closed，模拟不能由新密钥单方面抹掉旧信任；
5. root version 断层/非 canonical 文件名、仓库内 bootstrap、外部 hardlink、root 1-of-1、
   跨角色 key reuse、500 天未来 root、index 篡改、symlink 和 `../` target 均分别失败。

这证明客户端和离线公开 metadata 审计机制能验证一次轮换/恢复发布，但不证明真实人员、HSM
或线上对象存储已经执行过该程序。

## 不扩大声明

每份成功报告强制保留 `externalRequired`，至少包括：

- 不同人员见证的 root/targets 离线仪式与 quorum 记录；
- 生产 HSM 或等价私钥托管证据；
- 原子 HTTPS 发布、freshness 监控和告警证据；
- 真实密钥轮换与 compromise-response 演练记录；
- 声明 builder identity/SLSA predicate 时的独立 DSSE/SLSA policy verifier。

因此 M5 的本地运维工具和确定性演练已前进，但“真实运营公开 registry”仍为 external-blocked，
仓库继续不提供默认 endpoint 或 TOFU。

## 同批 CI 清洁

此前远端 run `29487948296`/`29487948316` 虽全绿，但 GitHub 对 checkout、setup-go、
setup-python、setup-node 和 upload-artifact 的旧 major 报告 Node.js 20 弃用警告。本批依据
各 action 官方仓库的 `action.yml`，把 CI、CodeQL 支撑步骤、candidate、Upstream Watch 和
遗留 deploy workflow 迁移到 Node.js 24 majors，并新增 Python 合同按 action family 的最低
Node 24 major 阻止回退；合同覆盖 `.yml/.yaml`，未知 ref 或未经审计的 40 字符 pin 同样
fail closed，只有记录过的 Node 24 tag commit 可作为 immutable pin。

一手版本依据为官方 release/action metadata：[checkout v7.0.0](https://github.com/actions/checkout/releases/tag/v7.0.0)、
[setup-go v7.0.0](https://github.com/actions/setup-go/releases/tag/v7.0.0)、
[setup-python v6.3.0](https://github.com/actions/setup-python/releases/tag/v6.3.0)、
[setup-node v7.0.0](https://github.com/actions/setup-node/releases/tag/v7.0.0)、
[upload-artifact v7.0.1](https://github.com/actions/upload-artifact/releases/tag/v7.0.1)、
[github-script v9.0.0](https://github.com/actions/github-script/releases/tag/v9.0.0) 与
[pnpm/action-setup v6.0.9](https://github.com/pnpm/action-setup/releases/tag/v6.0.9)。

## 当前验证

本地已通过：root `go build ./...`、`go vet ./...`、`go test ./internal/...`；Desktop
build/vet/full test；前端 `test:all`/production build/bundle budget；121 项 Python 合同
（2 skipped）；文档/公开/部署/发布合同；actionlint；`pluginregistry`/CLI race；六目标
`CGO_ENABLED=0` CLI 与 Linux/macOS 两包八个测试二进制交叉编译。交叉产物只写入系统临时
目录。提交 `9295f8b` 的 clean clone 又重复通过 root、Desktop、前端与合同门禁，构建后
tracked 工作树保持干净。集中 push 后，普通 CI `29510215514` 的 8 个 jobs 与 CodeQL
`29510215449` 的 Go、JavaScript/TypeScript、Actions 3 个 jobs 全绿，两个 run 的 head SHA
均为 `9295f8bbb8163bc7a32f233a1a7f0f9fb54b6e8c`；远端日志未再出现 Node.js 20 弃用告警。
