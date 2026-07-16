# 插件 Registry 运维

本文定义 Reames Agent 插件 registry 的运维合同。客户端机制已经存在，但项目目前不运营、
也不预配置公开 registry。只有部署方完成自己的密钥仪式、仓库发布、监控和事故演练后，
才能把某个实际部署称为可信 registry。

## 信任与仓库结构

Reames Agent 通过官方 [go-tuf/v2 客户端](https://github.com/theupdateframework/go-tuf)
遵循 [The Update Framework 规范](https://theupdateframework.github.io/specification/draft/)。
每个客户端必须从带外渠道取得初始 `root.json`；不能从同一个待验证 URL 下载 root，
再把这种行为称为验证。

首版支持的仓库结构：

```text
metadata/
  <N>.root.json
  timestamp.json
  <N>.snapshot.json
  <N>.targets.json
targets/
  <sha256>.plugins.json
  attestations/<sha256>.<name>.json       # 可选
```

root 必须启用 consistent snapshots。默认签名索引 target 是 `plugins.json`，用户也可配置
其他干净相对路径。metadata 与 targets 可使用不同 HTTPS origin，但每个请求都拒绝跨 origin
重定向；明文 HTTP 只允许 loopback 测试。

## 最低密钥策略

四个顶层角色必须使用独立密钥。下面是部署基线，不替代组织自身的威胁分析：

| 角色 | 建议保管方式 | 建议阈值 | 作用 |
|---|---|---:|---|
| root | 不同人员分别持有的离线设备 | 2-of-3 或更强 | 委派和轮换所有角色 |
| targets | 离线或受保护的 release 仪式 | 公开 registry 建议 2-of-3 | 授权索引和 attestation |
| snapshot | 隔离的在线 publisher | 1-of-1 | 绑定一致的 metadata 集合 |
| timestamp | 隔离的在线 publisher | 1-of-1 | 提供新鲜度并限制 freeze attack |

root 密钥必须离线且在地理或管理上隔离。不得把 root/targets 私钥放进 Web 服务器、对象
存储、CI 日志或 Reames Agent 源码树。snapshot/timestamp 应使用不同凭据和最小写权限。
离线仪式记录要保存公钥 ID、阈值、负责人、备份位置、创建时间、计划过期时间和撤销状态。

过期策略既要迫使监控生效，也不能让普通短时故障立即中断。可从 timestamp 24 小时、
snapshot 7 天、targets 30 天、root 365 天开始，并在到期前充分告警；到期后客户端会正确
fail closed。

## 签名索引合同

认证 target 使用严格 JSON schema v1：

```json
{
  "schemaVersion": 1,
  "registry": "example-production",
  "updated": "2026-07-16T00:00:00Z",
  "plugins": [{
    "name": "example-plugin",
    "description": "经过运营者审核的单行描述",
    "version": "1.2.3",
    "author": "Example",
    "category": "development",
    "source": "https://github.com/example/example-plugin",
    "subpath": "plugin",
    "revision": "0123456789abcdef0123456789abcdef01234567",
    "digest": "sha256-git-tree-v1:<64位小写十六进制>",
    "permissions": ["skills.load"],
    "provenance": {
      "source": "https://github.com/example/example-plugin",
      "subpath": "plugin",
      "revision": "0123456789abcdef0123456789abcdef01234567",
      "digest": "sha256-git-tree-v1:<同一个64位小写十六进制>",
      "builderId": "https://registry.example/builders/release-v1",
      "attestationTarget": "attestations/example-plugin.dsse.json"
    }
  }]
}
```

首版 source 仅接受规范的 `https://github.com/<owner>/<repository>` 与完整 40 字符 Git
commit，且必须可匿名、无交互拉取：registry 物化忽略 system/global Git 配置、credential
helper、filter、replace refs 与 LFS smudge；私有仓库凭据不属于当前 registry 合同。索引权限集合必须与 manifest 完全一致；provenance 必须绑定完全相同的 source、
subpath、revision 和 `sha256-git-tree-v1` digest。展示与路径字段有长度限制，并拒绝终端控制、双向
格式化和行分隔字符。

`attestationTarget` 可选。客户端验证其字节符合 TUF target metadata，但不会解析或独立验证
DSSE envelope、builder 身份、SLSA predicate、透明日志或 SLSA 等级。在增加独立 policy
verifier 前，应称其为“TUF 认证的 attestation target”，而不是“SLSA 已验证”。相关标准见
[DSSE](https://github.com/secure-systems-lab/dsse) 与
[SLSA 1.2 provenance](https://slsa.dev/spec/v1.2/provenance)。

## Release 与发布顺序

每次插件 release 必须：

1. 审核 canonical repository 和完整 commit；在干净环境 checkout 后执行
   `reames-agent plugin registry digest <checkout> [subpath]`，记录完整 revision 和跨平台
   `sha256-git-tree-v1` 摘要。另执行
   `reames-agent plugin install <checkout或subpath> --dry-run` 检查 manifest 版本与规范化权限，
   不执行本地安装；该预检中的 `sha256-tree-v1` 是本机安装树摘要，不能写入 registry 条目。
2. 审核 manifest 名称、语义版本、权限、skills、hooks、MCP servers 和环境变量合同；
   attestation 单独生成、单独审核。
3. 更新 `plugins.json`，确认 provenance 与条目精确一致。
4. 先写入不可变的 hash-prefixed target 文件。
5. 依次签名/发布 targets metadata、snapshot metadata，最后发布 `timestamp.json`，避免
   客户端看到已经广告的新 snapshot，却取不到被引用的字节。
6. 用只含已批准 bootstrap root 的干净、持久 client home 执行 registry refresh/search/show，
   再预检并安装 release，核对落盘 revision、canonical source 与安装树摘要、root 版本、bootstrap root 摘要、
   provenance 状态和 attestation 摘要。
7. 保留上一份完整仓库 generation 用于诊断，但不得重新发布更低 metadata 版本。错误
   release 必须用递增的新 metadata 和重新审核的插件 version/revision 修正。

对象发布必须原子或启用不可变版本。CDN 不能把旧 timestamp 和新 snapshot 混合缓存。
持续监控 metadata 新鲜度、HTTP 失败、target 可用性、意外 root 版本和客户端 refresh 失败。

## 轮换

root metadata 版本必须连续。普通 root 轮换产生 `N+1`，同时满足可信 `N` root 的阈值和
新 `N+1` root 的阈值。所有中间 `<N>.root.json` 都必须发布；客户端不会跳过缺失版本，
单次 refresh 最多接受 32 次轮换。

轮换 targets、snapshot 或 timestamp key 时，先发布正确签名的新 root，由它委派新 key 并
移除被撤销/退役 key；再按正常顺序发布新的 targets/snapshot/timestamp metadata。上线前
必须用隔离且保留 cache 的客户端演练。

替换用户 bootstrap root 是带外信任重置，不是普通轮换。它会选择新的 cache namespace，
丢弃旧 root 下学到的连续性。只有在独立认证新 root、并记录普通 root 轮换为何不可用后
才能执行。

## 泄露与恢复

- timestamp/snapshot 泄露：通过新 root 撤销、递增 metadata 版本并重新发布；客户端
  fail closed 期间可用性可能中断。
- targets 泄露：攻击者可授权恶意插件条目。通过 root 撤销、发布更高修正版、列出受影响
  revision/digest，并通知用户禁用/卸载。sandbox 和显式权限只能降低影响，不能让已签名
  恶意插件变安全。
- 若剩余未泄露的当前 root keys 仍足以满足当前阈值，则用该 quorum 轮换；否则立即停止 registry，恢复必须依赖
  另行认证的 bootstrap root 和用户显式操作，并把已经安装攻击者授权代码的客户端视为
  可能失陷。
- 不要把删除 `registry-cache` 当作普通修复。删除会清除本地学到的 rollback 状态，并从
  bootstrap root 重新开始；除非审核后的恢复程序明确要求重置，否则应保留用于调查。

本仓库当前不提供公开 endpoint、生产私钥仪式、HSM 策略、透明监控或 compromise drill。
这些属于外部运营证据，不能由客户端实现替代完成声明。
