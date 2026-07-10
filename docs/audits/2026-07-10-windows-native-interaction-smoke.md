# Windows 原生 Desktop 交互 smoke 审计

日期：2026-07-10

## 目标

关闭 Windows frameless Wails 窗口无法被既有截图工具可靠捕获后留下的 M1 证据缺口，并验证用户指出的首屏 API key 引导不会再次阻塞自动化或无密钥本地使用。

本审计覆盖真实 Wails/WebView2 窗口中的以下纵向路径：

```text
隔离启动
→ 无密钥首屏可用
→ 选择预置项目并新建会话
→ 输入和发送 marker
→ provider 请求与会话持久化
→ 启动并停止本地长命令
→ 关闭并重启
→ 恢复同一工作区、会话和消息
```

## 实现

### 首次启动与状态隔离

- “暂时跳过”首次 provider 配置现在写入 `[desktop].onboarding_dismissed`，不再只是 React 进程内状态；用户重启后不会反复看到 API key 引导。
- 真实已保存凭据仍可直接关闭引导，持久化跳过不伪造凭据，也不把密钥写入配置。
- 显式 `--home` 或 `REAMES_AGENT_HOME` 启动时，Windows WebView2 user data 固定在 `<home>/webview2`。普通启动继续使用 Wails 默认目录，避免迁移现有用户状态。
- smoke 同时围栏 `%APPDATA%/reames-agent`、`%LOCALAPPDATA%/reames-agent` 和 Wails 默认 `%APPDATA%/<executable>` 元数据；不读取这些默认目录的内容。

### 截图无关的 UIA 驱动

`scripts/windows_uia.py` 直接通过 Windows UI Automation COM 接口枚举 WebView 可访问性树，不依赖截图缓存。驱动优先使用 `InvokePattern` 和 `ValuePattern`；文本先由 `ValuePattern.SetValue` 写入，再向实际焦点 WebView HWND 发送最小字符消息以同步 React `onChange`，Enter 同样投递到该焦点窗口，只有失败时才回退到 `SendInput`。Composer 的输入、发送和停止控件分别暴露稳定的 `composer-input`、`composer-send`、`composer-stop` AutomationId，按钮状态通过 AutomationId、名称和 `IsEnabled` 共同形成有界等待围栏。

该路径绕开了 frameless 窗口截图仍会返回的错误：

```text
SetIsBorderRequired failed: 不支持此接口 (0x80004002)
```

错误仍是第三方截图通道的已知限制，但不再阻塞当前 Windows 原生交互验证。

### 确定性 provider

`scripts/smoke_desktop_interaction.py` 在测试进程内启动仅监听随机 `127.0.0.1` 端口的最小 OpenAI 兼容 SSE 服务。最初基线无需 key；当前 schema v2 使用只存在于隔离 Desktop 子进程中的合成 `REAMES_NATIVE_SMOKE_API_KEY`，从而同时验证服务端拒绝非空无效 key，仍不读取或保存真实 API key。

这项设计证明前端输入经过 Wails bridge、tab 绑定 controller 和 OpenAI 兼容 provider 后返回 assistant response；它不会把“远端鉴权失败产生的事件”冒充成功消息。真实公网 Provider 与使用量证据仍由 `audits/2026-07-09-real-provider.md` 单独承担。

### 当前会话格式

当前 session 的 canonical 内容是 `<session>.events.jsonl` 事件账本，主 `<session>.jsonl` 可以是空的兼容 checkpoint。smoke 会重放 `replace` / `append` 记录，并按 role + 精确 content 验证 user marker 和 assistant response；不再错误地把空兼容 checkpoint 判为持久化失败。

## 本地原生证据

验证二进制：

```text
desktop/build/bin/reames-agent-desktop.exe
size: 47,875,584 bytes
SHA-256: 27D2B7567476D9BF09B670EC0B965238D76A9BBAA5B32D1B2D85CE2B68192E16
```

执行命令：

```powershell
python scripts/smoke_desktop_interaction.py `
  --exe desktop/build/bin/reames-agent-desktop.exe `
  --artifact desktop/build/bin/reames-agent-desktop.exe `
  --out artifacts/windows-interaction-smoke.json `
  --timeout-seconds 45
```

本地 JSON 证据结果：

- `outcome = passed`；
- `onboarding_absent`、`project_visible`、`new_session_invoked`、`workspace_selected` 均为 `true`；
- loopback provider 收到 1 个请求并包含唯一 marker；
- user marker 与固定 assistant response 都进入 canonical 事件账本；
- UIA 输入跨 Git Bash/PowerShell 可用的 `!python -c "import time; time.sleep(30)"` 后，通过 `composer-stop` AutomationId 发现 Stop，调用 `InvokePattern` 并等待 Stop 消失；
- 两次 `WM_CLOSE` 均在有界等待内结束进程，没有 `terminate` / `kill` 回退；
- 重启后恢复的 session path 与首次运行完全一致，UIA 同时看见 user marker 和 assistant response；
- `boundary_changes` 与 `errors` 为空，临时夹具已删除。

## CI 与发布候选契约

- 普通 CI 的 release-contracts job 会在 Linux 上运行交互脚本的纯 Python 合同测试。
- 手动 `Desktop candidate` workflow 在 Windows 静默安装真实 NSIS 后，先运行窗口消息泵 smoke，再对安装后二进制运行完整 UIA 交互 smoke，最后静默卸载。
- `desktop-windows-interaction-smoke.json` 随候选 artifact 上传；release contract 要求 workflow 不能移除该步骤或证据路径。

WebView2 子进程在 GitHub Windows runner 上可能比 Wails 主进程更晚释放 user-data 数据库；真实安装后交互曾在功能链路全部通过后仍超过 8 秒才释放锁。夹具删除因此使用总计 20 秒的有界退避重试；若 WebView2 恰好在 `rmtree` 扫描后并发删除某个子文件，只有 fixture 根目录也确实消失才把 `FileNotFoundError` 判为成功，否则继续重试。锁释放后仍删除整个隔离 home，锁持续不释放则保持失败。该重试只处理 teardown 的短暂文件占用，不放宽 `cleanup_ok`、`temp_cleaned` 或默认状态围栏。

GitHub Windows runner 可能由 Git Bash 而不是 PowerShell 承担 controller shell。交互 smoke 因此不使用 PowerShell 专有的 `Start-Sleep`；长命令由 runner 已必备的 Python 执行，在两种 shell 中保持同一取消语义。非交互 runner 也不依赖前台桌面的全局 `SendInput`：`ValuePattern` 与焦点窗口消息是主路径，`SendInput` 仅保留为显式回退。

## 证据边界

本审计的 schema v1 证据证明 Windows 原生安装后二进制的无密钥启动、项目会话、输入、provider round-trip、持久化、停止和重启恢复路径。schema v2 已继续覆盖原生失败矩阵，见 `2026-07-11-windows-native-failure-recovery.md`。两份证据均不声称：

- loopback 固定响应等于真实公网模型质量或鉴权可用性；
- Windows 证据自动代表 Linux/macOS 的 UI 交互；
- 候选包已经签名、发布或具备稳定版承诺。

后续应继续按提交、取消、审批、会话和状态查询纵向收缩 `control` 边界，并让 schema v2 失败矩阵持续留在 Desktop candidate workflow 中，而不是重新依赖易失败的截图索引动作。
