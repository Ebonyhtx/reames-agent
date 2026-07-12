# M3 主题对比度与焦点审计

日期：2026-07-13

状态：完整本地门禁与真实浏览器交互通过；commit 与远端证据待完成

## 缺口

Desktop 有 Graphite、Aurora、Slate、Carbon、Nocturne、Amber 六套视觉风格，同时支持显式深浅色、跟随系统和独立的 Creation 画布覆盖。此前这些 token 靠人工复制维护，没有可执行的对比度下限；较淡文字、浅色状态色和亮色主按钮已经出现系统性失败，显式浅色与自动浅色也发生漂移。焦点环 token 还被误作 `outline` 颜色，旧主题高特异性规则会覆盖新规则；Creation 从根继承焦点环时，内层间隔色按工作台背景而不是本地画布计算。

设置内切换“工作台/创作”会重挂载侧栏。旧 `useDialogFocus` 只保存 opener DOM 节点，因此即使切回工作台，关闭设置时也会因原节点已经断开而把焦点落到 `body`。

## 实现

- 新增 `theme-contrast.test.ts`，直接解析生产 `styles.css` 并按 CSS 特异性和源顺序合并 token。合同覆盖六主题 × 深浅色 × 普通/Creation 两种产品模式，检查正文、弱化文字、强调色、成功/警告/错误、diff 增删、主按钮和焦点指示器；小文本和主按钮要求至少 4.5:1，非文本焦点指示器要求至少 3:1。
- 显式浅色与系统自动浅色的关键 token 必须逐项一致；Creation 必须在自己的画布作用域重算双层不透明焦点环。合同同时禁止把 shadow token 用作 outline 颜色，并要求存在 `forced-colors: active` 焦点规则。
- 集中建立对比度下限层，修正六套浅色强调色、淡文字、状态色、主按钮背景和 Creation 覆盖；Amber 自动浅色补齐与显式浅色一致的工作台底色，Graphite 自动浅色 Creation 补回白色主按钮文字。
- 焦点环统一为“本地背景 2px 间隔 + 强调色 4px 外环”，提高规则特异性以压过旧风格 token；Creation 在深色、浅色和跟随系统作用域重新声明，确保 `var(--bg)` 按局部画布求值。高对比模式使用系统 `Highlight` 且移除自定义 shadow。
- `useDialogFocus` 支持 `data-dialog-return-focus` 语义键：原 opener 节点断开后寻找当前已连接的等价入口。工作台与 Creation 的设置按钮共用稳定键，退出动画后二次恢复仍保留嵌套顶层保护。

## 参考与取舍

Impeccable 的静态颜色规则使用 WCAG 4.5:1 小文本和 3:1 非文本边界，本批吸收“把设计建议变成可执行门禁”的机制，没有复制其前端框架或规则引擎。MiMo 提供主题与 OKLCH 设计参考，但没有可直接证明 Reames 生产 token 的合同；DeepSeek Reasonix 也没有覆盖六主题与 Creation 级联，因此都不作为完成证据。

合同只解析可确定计算的实色 token，不声称替代完整浏览器 CSS 引擎、透明叠色分析或人工视觉审查。`forced-colors` 在本批有静态生产规则，尚未冒充 Windows High Contrast 原生交互证据。

## 当前证据

```text
corepack pnpm test:theme-contrast                         PASS (305)
corepack pnpm test:dialog-focus                           PASS (10 hook + 2 palette)
corepack pnpm typecheck / test:all / build                PASS
in-app Browser http://127.0.0.1:5173                     PASS
desktop/go test ./... -count=1 -timeout 10m               PASS
go build ./... / go vet ./... / go test ./internal/...    PASS
docs/public/release contracts                             PASS
```

首版根主题合同暴露 48 个失败；扩展到 Creation 后又捕获 Graphite 自动浅色仍使用深色主按钮文字。最终 305 项合同全部通过，且合同本身进入 `build`，production 构建不能绕过。

真实浏览器在设置 → 外观中依次验证 Graphite、Carbon、Amber 显式浅色的最终计算 token；三者分别得到强调色 `#bd3f18`、`#08766d`、`#b9441c`，状态色统一达到下限，双层焦点环使用各自背景和强调色。Amber Creation 得到本地 `--bg=#f6f4f1` 与 `0 0 0 2px #f6f4f1, 0 0 0 4px #b9441c`。随后完成“工作台 → 创作 → 工作台”重挂载回环，关闭设置后活动元素是带 `data-dialog-return-focus=settings` 的新“设置”按钮；恢复自动主题 + Graphite + 工作台后 warning/error 为 0。

浏览器证据运行于 Vite dev 和确定性 mock bridge，证明 React 交互、真实 CSS 级联与焦点生命周期，不冒充 production Wails、屏幕阅读器或 Windows High Contrast 原生证据。

当前 production 产物为 entry JS 624,233 B、initial JS 1,213,450 B（5 files）、initial CSS 611,424 B、largest JS 704,186 B，全部低于既有硬预算。
