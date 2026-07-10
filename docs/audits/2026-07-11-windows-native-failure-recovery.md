# Windows 原生 Desktop 失败恢复审计

日期：2026-07-11

## 结论

M1 的原生失败场景缺口已在真实 Windows Wails/WebView2 窗口中关闭。扩展后的 `scripts/smoke_desktop_interaction.py` 使用隔离 home、隔离 workspace、localhost OpenAI 兼容 SSE fixture 和 Windows UI Automation，连续验证无效密钥、429、流中断、权限拒绝和工具超时。每个场景都必须满足：用户可见信号成立、Stop 消失且 Send 恢复、随后成功 turn 可执行。

这不是 reducer 或 Controller 进程内模拟。脚本启动 production Wails 二进制，经 Desktop tab、`control.Controller`、真实 OpenAI provider 实现、权限门控和内置工具执行完整路径；fixture 只替代不可控的公网模型端点。

## 实现

### 可访问状态合同

- `TurnDone.error.code` 进入 notice item；原生 warning 以 `notice-<error_code>` 暴露 AutomationId，并使用 `role=alert`。
- 重试提示不再被后续 `phase` / `notice` 立即清除；只有 Provider 输出、工具/审批事件或 `turn_done` 才清理 retry 状态。
- tool approval dialog、拒绝按钮和失败 tool card 分别暴露稳定的 `tool-approval-dialog`、`tool-approval-deny` 和 `tool-error-<call_id>`。
- Composer 的 `composer-input`、`composer-send`、`composer-stop` 合同保持不变；`composer-runstatus` 额外提供状态容器标识。

### 确定性 fixture

fixture 仅监听随机 `127.0.0.1` 端口。隔离进程通过 `REAMES_NATIVE_SMOKE_API_KEY=invalid-local-fixture-key` 注入合成凭据，配置文件只保存环境变量名，不保存该值。它不读取用户密钥，也不访问默认用户状态。

隔离配置固定：

- Desktop 新会话为 Ask approval mode；
- `write_file` 必须请求审批；
- 仅允许 fixture 的 `python -c` Bash 前缀直接执行；
- Bash 前台超时为 1 秒；
- 更新检查关闭，关闭行为为退出。

## 场景矩阵

| 场景 | 真实路径 | 原生可见证据 | 恢复证据 |
|---|---|---|---|
| 无效密钥 | 合成非空 key 收到 HTTP 401；已有成功请求使 transient-auth 策略完成 3 次请求后失败 | `notice-provider_auth` warning | Stop 消失，后续成功 turn 完成 |
| 429 | 首次请求返回 429 和 `Retry-After: 8`，第二次返回 SSE success | UIA 树出现 `retrying (1/10)…` | 原 turn 自动成功，后续 turn 再次成功 |
| 流中断 | 每次返回部分 SSE 后在 `[DONE]` 前关闭；初始请求加 3 次 Agent stream recovery 共 4 次 | `notice-stream_interrupted` warning | 部分 assistant 文本进入 canonical event log，后续 turn 成功 |
| 权限拒绝 | Provider 发出真实 `write_file` tool call，用户通过原生 approval shelf 拒绝 | approval dialog、拒绝按钮及 `tool-error-native-denied-call` | `native-denied.txt` 不存在，解释消息和后续 turn 成功 |
| 工具超时 | Provider 发出真实 `bash` tool call，30 秒命令被 1 秒工具上限终止 | `tool-error-native-timeout-call` | 模型收到 timeout tool result，解释消息和后续 turn 成功 |

## 本地原生证据

最终验证二进制：

```text
desktop/build/bin/reames-agent-desktop.exe
size: 47,897,600 bytes
SHA-256: 9011DD5E634F601D275BE893B743335A234116F420AD4B29D8305B2832814D1F
```

执行命令：

```powershell
python scripts/smoke_desktop_interaction.py `
  --exe desktop/build/bin/reames-agent-desktop.exe `
  --artifact desktop/build/bin/reames-agent-desktop.exe `
  --out artifacts/desktop-windows-interaction-smoke-local.json `
  --timeout-seconds 45
```

结果：

- `outcome = passed`，`errors = []`，`boundary_changes = []`；
- Provider 请求共 19 次，其中 failure scenario 请求数依次为 3、2、4、2、2；
- 五个 scenario 的 `signal_visible`、`idle_recovered`、`followup_succeeded` 全部为 `true`；
- `stream_partial_persisted`、`permission_denied`、`permission_write_blocked`、`tool_timeout_error_visible` 全部为 `true`；
- 原有长命令 Stop 和重启恢复继续通过，恢复后的 workspace 与 session path 完全一致；
- 两次 Desktop 清理成功，临时夹具删除，默认用户状态无变化。

## 自动化与验证

- Python 合同覆盖 fixture 路由、HTTP/SSE/tool-call payload、最新 user turn 分类、证据 schema 和前端 AutomationId 源码合同。
- 前端测试覆盖结构化 error code、retry 生命周期、approval AutomationId 和失败 tool card AutomationId。
- `desktop-candidate.yml` 已调用同一脚本；Windows candidate 会在 NSIS 静默安装后对安装后二进制执行矩阵。后续 M2 批次将证据升级为 schema v3，并增加认证设置与流中断续接的真实点击，详见 `2026-07-11-m2-error-session-control.md`。
- 本批本地通过 root build/vet/internal 全测、Desktop 全测、前端 `test:all`/production build、工具合同和 smoke 脚本合同。

## 证据边界

本审计证明 Windows production Wails 构建中的真实前端、Controller、Provider、权限和工具失败恢复语义。它不把 localhost fixture 冒充公网 Provider；真实 Provider 成功证据仍见 `2026-07-09-real-provider.md`。它也不代表 Linux/macOS UI 交互、真实 IM、云部署、代码签名或 notarization 已完成。
