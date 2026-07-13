# M6 Gateway setup 配置闭环审计

日期：2026-07-13

状态：本地实现与确定性纵向验证通过；干净云节点和真实 IM 回环仍为外部证据

## 缺口与参考边界

M6 已有 `gateway run`、`doctor`、Linux systemd、macOS launchd、Windows
Scheduled Task 生命周期和隔离 HOME 的 headless smoke，但服务器用户仍只能先
打开 Desktop 或手写完整 TOML。Hermes 的可取机制是独立 gateway daemon、setup
入口和本机审批配对；Reames 保留自己的 Go 配置、`control.Controller`、权限和
会话模型，没有复制 Hermes 的 Python runtime。DeepSeek Reasonix 继续提供当前
Go/Wails 基座，Reames Lite 只提供凭据不入命令行、metadata 不污染 prompt 的
历史契约。

## 实现合同

新增 `reames-agent gateway setup` 与 `internal/gatewaysetup`：

- 支持 Feishu、Lark、QQ、Weixin，以及连接 ID、workspace、model 和工具审批模式。
- Feishu/Lark/QQ 只接收 app ID 与 app-secret 环境变量名；Weixin 只接收 account
  ID 与 token 环境变量名。环境变量名使用常规大写格式，CLI 不提供 secret-value
  参数，脱敏计划只显示标识符是否已设置。
- 新连接必须显式选择 pairing、users、groups、approvers、admins 或有意
  `allow_all`；否则 fail closed。`--reset-access` 可把旧 `allow_all` 收窄到新名单。
- 配置事务持有进程内用户配置编辑锁，严格解析现有 TOML，再用配置层的 sibling
  temp + rename 原子写入。损坏配置不会回退到默认值后被覆盖。
- 更新按 connection ID 幂等合并，保留其他 provider、connection、route、
  `created_at`、session mappings 和旧 access；相同重跑不刷新 `updated_at`，也不
  重写文件。
- `--dry-run` 不创建或改写配置，输出稳定的 redacted plan，可直接进入部署审计。
- 同步 legacy bot adapter 字段，使现有 runtime、`gateway doctor`、Desktop 和
  service manager 继续消费同一份用户配置。

## 确定性证据

测试覆盖：

- 四渠道连接创建、默认环境变量引用和 legacy adapter 同步。
- 其他连接、route、session mapping、access、凭据引用和 `created_at` 保留。
- 完全相同重跑的 byte-for-byte 不改写与 `updated_at` 不漂移。
- `--reset-access` 从 `allow_all` 收窄到明确 owner。
- 缺少访问规则、误传小写 secret、缺少渠道标识符、未知渠道和损坏 TOML 的
  fail-closed 行为。
- dry-run 对不存在的 HOME 零落盘，对会触发旧配置规范化的已有文件仍保持
  byte-for-byte 不变，计划不打印 app ID/account ID。
- 无真实 secret 的 `setup -> gateway doctor -> gateway install --dry-run` 纵向链路；
  doctor 准确报告环境变量未设置，service plan 只绑定 `REAMES_AGENT_HOME`。

这些证据证明配置和运维准备链路，不证明平台鉴权、消息收发或云服务生命周期。
下一层必须在干净 Linux 节点使用真实二进制运行 setup、doctor、user systemd 和
feedback，再用真实飞书应用完成文本、审批、取消和恢复回环。真实 API key、IM
应用和云服务器证据保持 `external-blocked`，不得由 fixture 冒充。
