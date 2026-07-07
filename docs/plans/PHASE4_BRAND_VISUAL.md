# Phase 4: 品牌与视觉系统重塑

> 状态：⏳ 待执行
> 参考：F:\code-reference\impeccable（Neo Kinpaku 设计系统）、F:\code-reference\scream-code（主题令牌）

## 4.1 设计令牌系统

### 改造步骤

1. **色彩空间**
   - 全项目切换到 OKLCH 色彩空间
   - 主色（品牌金/铜）：`oklch(0.72 0.15 85)`
   - 辅色（冷静蓝/青）：`oklch(0.65 0.10 230)`
   - 底色（深 lacquer）：`oklch(0.12 0.02 260)`
   - 语义色：success, warning, error, info, dim, accent

2. **CSS 变量层**
   - 在 `desktop/frontend/src/styles.css` 中建立 `:root` 变量：
     ```css
     --color-brand: oklch(0.72 0.15 85);
     --color-brand-dim: oklch(0.55 0.10 85);
     --color-surface: oklch(0.12 0.02 260);
     --color-surface-raised: oklch(0.16 0.02 260);
     --color-text: oklch(0.92 0.01 260);
     --color-text-dim: oklch(0.60 0.01 260);
     ```
   - 深浅主题切换：`[data-theme="light"]` 覆盖变量值

3. **设计原则**
   - 无玻璃效果（no glassmorphism）
   - 金色承载品牌，不滥用
   - 避免 AI 霓虹感
   - 仅 OKLCH，不用 hex/rgb

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | CSS 变量完整 | `grep '--color-' desktop/frontend/src/styles.css` 覆盖所有语义色 |
| A2 | 深浅主题切换 | 切换 `data-theme` 属性，所有颜色正确变化 |
| A3 | 无 hex/rgb 硬编码 | `grep -E '#[0-9a-fA-F]{3,6}' desktop/frontend/src/styles.css` — 仅 CSS 变量定义处有 |

## 4.2 Desktop 前端视觉刷新

### 改造步骤

1. **启动画面** — `StartupSplash.tsx`
   - Logo 替换（准备 Reames Agent 品牌 logo SVG）
   - 产品名和版本号
   - 加载动画

2. **欢迎页** — `OnboardingOverlay.tsx`
   - 文案："连接 Reames Agent"
   - API Key 输入引导
   - 隐私说明

3. **窗口标题栏** — `AppChrome.tsx`
   - 品牌色应用到标题栏
   - 窗口控制按钮样式

4. **设置面板** — `SettingsPanel.tsx`（298KB）
   - 品牌相关文案替换
   - 图标风格统一

5. **状态栏** — `StatusBar.tsx`
   - 底部产品名和状态指示器
   - 缓存命中率显示

6. **命令面板** — `CommandPalette.tsx`
   - 产品名更新

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | CSS 构建成功 | `cd desktop/frontend && npm run build` |
| A2 | 视觉一致性 | 手动截图对比新旧版本 |
| A3 | 前端测试通过 | `cd desktop/frontend && npm test` |
| A4 | 无硬编码旧品牌色 | `grep -rn 'reasonix\|Reasonix' desktop/frontend/src/` 返回 0 |

## 4.3 CLI 视觉刷新

### 改造步骤

1. **启动 Banner** — `internal/cli/chat_tui.go`
   - `◆ reames-agent vX.X.X · model · cwd`
   - 品牌色 `◆` 符号

2. **帮助文本** — `internal/cli/cli.go`
   - 所有 `reasonix` 引用已替换
   - 命令示例更新

3. **Bubble Tea 主题** — 颜色映射更新
   - accent → 品牌金
   - dim → 灰蓝
   - ok → 绿色

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | CLI 启动显示正确品牌 | `./bin/reames-agent` 截图验证 |
| A2 | `/help` 命令无旧引用 | `grep 'reasonix' internal/cli/*.go` → 0 |

## 4.4 Logo 和图标

### 改造步骤

1. **Logo SVG** — 替换以下文件：
   - `desktop/frontend/src/assets/logo-symbol.svg`
   - `desktop/frontend/src/assets/logo-wordmark.svg`
   - `docs/logo.svg`
   - `desktop/build/appicon.svg`

2. **Favicon** — `docs/favicon.svg`

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | Desktop 启动显示新 logo | 启动 Desktop 应用截图 |
| A2 | 文档站点显示新 logo | 打开 `site/` 构建预览 |

## 总回归检查

```bash
cd desktop/frontend && npm run build            # 前端构建
go build ./...                                   # Go 构建
go test ./internal/cli/... -count=1             # CLI 测试
grep -rn 'reasonix\|Reasonix' --include='*.go' --include='*.ts' --include='*.tsx' --include='*.css' -l | grep -v node_modules | wc -l  # 应为 0
```
