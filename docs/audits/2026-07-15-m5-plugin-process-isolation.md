# M5 插件进程隔离与真实第三方 E2E 审计

> 日期：2026-07-15
> 更新：2026-07-16
>
> 范围：安装包拥有的 Hook/MCP 子进程、运行时撤销、敏感环境和真实第三方插件
>
> 结论：package-owned 进程隔离与一条真实第三方 Hook E2E 已形成 Windows 原生证据；
> M5 仍等待干净 clone、最新远端 CI/CodeQL 和运营 registry/签名信任链

## 采用边界

本批只收紧由已安装插件包贡献、且已通过 digest 与精确权限授权的 Hook/MCP。用户手工
配置的全局/项目 Hook、用户 MCP 和 LSP 保持现有兼容行为，避免把一次生态安全收口扩大为
不兼容迁移。package owner、不可变 generation root、托管 state root、workspace 和 Reames
Agent home 从 `config -> boot/hook -> processpolicy -> spawn` 显式传递，不从插件可伪造的环境
变量反推。

参考机制：

- Codex `rmcp-client`：本地 stdio MCP 使用 `env_clear`，只继承 core environment，再叠加
  配置明确声明的变量；
- AgentArk：MCP/extension runtime 使用清空环境、显式 secret/env 注入和独立 runtime dir；
- Claude Code security guidance：禁止把不可信 env 合并到 sandbox wrapper，避免
  `LD_PRELOAD`、`DYLD_*`、`NODE_OPTIONS`、`PYTHONPATH` 等在隔离前执行；
- DeepSeek Reasonix/Reames Agent 既有 sandbox：复用 Linux bubblewrap、macOS Seatbelt、
  Windows AppContainer/低完整性 token + Job Object，不引入第二套执行后端；
- Reames Lite 的 best-effort shell/worktree 隔离不足以证明 OS 边界，因此未复用。

## 已实现合同

1. `internal/processpolicy` 为 package-owned 进程建立强制策略：宿主 wrapper 只收到 OS
   启动核心环境；插件 child 只收到核心环境、manifest 明确 env 和不可伪造的
   `REAMES_AGENT_PLUGIN_*`/workspace/state 字段。宿主 Provider/API/token 环境不会自动继承。
2. 包装器环境和 child 环境分离。显式 child 环境先编码到单个保留环境变量，原始变量名和值
   不进入 wrapper argv，也不能作为 `LD_PRELOAD`、`DYLD_*`、`NODE_OPTIONS` 等影响可信
   wrapper。Linux/macOS 在 bubblewrap/Seatbelt 边界内启动同一可信二进制的隐藏 child-exec
   helper；helper 读取并立即清除保留变量，再以清空后重建的环境 `exec` 真正插件命令。
   Windows helper argv 只保留非敏感 sandbox/cwd 策略，child 环境从同一保留变量读取、清除后
   交给低完整性/AppContainer child。任何需要 child 环境的 helper 若缺少或无法解码该变量均
   fail closed；CLI 与 Desktop 在正常启动、配置和单实例逻辑前注册并分派隐藏入口。
3. sandbox backend 不可用时 package-owned Hook/MCP 无 escape approval，直接 fail closed；
   用户手工 MCP/Hook 不受此策略隐式改写。
4. `plugins/<name>/state/` 是 generation-independent 可写状态根，通用 child env 使用
   `state/tmp/`；Windows backend 会进一步覆盖为每次命令独立的 sandbox temp。创建使用
   `os.Root` resolve-beneath，拒绝 symlink escape。update/rollback 保留 state，uninstall
   删除整个安装根；不可变 generation 本身不作为写根。
5. 严格 sandbox 只允许 workspace、state 和隔离 temp 写入。Linux 额外启用 PID/IPC/UTS
   namespace 与 `--die-with-parent`，并用私有 `/tmp`；私有 overlay 之后只读重挂不可变
   generation root 和精确 helper 文件，因此位于临时目录的隔离 home、测试或安装不会失去
   可执行文件，也不会把 generation 变成可写。macOS strict 模式不再开放用户
   toolchain cache 或全局 `/tmp`；Windows writable child 使用低完整性 token、临时 ACL 和
   kill-on-close Job Object。
6. `.ssh`、`.gnupg`、`.aws`、`.azure`、`.kube`、`.docker`、gcloud/gh 配置，以及
   Reames Agent `.env`、legacy credentials、`credentials.enc`、config、bot pairing 等已存在
   敏感路径进入 read barrier。Linux 对单文件 bind `/dev/null`，Seatbelt 使用 literal deny，
   Windows 合并到 deny-ACE 集合。
7. 当前权限词汇没有独立 network capability，且 Windows writable low-integrity 模式无法
   可靠执行 `network=false`，因此已授权的 `hooks.execute`/`mcp.stdio` package 进程目前允许
   网络；这是一条明确边界，不冒充“默认无网”。
8. Hook timeout/cancel 改用 tracked process tree；正常结束也回收后台后代。Runner 记录每个
   owner 的 active cancel，disable/update/rollback/uninstall 撤销会取消已运行 Hook，并用
   disabled generation 标记封住“已取快照、尚未 spawn”的竞态。MCP `Host.Remove` 继续关闭
   transport 并回收进程树。
9. package Hook/MCP 的 bounded stderr/stdout 会按 manifest 显式敏感 env 值精确脱敏；MCP
   stderr 保持 16 KiB tail，Hook 双流各 256 KiB，启动/调用/Hook 超时与 MCP 启动并发上限
   继续生效。
10. Windows package Hook 若是已验证 generation 内的绝对路径 shebang 脚本，会使用经过
    WSL 排除与可用性探测的 Git Bash 直接执行；用户/global Hook 不获得该 package-only 路径。
11. MCP schema cache 改用项目统一 `AtomicWriteFile`，修复 Windows 覆盖已有 cache 时偶发
    保留旧快照的真实不稳定；分歧握手回归连续 10 次通过。

## 自动验证

定向包全测：

```powershell
go test ./internal/processpolicy ./internal/sandbox ./internal/hook `
  ./internal/config ./internal/boot ./internal/plugin -count=1
go test ./internal/plugin -run '^TestLazyCacheHitPinsToolBytesAcrossDivergentHandshake$' -count=10
go test ./internal/pluginpkg -run '^TestRuntimeStateSurvivesGenerationChangesAndUninstallRemovesIt$' -count=1
```

覆盖证据包括：

- ambient API key/token/runtime-injection 环境被过滤，显式 package secret 保留，可信 root/name
  不能被 manifest env 覆盖；
- wrapper argv 与原始 host 环境键值不包含 manifest 显式 secret；编码环境可在隐藏 helper 中
  往返恢复，保留变量在 child 前清除，Windows 缺失 payload 时在启动 child 前 fail closed；
- state resolve-beneath 创建、state symlink escape、sandbox unavailable、strict write roots、
  Linux private `/tmp` 后的 generation/helper 精确只读重挂、敏感目录/文件 barrier 和
  child-only env/cwd 传递；
- Windows helper 实际完成 workspace 内写、workspace 外写拒绝、child env 与 cwd 应用；
- Hook timeout 后代回收、运行中 disable cancel、快照后 late-start 拒绝、输出上限和敏感诊断
  脱敏；
- config/load/boot 将真实 package owner/root/state/home/workspace 传入 Hook/MCP；
- runtime state 在 generation update/rollback 后保留，uninstall 后删除。

## 真实第三方插件 E2E

使用公开 MIT 仓库 [obra/superpowers](https://github.com/obra/superpowers) 的固定 revision：

```text
revision: d72560e462a74e10d161b7f993d5fc3282bfa1e2
version: 5.1.0
manifest: .codex-plugin/plugin.json
trust: github-https-unsigned
digest: sha256-tree-v1:26bcf3b5d0eafe546bdd843185960c65aef576ebc7a1ee8d530c2f8487de7e79
permissions/grants: hooks.context, hooks.execute, skills.load
```

在 Windows 隔离临时 home/workspace 中，实际二进制 harness 注册同一 sandbox helper dispatch，
完成 copy install -> disabled -> exact grant enable -> VerifyInstalled -> Hook load -> 原生
SessionStart 执行。结果：`sandboxAvailable=true`、`hookCount=2`、`contextCount=2`、
`contextVerified=true`、`notices=[]`，输出包含真实 `You have superpowers` 与
`using-superpowers` 上下文，托管 `state/tmp` 存在。该链没有 Provider/API key，不把固定
revision 的 unsigned GitHub 来源描述为 Reames 签名或 provenance；临时 harness、clone、
home 和 workspace 已在成功后删除。

## 尚未关闭

- 本机原生执行证据是 Windows；Linux/macOS 本批代码需由交叉编译、平台测试和集中 push 后
  的 CI/CodeQL 继续验证，不能由 Windows 结果代替。
- 当前有启动并发、超时、输出上限、严格写/读边界和进程树回收，但没有跨三平台统一的硬
  CPU/RSS 配额；不能声明对所有资源耗尽攻击完全免疫。
- 用户手工 Hook/MCP 与 LSP 未自动迁入 package policy；后续若收紧必须提供显式配置与迁移。
- package process 当前允许网络；独立 `network`/`filesystem` capability 仍是后续权限 schema
  设计事项。
- 保留环境变量使用编码而非加密；它关闭的是命令行/动态加载器边界，不是对同机高权限进程、
  调试器或进程转储的独立密钥保险箱。child 本来就被显式授予这些变量。
- registry 仍非运营默认服务；GitHub 仍是 unsigned HTTPS + revision。签名、provenance、
  密钥轮换、干净 clone 和最新提交远端全绿仍是 M5 关闭条件。
