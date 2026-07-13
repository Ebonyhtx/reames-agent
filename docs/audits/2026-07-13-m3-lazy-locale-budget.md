# M3 多语言按需加载与预算审计

日期：2026-07-13

状态：已交付；commit `bbdddde`，普通 CI `29214262280` 8/8、CodeQL `29214262276` 3/3

## 缺口

Desktop 虽已将关闭态界面拆出首启路径，但 `i18n.tsx` 仍静态导入英文、简中和繁中三份完整词典。2026-07-13 拆分前 production base initial JS 为 1,213,626 B，只比 1,250,000 B 上限少约 36 KB；任意语言的用户都必须先加载三份词典。单纯把词典改成异步又会让中文系统首帧短暂显示英文或原始 key，因此体积和首帧正确性必须一起解决。

## 实现

- 英文词典保留同步导入，作为损坏 chunk、未知 key 和加载失败的确定性兜底。
- 简中与繁中改为两个命名稳定的动态 chunk；每种 locale 共享一个在途 promise，加载完成后缓存词典，避免并发设置和 StrictMode 重复请求。
- `main.tsx` 在创建 React root 前先读取 legacy 迁移值，否则调用轻量本地 `DesktopStartupSettings` 取得权威保存语言；只有 auto 模式才检测 OS。它只预取最终的一个离线内嵌词典，并把同一 preference 传给 `LocaleProvider`，因此 OS 与保存偏好不同时不会加载第二份词典或闪现错误语言。若 bridge/词典读取失败则回退 auto/英文并继续启动，不把异常变成空白窗口。
- 设置页运行期切换语言时继续显示当前完整词典，待目标词典就绪后原子切换；模块级 `t()`、React context 与 `document.lang` 同步跟随真正已加载的 locale。
- 真实产物预算区分不含异步词典的 base initial 与最坏本地化首启；Vite manifest 递归汇总 locale chunk 及其静态传递依赖，并与 HTML initial 集合去重。同时要求恰好生成两个 locale JS 文件，避免命名、共享依赖或打包回归使预算虚假通过。

## 结果与预算

```text
                              拆分前     已交付 bbdddde      变化
entry JS                    624,409 B      623,577 B        -832 B
base initial JS           1,213,626 B      984,616 B    -229,010 B (-18.9%)
worst localized startup   1,213,626 B    1,100,036 B    -113,590 B (-9.4%)
largest locale                    n/a      115,420 B
initial CSS                 611,424 B      611,424 B             0
largest JS                  704,186 B      704,186 B             0
```

收紧后的硬门槛为 entry 640,000 B、base initial 1,025,000 B、最坏本地化首启 1,150,000 B、initial CSS 620,000 B、最大 JS 725,000 B、initial 文件最多 6 个、locale 文件恰好 2 个。首启 HTML 现在是 1 个 entry 加 5 个 module preload，共 6 个 initial JS 文件，但传输字节明显下降；非目标中文词典不进入该语言的首启路径。

## 参考与取舍

DeepSeek Reasonix `main-v2@0e0cb63c712e89f8ab8f23cd1a30f374f9f386ed` 仍在 `i18n.tsx` 静态导入三份词典；本批保留其 locale API 和英文回退语义，在 Reames 的既有 Vite 分块、Wails 离线资源和真实产物预算之上做适配，不复制另一套 i18n runtime。`F:\Reames-Lite` 未提供可直接复用的 Wails 首帧机制，本批不声明其为代码来源。

## 证据边界

`i18n-lazy-locale.test.tsx` 从全新模块图证明：初始只有英文；OS 为繁中而权威保存偏好为简中时只加载简中，Provider 首次 render 已是简中；切回英文再恢复 auto 时按 OS 加载繁中，React、模块 translator 与 `document.lang` 一致；受限 localStorage 也不会在 mount 前中止启动。source contract 证明启动先读取权威保存值、只在其为空时才读取 legacy 偏好，中文词典没有静态导入，且首帧 mount 确实等待最终词典预取。真实 dist manifest 检查证明两个 locale chunk、递归依赖图、base 与最坏本地化字节均在硬门槛内。

当前已通过：

```text
corepack pnpm typecheck                                      PASS
corepack pnpm test:typecheck                                 PASS
tsx src/__tests__/i18n-lazy-locale.test.tsx                  PASS (18)
tsx src/__tests__/bundle-contract.test.ts                    PASS (22)
node scripts/test-bundle-budget.mjs                          PASS (5)
corepack pnpm build                                          PASS
node scripts/check-bundle-budget.mjs                         PASS
corepack pnpm test:all                                       PASS
go build ./... / go vet ./... / go test ./internal/...       PASS
desktop/go test ./...                                        PASS
docs/public/release/deploy/tool contracts                     PASS
python -m unittest scripts.test_smoke_desktop_interaction
  scripts.test_smoke_desktop_native -v                        PASS (38, 1 skipped)
python -m unittest discover -s scripts -p 'test_*.py' -v      PASS (79, 2 skipped)
Wails v2.12.0 production Windows build                       PASS
smoke_desktop_native.py --max-startup-seconds 8
  --max-warm-startup-seconds 6                                PASS
smoke_desktop_interaction.py --timeout-seconds 45             PASS
```

最终 Windows production 可执行文件大小为 48,044,032 B，SHA-256 为 `7CE473389D54DA662299DB03BE80FD571D95E7AA3A1E671FC8855866E67D0783`。隔离 HOME 的冷启动首次/稳定响应为 0.516/1.516 秒，同 HOME warm relaunch 为 0.500/1.500 秒，均满足 8/6 秒本地门槛；进程、临时目录和默认状态边界检查通过。最终 UIA 交互完成 19 次 loopback provider 请求、五类失败恢复、停止和同 session path 重启恢复，`recovery_verified=true`、清理成功、边界变化与 errors 均为空。原生 UIA 使用保存英文的既有稳定夹具，证明新启动 preflight 与打包资源没有破坏主流程；OS 繁中与保存简中不一致时的单词典、保存值优先，以及恢复 auto 后按 OS 加载繁中的正确性由组件、source contract 和真实 manifest 产物共同证明，不把英文原生交互冒充中文 UI 证据。

这些证据证明该批 Windows 源码 production 的离线词典首启和主流程，没有替代三平台安装候选、签名发布或真实公网 Provider。commit `bbdddde` 已推送到 `origin/main`；普通 CI run `29214262280` 的 8 个 jobs 与 CodeQL run `29214262276` 的 3 个分析目标均通过。后续首启图拆分应以本页的已交付产物作为比较基线，不回写或冒充本批历史结果。
