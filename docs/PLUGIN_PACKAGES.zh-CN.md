# Reames Agent 插件包

Reames Agent 插件包把 skills、hooks 和 MCP servers 组织成一个可安装单元。

## CLI 模式

在终端里使用 `reames-agent plugin` 安装和管理插件包。插件包当前按全局范围安装，
写入 Reames Agent home 目录。

### 通过 CLI 安装

`install` 接收一个来源：

- GitHub 仓库，例如 `git:github.com/obra/superpowers` 或
  `https://github.com/obra/superpowers`。
- GitHub 分支或子目录 URL，例如
  `https://github.com/owner/repo/tree/main/path/to/plugin`。
- 本地目录，目录内需要包含 `reames-agent-plugin.json`、
  `.codex-plugin/plugin.json` 或 `.claude-plugin/plugin.json`。
- 显式配置的签名 registry 中的 release，写作 `registry:<插件名>`。registry 安装只接受
  TUF 索引认证过的完整 Git commit、源码树摘要、manifest 版本和权限。

项目不内置 registry，也不使用 TOFU（首次使用即信任）。请只在用户级配置中设置 registry，
并通过独立可信渠道取得 bootstrap `root.json`：

```toml
[plugin_registry]
metadata_url = "https://registry.example/metadata"
targets_url = "https://registry.example/targets"
trusted_root = "registry/root.json"
index_target = "plugins.json"
```

相对 `trusted_root` 路径解析到 `REAMES_AGENT_HOME` 下。endpoint 与 bootstrap root 字段中的
变量只从进程环境展开，`index_target` 按字面值使用；项目 `.env` 不能替换 endpoint 或
bootstrap root。以下命令只返回通过认证的条目：

```bash
reames-agent plugin registry refresh
reames-agent plugin registry search [查询词]
reames-agent plugin registry show <插件名>
reames-agent plugin registry digest <checkout> [subpath]
```

插件名使用跨平台 ASCII 命名空间。仅 ASCII 大小写不同的名称视为同一别名，不能共存；
尾点以及 `CON`、`AUX`、`COM1` 等 Windows 设备名（包括带扩展名的形式）会在物化任何
包内容前被拒绝。状态文件和签名 registry 索引使用同一规则，确保所有支持的文件系统上
`plugins/<name>` 只有一个物理所有者。

签名条目使用跨平台 `sha256-git-tree-v1`，覆盖 canonical Git blob、路径和可执行意图；
安装 generation 另用 `sha256-tree-v1` 覆盖本机实际树，用于生命周期防篡改。registry
GitHub source 必须可匿名拉取，不依赖交互凭据或 ambient Git 配置。

安装仍使用同一套两阶段合同：

```bash
reames-agent plugin install registry:<插件名> --dry-run
reames-agent plugin install registry:<插件名> --yes --plan-id sha256:<预检返回的ID>
```

客户端使用官方 `go-tuf/v2` 工作流和持久 metadata，验证 root 轮换、过期、回滚、冻结和
mix-and-match 防护；apply 会重新解析条目并重算源码树摘要。持久证据会区分 TUF 签名的
registry assertion 与可选的 TUF 认证 attestation target。后者目前只证明 target 完整性：
Reames Agent 尚不验证 DSSE 签名者身份、SLSA predicate policy 或 SLSA 等级。registry
运营和密钥仪式见 [插件 Registry 运维](PLUGIN_REGISTRY_OPERATIONS.zh-CN.md)。

只预览安装计划，不写文件：

```bash
reames-agent plugin install git:github.com/obra/superpowers --dry-run
```

确认计划后安装：

```bash
reames-agent plugin install git:github.com/obra/superpowers --yes --plan-id sha256:<预检返回的ID>
```

指定安装名称，或覆盖已安装的同名插件：

```bash
reames-agent plugin install git:github.com/obra/superpowers --name superpowers --replace --dry-run
reames-agent plugin install git:github.com/obra/superpowers --name superpowers --replace --yes --plan-id sha256:<预检返回的ID>
```

以开发模式使用本地目录：

```bash
reames-agent plugin install /path/to/plugin --link --replace --dry-run
reames-agent plugin install /path/to/plugin --link --replace --yes --plan-id sha256:<预检返回的ID>
```

CLI 安装参数：

- `--dry-run` 只规划和校验安装，不写文件。
- `--yes` 用于确认执行会写文件的安装。
- `--plan-id <id>` 将执行绑定到预检返回的摘要、来源 revision、权限和动作；
  插件执行会拒绝缺失或过期的 ID。
- `--replace` 允许当前来源替换已安装的同名插件。
- `--name <name>` 或 `--name=<name>` 覆盖插件 manifest 里的名称，
  作为本次安装名称。
- `--link` 链接本地插件目录，而不是复制到 Reames Agent 的插件存储目录。
  移动或删除该目录会导致这个链接插件失效。

如果运行 `reames-agent plugin install <source>` 时既没有 `--dry-run`，
也没有 `--yes`，CLI 会拒绝写文件，并提示使用其中一个参数重新运行。
安装和移除命令会输出结构化 JSON，来源于桌面端同一套 install-source 后端。

插件状态和内容写入：

```text
~/.reames-agent/plugin-packages.json
~/.reames-agent/plugins/<name>/versions/<sha256-tree-v1-id>/
```

复制安装会发布不可变、内容寻址的 generation。状态文件原子选择 active generation，
并保留一个已验证的前代用于回滚。新安装默认禁用，只有精确摘要和请求权限得到授权后
才会加载。GitHub 来源会记录 commit revision，但当前信任状态仍是
`github-https-unsigned`；HTTPS 传输不等于 Reames 签名。

### 通过 CLI 管理

列出已安装插件：

```bash
reames-agent plugin list
```

查看某个插件的元数据、根目录、来源以及导出的能力数量：

```bash
reames-agent plugin show superpowers
```

如果能读取到能力明细，`show` 也会输出具体清单：

- **skills** 会展示建议的 `/<skill>` 调用方式和描述。
- **hooks** 会展示生命周期事件、matcher、命令或上下文文件。
- **mcpServers** 会展示服务器名称、传输方式和启动目标。

验证受管根目录、manifest、内容摘要、权限合同和 skill roots：

```bash
reames-agent plugin doctor superpowers
```

在不卸载的情况下启用或禁用插件：

```bash
reames-agent plugin disable superpowers
reames-agent plugin enable superpowers
reames-agent plugin enable superpowers --yes
```

第一次 enable 只展示信任状态、摘要和精确权限，不会启用；确认后再带 `--yes` 执行。
链接插件的字节或权限变化后必须重新预检。

预检并执行更新：

```bash
reames-agent plugin update superpowers --dry-run
reames-agent plugin update superpowers --yes --plan-id sha256:<预检返回的ID>
```

权限扩张会让新 generation 保持禁用，直到重新授权；若旧授权已完整覆盖新权限，
更新后可以继续保持启用。

预检并回滚到前一个已验证 generation：

```bash
reames-agent plugin rollback superpowers --dry-run
reames-agent plugin rollback superpowers --yes --plan-id sha256:<预检返回的ID>
```

移除插件：

```bash
reames-agent plugin remove superpowers --dry-run
reames-agent plugin remove superpowers --yes --plan-id sha256:<预检返回的ID>
```

`remove` 也可以写成 `uninstall`。更新、回滚和移除共用 preview/planId/apply
合同。如果是链接模式安装的本地插件，外部源目录会保留。

### 在 CLI 中使用已安装插件

已安装插件不会打开一个独立聊天界面。插件启用后，Reames Agent 会把它的能力加载到普通交互会话里：

- 在交互会话里运行 `/plugins` 可以列出已安装插件包。
  运行 `/plugins show <name>` 可以在不离开聊天的情况下查看该插件导出的
  skills、hooks、MCP servers 和使用提示。
- **Skills** 会出现在 `/skills` 中。可以用 `/<skill> [args]` 直接调用，
  也可以自然描述任务，让 agent 按 description 选择匹配的 skill。
- **Hooks** 会在配置的生命周期事件里自动运行，例如 `SessionStart`、
  `UserPromptSubmit`、`PreToolUse` 或 `PostToolUse`。
- **MCP servers** 会进入正常 MCP/工具流程。用户只需要描述任务，
  Reames Agent 会在相关时调用插件提供的工具。

如果是在另一个终端里安装、启用、禁用或更新插件，而当前已有 `reames-agent` 会话正在运行，
建议开启新会话，或重新打开 `/skills` 确认当前会话能看到预期技能。

## 桌面端设置

打开 **设置 -> 插件**，可以不用 CLI 直接安装和管理插件包。

### 安装插件

安装区有三种模式：

- **本地目录**：可在路径输入框直接填写或粘贴插件目录，也可以点击
  **选择插件目录** 从磁盘选取。
- **Git 仓库**：填写 Git 来源，例如 `git:github.com/obra/superpowers`。
  **安装名称（可选）** 可覆盖插件 manifest 声明的名称，用于本次安装或覆盖。
- **签名 Registry**：搜索已配置且通过认证的索引，选择 release，并在预检
  `registry:<插件名>` 前核对 registry 名和可信 root 版本。缺少信任配置时 fail closed。

选择来源和选项后，再使用操作按钮：

- **预检** 校验来源并展示计划安装动作，不写入文件。
- **安装插件** 按当前来源和选项执行安装。
- **刷新插件** 从磁盘和配置重新读取已安装插件列表。

安装选项：

- **覆盖同名插件** 允许当前来源替换已安装的同名插件。关闭时，同名安装会失败，
  而不是覆盖已有内容。
- **开发模式：链接源目录** 只在 **本地目录** 模式出现。它不会复制插件，
  而是直接链接所选目录；适合开发或调试插件。移动或删除该目录会导致这个链接插件失效。

桌面端必须先完成 **预检** 才能执行安装。执行会绑定到已展示的 `planId`；来源、
名称、链接模式或覆盖选项变化都会使旧预检失效，必须重新预检。

### 管理已安装插件

已安装插件列表会展示每个插件包以及它导出的 skills、hooks 和 MCP servers。
通过应用外编辑插件文件或配置后，可点 **刷新插件** 重新读取。

展开插件行后可以：

- 启用或禁用插件。
- 在启用或切换 generation 前查看来源信任、摘要、请求/已授权权限和回滚状态。
- 查看 **使用方法**，了解该插件导出的 skills、hooks 和 MCP servers。
- 使用 **更新** 拉取或刷新具备更新来源的插件。更新、回滚和移除都会先展示版本、
  权限、信任、摘要和风险差异，确认动作只执行对应的 `planId`。
- 在存在前一个已验证 generation 时使用 **回滚**。
- 使用 **诊断** 检查插件 manifest，并查看警告或诊断信息。
- 使用 **移除插件**，确认后卸载该插件包。

更新、回滚、移除或禁用后，Desktop 会在所有 live/detached controller 中断开
controller 加载时绑定且精确匹配该插件的 MCP server，并移除该插件 Hook。同步 rebuild
与 mutation 串行；controller runtime reservation 和 Desktop work-start gate 会阻止空闲
检查后新 turn、Shell 或会话旋转起跑，按旧状态启动但尚未发布的 startup build 会先被
取消。旧 controller 的 Skill 入口会 fail closed，直到 controller 重建或开启新会话，
因为共享 Skill store 不能在原地安全切换 generation。MCP connection 与 owner 更新保持
串行；断开插件 MCP 会清除其 owner，因此之后接管同名的用户 MCP 不会被插件生命周期
操作断开。

### 在桌面端使用已安装插件

桌面端设置页和 CLI 使用同一套运行模型：

- 展开已安装插件，可以看到 **使用方法** 区域。
- 在任意桌面会话里输入 `/plugins` 可以列出已安装插件；
  输入 `/plugins show <name>` 可以直接从聊天界面查看同一套使用详情。
- Skills 会展示建议的直接命令，例如 `/plan`；在会话中也可以通过 `/skills` 浏览。
- Hooks 和 MCP servers 作为透明能力清单展示。它们不需要单独的“运行”按钮：
  启用的 hooks 会自动触发，MCP 工具会通过普通工具调用流程可用。
- 如果当前打开的会话没有反映插件变更，刷新插件列表并开启新会话。

## 原生 Manifest

Reames Agent 原生插件在根目录声明 `reames-agent-plugin.json`：

```json
{
  "schemaVersion": 1,
  "name": "example",
  "version": "1.0.0",
  "description": "Example plugin",
  "skills": ["skills"],
  "hooks": {
    "SessionStart": [
      {
        "command": "hooks/session-start",
        "description": "Load startup context"
      }
    ]
  },
  "mcpServers": {
    "helper": {
      "command": "bin/helper"
    }
  },
  "permissions": ["hooks.execute", "mcp.stdio", "skills.load"]
}
```

相对路径都按插件根目录解析。Reames Agent 安装插件时不会执行第三方安装脚本。
原生 schema v1 要求合法语义版本，且声明权限必须与实际能力推导出的集合完全一致。
支持 `skills.load`、`hooks.context`、`hooks.execute`、`mcp.stdio` 和
`mcp.remote`。没有 `schemaVersion` 的原生 manifest 仍可按 legacy 兼容读取，
但会警告，metadata-only 旧状态不会被静默提升为安全安装。
历史文件名 `reamesAgent-plugin.json` 仍可兼容读取，但会产生弃用警告；新插件必须使用
`reames-agent-plugin.json`。

## Codex 与 Claude 兼容

Reames Agent 也会读取 `.codex-plugin/plugin.json` 和 `.claude-plugin/plugin.json`。
Reames Agent 尚未映射的 Claude 插件能力（`commands/`、`agents/`、`hooks/hooks.json`、
`.mcp.json`）会以安装警告的形式提示，而不是被静默丢弃；多插件的
`marketplace.json` 索引暂不支持——请逐个安装插件目录。
对于 Superpowers 和 Claude 风格 skill 包，Reames Agent 会映射：

- `skills` 到 Reames Agent skill root。Claude 清单若未声明 `skills` 字段，会回退到
  约定目录 `skills/`（或 `.claude/skills/`），与 Claude 自身的自动发现一致。
- 如果存在 `hooks/session-start-codex`，映射为 Reames Agent `SessionStart` hook。
- 插件根目录的 `CLAUDE.md` 会映射为内置的 `SessionStart` 上下文 hook。
  Reames Agent 会直接读取该文件，不通过 shell 命令。
- `.claude/settings.json` 里的 command hooks 会按同名事件映射到 Reames Agent hooks。
  Claude 的 `matcher` 字段会映射到 Reames Agent `match`；hook 命令以插件根目录作为
  `cwd` 执行；Claude `timeout` 按秒解析。

不支持的 Claude hook item type 会跳过并产生 warning。Reames Agent 不会执行第三方安装脚本，
也不会实现 marketplace 专用安装协议。

Reames Agent 当前没有已运营的默认插件 registry。Registry URL 必须显式配置；
任意 registry index 或 unsigned GitHub 仓库都不能被当作可信发布者。

插件 hook 会收到这些环境变量：

- `REAMES_AGENT_PLUGIN_ROOT`
- `REAMES_AGENT_PLUGIN_NAME`
- `REAMES_AGENT_PLUGIN_VERSION`
- `REAMES_AGENT_PLUGIN_STATE`
- `REAMES_AGENT_HOME`
- `REAMES_AGENT_WORKSPACE_ROOT`
- `CLAUDE_PROJECT_DIR`

`REAMES_AGENT_PLUGIN_STATE` 是 `plugins/<name>/state` 下与 generation 无关的可写目录。
通用 child 环境把临时文件指向其 `tmp/` 子目录；Windows 原生 backend 会进一步把
TEMP/TMP 替换为每次命令独立的 sandbox temp。已安装包贡献的 Hook 和 stdio MCP 不继承 Reames Agent
完整环境，只接收 OS 启动核心变量、manifest 显式变量和上面的可信字段；它们在 fail-closed
OS sandbox 中运行，写入限制在 workspace/state/temp，已知凭据路径会被阻止读取。Linux 的私有
`/tmp` overlay 会把不可变 generation root 与精确 helper 文件只读重挂，兼容位于临时隔离
home 的安装，同时不扩大写权限。包装器 argv 不携带 manifest 环境变量名或值；隐藏 helper
只在进入隔离边界后恢复编码的 child 环境，并在 `exec` 前清除传递变量。payload 缺失、损坏或
宿主未注册 helper 时 fail closed。
该编码不是加密，也不承诺防护同机高权限调试器或进程转储；插件 child 本来就被显式授予这些值。
Windows 对已验证 generation 内的绝对 shebang Hook 可使用 Git Bash，使无扩展名 Codex Hook 保持可用。

该策略只适用于已安装插件包，不自动改变用户手写的全局/项目 Hook、用户 MCP 或 LSP。
由于 v1 权限还没有独立 network capability，package process 当前允许网络；项目也尚未承诺
跨平台统一的硬 CPU/RSS 配额。

## 桌面端后端方法

Desktop 通过 Wails 方法暴露插件包操作：

- `Plugins`
- `PlanPluginInstall`
- `InstallPlugin`
- `PlanPluginUpdate`
- `UpdatePlugin`
- `PlanPluginRollback`
- `RollbackPlugin`
- `PlanPluginRemove`
- `RemovePlugin`
- `SetPluginEnabled`
- `PluginDoctor`
