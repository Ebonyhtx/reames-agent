# 工具合约

<a href="./TOOL_CONTRACT.md">English</a>

本文记录 Reames Agent 编译期内置工具的 provider-visible 合约。运行时 registry 使用同一条 canonical schema 路径；测试会校验这里列出的工具名、read-only 标记和 schema 快照不会漂移。

| 工具 | Read-only | 说明 |
| --- | --- | --- |
| `bash` | false | 执行 shell 命令并返回 stdout/stderr。构建、测试、git、包管理器等使用它；读写查找文件优先使用专用工具。 |
| `bash_output` | true | 读取后台 `bash` 或 `task` job 自上次读取后的新增输出和状态；`interrupted` 表示进程重启后未自动重放的可恢复任务。 |
| `code_index` | true | 轻量内置代码符号索引；优先使用 `lsp_*` 或代码图 MCP，缺失时用它兜底。 |
| `complete_step` | true | 用证据记录已批准计划中一个步骤的完成情况。 |
| `delete_range` | false | 用精确 start/end 文本锚点删除文件中的连续范围。 |
| `delete_symbol` | false | 用 Go AST 删除 Go 源文件中的命名符号。 |
| `edit_file` | false | 将文件中的唯一精确字符串替换为另一个字符串。 |
| `glob` | true | 查找匹配 glob pattern 的文件。 |
| `grep` | true | 在文件或目录下按正则搜索文本。 |
| `kill_shell` | false | 终止后台 `bash` 或 `task` job。 |
| `ls` | true | 列出目录条目，可递归。 |
| `move_file` | false | 移动或重命名文件。 |
| `multi_edit` | false | 对单个文件原子应用多个编辑。 |
| `notebook_edit` | false | 编辑 Jupyter notebook 的单个 cell。 |
| `read_file` | true | 按可分页的行号格式读取文本文件。 |
| `todo_write` | true | 记录并替换当前工作的结构化任务列表。 |
| `wait` | true | 等待后台 job 完成并返回最终输出。 |
| `web_fetch` | true | 通过 HTTP/HTTPS 获取 URL 文本内容。 |
| `web_search` | true | 搜索网络并返回标题、URL 和摘要。最多 10 条结果。 |
| `apply_patch` | false | 将 unified diff 补丁应用到文件。支持 dry-run 预览。 |
| `cronjob` | false | 创建、列出或删除持久化定时任务。支持间隔（every 30m）和一次性（30m）调度。 |
| `list_jobs` | true | 列出所有运行中的后台任务（bash 和 task），不阻塞。 |
| `write_file` | false | 写入文件内容，必要时创建父目录。 |

## 文件变更边界

所有可预览的内置 writer 都先把目标绑定为打开的 `os.Root` 与 root-relative
path；read/stat、临时写入、fsync/chmod、rename/remove 均通过该 handle
resolve-beneath 执行，验证后的路径组件被替换成 symlink/reparse point 时也不能把
操作重定向到授权 root 外。`move_file` 同时预览 source delete 与 destination
create；`apply_patch` 在写入前验证完整 multi-file diff，逐文件原子替换，并在后续
文件失败时回滚先前文件。Agent 会在执行前 checkpoint 全部 preview change。

该边界不为 `bash`、MCP 或外部 API 提供 exactly-once，也不保留 ACL、xattr 或
硬链接身份。

## Schema 快照

完整 canonical schema 不在文档中手写，避免文档和代码手工漂移。运行：

```bash
go test ./internal/tool -run TestBuiltinToolContractDocumentation
```

该测试会用 `tool.BuiltinContractEntries` 校验每个内置工具都有文档行、read-only 标记、非空 description 和 canonical JSON schema。

## 默认 Full Boot Surface

默认 full-token boot 会发送上面的内置工具，并额外发送 session、memory、skill、subagent、LSP、install 和 slash-command 工具：

`ask`, `explore`, `forget`, `history`, `install_skill`, `install_source`,
`list_sessions`, `lsp_definition`, `lsp_diagnostics`, `lsp_hover`,
`lsp_references`, `memory`, `parallel_tasks`, `read_only_skill`,
`read_only_task`, `read_session`, `read_skill`, `remember`, `research`,
`review`, `run_skill`, `security_review`, `slash_command`, `task`.

`internal/boot.TestBootToolContractMatchesProviderVisibleSurface` 会校验真实 boot registry 合约和 provider request 一致，包括 read-only 标记和 canonical schema。

## Token Economy Boot Surface

token economy 模式启动时保留核心编码、session、memory 工具，以及按需启用可选来源的 connector：

`ask`, `connect_tool_source`, `forget`, `history`, `list_sessions`, `memory`,
`read_session`, `remember`, `slash_command`.

`bash`、`read_file`、`grep`、文件写工具、后台 job 工具和 `todo_write` 等核心内置工具在 economy 模式下仍可用，见上方内置工具表。
