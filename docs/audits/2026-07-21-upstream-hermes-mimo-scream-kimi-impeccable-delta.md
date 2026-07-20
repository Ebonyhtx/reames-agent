# Hermes / MiMo / Scream / Kimi / Impeccable 最新增量审计

日期：2026-07-21

## 范围与证据

本轮先对所有本地镜像执行 `fetch --prune --tags`，随后只比较 reviewed lock → `origin/main`：

| 上游 | 区间 | 提交 | 非 merge | 变化 |
|---|---|---:|---:|---:|
| Hermes | `a7d7c02c..456f18b1` | 184 | 177 | 249 files, `+14121/-1555` |
| MiMo Code | `ec413ade..b7b2092a` | 21 | 15 | 46 files, `+1898/-103` |
| Impeccable | `e4ab5e24..b906b414` | 10 | 10 | 424 patch files, `+33908/-6390` |
| Scream Code | `22a2adaf..ae8cd938` | 5 | 5 | 34 files, `+1038/-1251` |
| Kimi Code | `df689955..c2d7bebd` | 13 | 13 | 232 files, `+14625/-6854` |

提交拓扑、subject、逐文件统计和下述关键 patch 均直接来自这些 Git 对象；不是 release note 推断。
首次全 manifest deep 接受因 Git smart-HTTP 超时，没有改写 lock；随后用 GitHub 官方 commit/compare API 复核远端
head。Impeccable 在审查中从 `e6f3ce6d` 移动到 `b906b414`，精确接受门禁正确拒绝旧 SHA；补审新增两个提交后，
五项最小 manifest 的 `--accept-revision ID=FULL_SHA` 返回 `changed_count=0` 并更新 lock。

## Hermes

代码级重点包括：delegation live transcript 强制脱敏、skill sandbox 禁止挂载 master credential store、Windows env
probe 有界退出、URL credential alias 脱敏、text-only stream 无 finish reason 时按中断处理、Gateway obligation 只在
adapter 可投递时消费 attempt、queued follow-up 使用 finalized/silence-filtered response，以及 compression/context handoff、
session-scoped model/reasoning、provider picker、Desktop diff/tree/render 性能与 Kimi/Bedrock catalog 更新。

Reames 决策：

- **等价/更强**：subagent effect journal 已在落盘前走 `trust.Redact`，package Hook/MCP 使用最小环境和敏感读取阻断，
  不把 Reames home/master store 挂给 package；OpenAI/Anthropic 流在缺少完成终止事件时返回
  `StreamInterruptedError`，不会把 partial text 盖章为完整答复；outbound obligation 在
  `bindingForOutboundTarget` 成功之后才 `beginOutboundAttempt`，平台未启动不会消耗 attempt；最终答复统一走
  obligation/render 结算，不存在 Hermes 的 queued raw-result 旁路。
- **候选**：provider picker 分组、Desktop review diff 虚拟化和 rAF/tree 细粒度失效进入 P9/Desktop 性能；只有真实
  Wails/WebView benchmark 证明收益后才采用。新的 provider catalog/context/pricing 需各自官方协议/文档证明，不能从
  Hermes 静态表继承。
- **拒绝/不适用**：Python/Electron runtime、Hermes config 的 warn-after-write 宽松行为、遥测/远端服务和品牌 UI
  不进入 Reames；未知或危险配置继续 fail closed。

## MiMo Code

MiMo 修复 dream/distill write sandbox 的 `.mimocode/../` 逃逸并把写入限制在 memory/`.mimocode`，同时增加空 user
message source guard、GPT reasoning-only terminal 分类、interleaved transform 与 xAI OAuth plugin。

Reames 决策：

- 文件 writer 已通过 `os.Root`/rooted target、symlink-aware confinement、preview/checkpoint 与独立 writer worktree
  约束，不存在依赖字符串前缀的 dream/distill 特例；该 path-normalization bug 作为既有边界回归信号，不复制 TS guard。
- 普通空输入在 control/CLI 入口拒绝，合成控制消息由 typed 路径生成；保持该合同。GPT reasoning-only final 不采用：
  Reames 要求用户可见最终答复，只有明确 DeepSeek native reasoning-only stop 例外，其他协议继续触发 visible-answer
  recovery。
- xAI OAuth/OpenCode runtime、prompt bundle 与产品文档不采用；interleaved reasoning 只在相应官方 Provider 协议下验证。

## Scream Code

Scream 增加 60 秒 hard timeout、取消优先和多 provider 失败汇总，并把 Sogou/360/Baidu HTML scraper 放到
DuckDuckGo fallback 尾部；另有 streaming Write tail preview、permission badge 和 pi-tui ConPTY/render 修复。

Reames 决策：

- `web_search` 已有 request context、10 秒 `http.Client.Timeout` 和可见错误，不存在无限挂起或静默吞错；国内搜索
  fallback 是 M7 通用检索候选，但必须先补 HTML/redirect/anti-bot fixture、结果 URL 信任边界和维护成本，不直接复制
  易漂移 regex scraper。
- Write tail preview、permission mode 就近显示和 ConPTY chunk/render cadence 作为 CLI/TUI UX/performance 信号；
  必须在 Bubble Tea/Windows 原生路径复现和 benchmark 后采用，不能继承 pi-tui 版本修复。

## Kimi Code

Kimi 的主要变化是 unified transcript layer：turn/step/frame、task/interaction/attachment/todo、idempotent upsert、唯一
non-idempotent append offset/gap、turn cursor pagination 与 transport projection；同时 ACP auth 接受 configured provider
credentials、permission mode 广播到 live subagents、thinking effort session scope，并把 Web server 收回 foreground-only。

Reames 决策：

- unified transcript 的 offset/gap、global interaction 和 turn pagination 是 P9 App-Server/headless 的高价值合同，
  但 Reames 已有 `eventwire`、展示安全 transcript、Controller DTO 和持久 session log；未来应在这些 Go 权威层上演进，
  不复制 12k 行 TS 第二模型。
- ACP 启动本来就直接复用完整 Reames provider config/env，没有 OAuth-only token gate，configured-key 修复不适用。
- foreground Web server 与当前 `serve` 行为等价；session thinking/model scope 已有。live subagent permission-mode 广播进入
  P9 权限传播审计：若允许运行中改变父权限，必须定义 child snapshot/epoch、收缩立即生效和扩大 fresh approval，不能只做
  UI/event 广播。

## Impeccable

Impeccable 的大 diff 主要来自多 provider 生成副本；源机制集中在 Live polling/source lock/preflight、session/path
validation、progressive publish、event identity 与 hook finding cache。关键修复包括：基于 pid liveness 而非 mtime 判断锁
stale、只释放本进程 token 的锁、`--id` path sink 验证、claim-before-await 的 async generation preflight、错误回复不能
通配消费其他事件、当前 scan 替换历史 finding cache，以及 generated/large-file 精确跳过。审查进行中上游又新增
`e6f3ce6d..b906b414`：把 detector/Live 的 template extension 归并为单一 owner，按最长 suffix 支持 `.blade.php`，
并统一 source search、跳过状态/生成目录、broken symlink 和 Phoenix `lib/`/`.heex`/`.ex` 模板查找。

Reames 决策：

- writer worktree、workspace/ref lease、writer identity、acceptance journal 与 rollback pre-state 已覆盖 source lock 的核心
  所有权语义；ACP steering 和 durable channel claim 也使用明确活动回合/消息身份，不接受 wildcard ACK。作为 P10 Browser
  Control/交互式生成的并发与事件身份回归信号保留。
- Hook current-findings、file-scoped waiver 和 generated-size skip 是未来通用 Hook 治理候选；不得复制 Impeccable 的设计
  规则、站点、Live browser runtime 或多 provider 生成树。

## 冻结结论

本轮没有新增生产代码吸收项；适用安全/恢复信号在 Reames 已有等价合同，其余已明确进入 P9/P10/M7 或拒绝。锁可推进到：

- Hermes `456f18b19c4208115acbf0c6b226af49916b5480`
- MiMo `b7b2092a27def7b14df663fc7698165806185635`
- Impeccable `b906b41462c26c359e452040994685ce6d8e4008`
- Scream Code `ae8cd938c54abcb29a720af064b9e26d9b129d06`
- Kimi Code `c2d7bebd04106473bb4dbab2903756aa3f14a880`

该冻结不表示采用上游 runtime，也不替代当前 Reames 批次的 build/test/clean-clone/CI/CodeQL。
