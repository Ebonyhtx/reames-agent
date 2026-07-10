# Windows 原生 Desktop smoke 证据

日期：2026-07-10

验证源码：本审计随附提交（基于远端基线 `8f749dc`）

## 已取得证据

1. `corepack pnpm build` 通过，随后使用固定版本 Wails CLI `v2.12.0` 从当前源码生成绑定并构建 Windows amd64 应用。
2. 构建产物为 `desktop/build/bin/reames-agent-desktop.exe`，大小 47,873,024 bytes，SHA-256 为 `E6B93A81487E745A492A94D586D8F630241343785A2B10EC58396670942EC12E`。
3. `scripts/smoke_desktop_native.py` 使用独立临时 home 启动该产物，连续观察 12 秒。目标 PID 始终存活，最多发现 1 个可见窗口，完成 24 次窗口检查和 23 次成功的 `SendMessageTimeoutW(WM_NULL)` 消息泵响应检查，最终检查仍可响应。
4. 隔离 home 内产生 `desktop-projects-legacy-recovered`、`desktop-tabs.json`、`desktop-window.json` 及会话元数据；对 `%APPDATA%/reames-agent` 和 `%LOCALAPPDATA%/reames-agent` 的前后元数据快照没有发现新增、删除或修改。
5. smoke 在隔离 home 中写入不含凭据的最小配置，将更新检查关闭并把关闭行为固定为 `quit`，避免默认 `background` 语义把 `WM_CLOSE` 变成隐藏到托盘。进程收到 `WM_CLOSE` 后在有界等待内自行退出，没有走 `terminate` / `kill` 回退；`cleanup_method` 为 `wm-close`，退出码为 2，因此这不是零退出码证据。
6. JSON 证据的 `outcome` 为 `passed`、`responding` 为 `true`、`responsive_checks` 为 23/24、`cleanup_ok` 为 `true`、`temp_cleaned` 为 `true`、`boundary_changes` 与 `errors` 均为空。

构建与验证命令：

```powershell
Push-Location desktop/frontend
corepack pnpm build
Pop-Location

Push-Location desktop
$env:GOPROXY = 'https://goproxy.cn,direct'
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 build -clean -s
Pop-Location

python scripts/smoke_desktop_native.py `
  --exe desktop/build/bin/reames-agent-desktop.exe `
  --out artifacts/desktop-native-smoke.json
```

`-s` 只跳过 Wails 对裸 `pnpm` 的再次调用；嵌入的 `frontend/dist` 来自前一步已通过的 production build。JSON 证据是本地验证产物，不纳入 Git。

## 证据边界

本次验证证明当前 Windows 原生候选能够启动、维持可响应的可见窗口、将状态限制在显式 home，并在观察后退出。它不读取用户状态内容，也不证明 Wails command bridge、真实模型调用或用户点击流程。

frameless Wails 窗口的既有捕获/输入自动化仍返回：

```text
SetIsBorderRequired failed: 不支持此接口 (0x80004002)
```

后续尝试确认 Windows UI Automation 可以读取当前 WebView 的完整可访问性树，包括新建会话、项目、输入框、发送、模式和模型控件；但截图仍因上述接口失败，索引主动作又要求有效截图缓存，UIA `set_value` 还会因缺少 CacheRequest 属性失败。因此该通道目前只提供被动文本证据，不能可靠执行点击。

本次没有执行新建会话、选择工作区、发送、停止或恢复点击流，不能将 M1 原生点击项标记为完成。后续需使用支持 frameless WebView2 的捕获/点击通道、独立 UIA InvokePattern 驱动，或记录 commit、步骤、结果和不含密钥截图的人工验证。
