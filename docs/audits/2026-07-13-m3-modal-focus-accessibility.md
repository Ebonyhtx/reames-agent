# M3 模态焦点与可访问性审计

日期：2026-07-13

状态：已交付（commit `9d8368c`；普通 CI `29203366720` 8/8、CodeQL `29203366703` 3/3）

## 缺口

Desktop 的模态行为此前分散：命令面板、快捷键帮助和图片预览各自处理部分焦点或 Escape；设置与历史虽然视觉上是模态层，却没有 `role="dialog"`、`aria-modal`、稳定标题关联或 Tab 围栏。懒加载进一步放大时序问题：chunk 挂载时可能已经丢失 opener，退出动画完成后 React 移除曾持有焦点的 input 还会把焦点重置到 `body`。

审批、提问和清理上下文使用 `PromptShelf` 的非阻断 `aria-modal="false"` 语义，不属于本批真正模态层，不能为追求统一而错误锁住整个应用。

## 实现

- 新增 `useDialogFocus`：聚焦显式初始控件或首个可聚焦元素，无控件时聚焦 dialog root；Tab/Shift+Tab 在 DOM 顶层 `aria-modal=true` 内循环，嵌套背景 dialog 不抢焦。
- 支持延迟挂载的最多四帧有界等待；cleanup 先恢复 opener，并观察 dialog 实际移除，在退出动画后再次恢复，防止 React selection restoration 把焦点落回 `body`。观察器 1 秒自动释放，目标断开或其他顶层 dialog 存在时不抢焦。
- Settings、History、Command Palette、Shortcuts、Image Viewer 和 Onboarding 接入共享生命周期。Settings/History 补 dialog、modal、labelledby 与 root tabindex；Onboarding 补首次启动 dialog 语义。
- 命令面板输入改为 `combobox`，关联唯一 listbox、`aria-expanded`、`aria-autocomplete` 和当前 option 的 `aria-activedescendant`。
- 命令面板首次打开后保持组件挂载，由内部 `useMountTransition` 完成 200ms 退出动画；App 在触发动作发生时显式记录 opener，覆盖 lazy chunk 到达前 active element 已变化的情况。AppChrome 与 topicbar 鼠标入口传 `event.currentTarget`，快捷键入口捕获当前键盘焦点。

## 参考与取舍

Kimi Code 的 `useDialogFocus` 提供了“挂载时聚焦、卸载时恢复”的小型 composable 思路。Reames 没有复制 Vue 实现，也没有引入新 UI 框架；基于现有 React refs、`useMountTransition` 和 DOM 合同补齐 Tab 围栏、嵌套顶层判定、延迟 chunk、退出动画后二次恢复。DeepSeek Reasonix 当前只有各组件局部焦点逻辑；MiMo 的 Kobalte dialog story 仍将 focus trap 标为 TODO，因此不作为完成证据。

## 当前证据

```text
corepack pnpm typecheck                                      PASS
corepack pnpm test:typecheck                                 PASS
corepack pnpm test:dialog-focus                              PASS (9 hook + 2 palette)
tsx src/__tests__/command-palette-interactions.test.tsx      PASS (5)
tsx src/__tests__/bundle-contract.test.ts                    PASS (17)
affected image/settings/startup component tests              PASS (57)
corepack pnpm test:all                                       PASS
corepack pnpm build                                          PASS (bundle budget enforced)
python scripts/check_docs_contracts.py                       PASS
python scripts/check_public_readiness.py                     PASS
in-app Browser http://127.0.0.1:5173                         PASS
```

共享 hook 测试覆盖初始焦点、正反向 Tab 循环、无控件 root fallback、关闭恢复、嵌套顶层与断开 opener；命令面板组件测试额外覆盖延迟挂载和退出动画后恢复。真实浏览器验证：命令面板打开后 combobox 获焦并携带 listbox/active-descendant，末项 Tab 回到 combobox，Escape 关闭且 200ms 动画移除后焦点回到“命令”；设置和历史均暴露唯一模态 dialog，关闭后分别回到“设置”“历史”。全新浏览器标签完成命令面板回环后 warning/error 日志为 0。

该证据是 Vite dev + 确定性 mock bridge 的真实浏览器交互，不冒充原生 Wails 屏幕阅读器或 Windows UIA 证据。Windows 显示缩放由同一交付批次的独立审计覆盖；主题对比度和入口重挂载焦点恢复在后续 `2026-07-13-m3-theme-contrast.md` 中继续收口。原生键盘/读屏和 Windows forced-colors 抽查仍属于后续 M3。
