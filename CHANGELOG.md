# CHANGELOG — REAMES Agent TUI Redesign

> **项目**: REAMES Agent (独立分支，基于 Hermes Agent fork)
> **原仓库**: https://github.com/Ebonyhtx/reames-agent
> **美学参考**: [MiMo-Code](https://github.com/XiaomiMiMo/MiMo-Code) — 小米出品的 AI 编码助手 TUI 设计
> **设计灵感库**: `D:\awesome-design-md` — 美学设计参考集合

---

## 2026-06-16 — TUI 界面改造（MiMoCode 风格）

### 概览

将 Hermes 原生 TUI 改造为 MiMoCode 风格的两层界面架构：

```
Home 界面（入口）         Session 界面（对话）
┌──────────────────┐    ┌──────────────────┐
│ [动画 REAMES LOGO] │    │ ◆ Reames Agent    │
│ Where Models and  │    │ ╭──────────────╮  │
│ Agents Co-Evolve  │    │ │ 工具/技能面板  │  │
│                   │    │ ╰──────────────╯  │
│ ❯ 输入框          │    │ 对话消息...       │
│                   │    │ ────────────────  │
│ 回车 → Session    │    │ ❯ 输入框          │
└──────────────────┘    │ [状态栏: 模型/tokens]│
                         └──────────────────┘
```

### 改动清单

#### 1. 新增文件

| 文件 | 说明 |
|------|------|
| `ui-tui/src/routes/homeScreen.tsx` | Home 入口界面 — 动画 LOGO + 居中输入框 + tagline |
| `ui-tui/src/components/logoAnimation.tsx` | LOGO 光波动画 — 周期 80ms 相位扫描，颜色混合渐变 |

#### 2. 修改文件

| 文件 | 改动 |
|------|------|
| `ui-tui/src/banner.ts` | LOGO 从 "HERMES AGENT" + 蛇杖 ASCII art 替换为纯 "REAMES" 块字符 logo；移除蛇杖（`CADUCEUS`）相关艺术字 |
| `ui-tui/src/app.tsx` | 增加 `screen` 路由（`'home'` → `'session'`）；Home 提交后调用 `newPromptSession()` 开新会话 |
| `ui-tui/src/app/uiStore.ts` | 状态栏默认位置从 `'top'` 改为 `'bottom'` |
| `ui-tui/src/components/appLayout.tsx` | intro 消息中的大 `Banner` 替换为紧凑 `◆ Reames Agent` 标签；限制最大宽度 80% |
| `ui-tui/src/components/branding.tsx` | `SessionPanel` 强制窄布局（移除左栏蛇杖列），模型名/路径/Session ID 改到右侧单栏；移除未使用的 `ArtLines`、`caduceus` 导入 |
| `reames_cli/main.py` | 修复 `HERMES_PYTHON` 路径解析 — `sys.executable` 在 LibreOffice 等嵌入式 Python 中返回的是目录路径而非可执行文件路径，导致 TUI 网关启动失败（ENOENT） |

### 技术细节

#### Home 界面实现
- 使用 Ink `Box` 布局居中，包含 `AnimatedLogo` + tagline + 带边框输入框
- 输入框使用现有 `TextInput` 组件，支持 placeholder 和回车提交
- 提交后调用 `appActions.newPromptSession(query)` 而非 `appComposer.submit()`，确保每次都开新会话不加载旧记录

#### LOGO 动画实现
- `setInterval(80ms)` 驱动 24 相位扫描
- 每个相位计算 Gaussian 亮度分布，高亮度区域混合到白色
- 从左到右周期性扫过，模拟 MiMoCode 的流星光效

#### SessionPanel 窄布局
- 原宽布局根据终端宽度计算两栏（左: 蛇杖图 + 模型信息，右: 工具列表）
- 因移除蛇杖图，左栏宽度变为 4px 导致模型名被挤碎
- 修复: 强制 `wide=false`，始终使用窄布局，所有信息在单栏内显示

### Bug 修复

1. **TUI 网关启动失败 (ENOENT)** — `sys.executable` 在 LibreOffice 嵌入式 Python 中返回目录而非 `python.exe` 路径。修复: 检测到目录时尝试子目录和父目录中的 `python.exe`。
2. **SessionPanel 模型名显示异常** — `CADUCEUS_WIDTH = 0` 导致左栏宽度计算为 4px，模型名每个字符换行。修复: 强制窄布局。
3. **旧会话内容显示** — 使用 `appComposer.submit()` 会往当前加载的旧会话追加消息。修复: 改用 `appActions.newPromptSession()` 开新会话。
4. **npm install 检测循环** — `_tui_need_npm_install()` 因 workspace 锁定文件不匹配一直返回 True。修复: 完整执行 `npm install`。

### 待办/已知问题

- [ ] 默认入口改为 TUI（当前 `reames` 仍是 CLI，需 `reames --tui`）
- [ ] HOME 界面在特宽终端上缩进过大（排版优化）
- [ ] 复制/粘贴需要按住 Shift 拖选（鼠标追踪副作用）
- [ ] 登录欢迎信息和状态栏图标还未更新（仍有 "Nous Research" 等字样）

---

*美学设计参考: [MiMoCode](https://github.com/XiaomiMiMo/MiMo-Code) 的 CLI 入口界面设计 — 两层架构（Home → Session）、动画 ASCII LOGO、流星般光波效果、底部状态栏信息卡片。*
