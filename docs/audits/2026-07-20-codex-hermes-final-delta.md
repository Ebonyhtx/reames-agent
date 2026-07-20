# Codex / Hermes 最终移动增量审计

日期：2026-07-20

## 固定范围

- OpenAI Codex：`7844386e3de08febd13075eaaaf0e6f9dbe52c58..678157acaa819d5510adfe359abb5d0392cfe461`
- Hermes Agent：`1b17015f7a8d0c0d68b1f08aa389538e7fd172e3..dd418284db1804d33cd3d6d51c17bbfb1ad8f685`

Codex 是二级战略代码上游，三项提交均完成源码和测试级审查；Hermes 仍是三级机制参考，只登记适合
Reames 单一 Go/Wails runtime 的 UX/benchmark 信号。

## Codex

### `5a208c1fc3`：paginated thread 的显式名称

Codex 把用户显式 `name` 从派生 `title`/`preview` 中分离，并以 SQLite 作为 paginated history 的 canonical
store；rollout/name index 只保留 legacy 或 best-effort compatibility。缺少 state DB 时，paginated name update
在任何 persistence mutation 前失败。

Reames 当前 Desktop 已把 `TopicTitle`、manual/auto title source、topic identity 和 session transcript 分离，
没有 Codex SQLite/paginated App-Server 同构缺口，因此本批不新增第二套 store。P9 固定以下合同：未来
App-Server paginated threads 必须区分 explicit name 与 derived title/preview；显式 name 由 canonical metadata
store 持有，缺失 canonical writer 时 fail before mutation，兼容索引只能 best effort，read/list/search/resume
必须返回同一名称。

### `a97ae65362`：动态 transcript cell 重新测量

Codex 为 committed TUI cell 增加 stable/dynamic height 声明：稳定 cell 保留缓存，status refresh 或 visualization
由 placeholder 变为可用时重新测量，避免 overlay clipping。

Reames 当前 React/DOM transcript 没有固定 committed-cell 高度缓存，浏览器布局会对 status、Mermaid 和异步内容
自然 reflow，因此没有同构生产修复。P9/P10 保留棘轮：若以后引入 transcript virtualization，只能缓存显式声明
stable 的 cell；status、异步 visualization、字体/主题变化和宽度变化必须重新测量。

### `678157acaa`：消除多余 subagent metadata 请求

Codex 不再为 fresh/forked thread 执行不可能有历史 descendants 的 backfill；resume 才主动补全。agent picker
复用 backfill 已取得的 status，并让 live event channel 继续作为更强 liveness，避免重复 `thread/read`。

Reames 当前 Desktop/Controller 同进程、child transcript 为 canonical，没有同构 App-Server 请求。P9 headless
合同补强为：fresh/fork 不做 descendant history backfill；resume 或显式导航才补全；一次 backfill 返回的
thread+status 必须在同一交互中复用；live channel liveness 高于 stale snapshot；child completion 不触发 primary
completion 或顶层 `thread/read`。

## Hermes

### `0d2ad3993`：per-session color override

Hermes 以 durable lineage ID 保存 session-local color，解析优先级为 session override > project color，清除后
回退项目色。Reames 已有统一 project color 和 topic identity，但没有明确用户需求证明每 topic 调色值得增加设置
面；本批只登记 UX 候选。若采用，必须键入 durable topic identity、复用单一 resolver，并让 sidebar/tab/read model
同时生效，不把颜色写入 Provider prompt 或 transcript。

### `dd418284d`：可信 Desktop stream benchmark

该提交确认无段落分隔的单一 22 KiB block 会制造不代表常见 LLM 输出的重渲染压力，并补上：等待 backend socket
连接、焦点模拟/禁用后台节流、rAF recorder generation guard、真实段落流与无换行 worst-case 分离、五次中位数、
平台/Node/dev-build 元数据。

Reames 本批 `BenchmarkAsyncRuntimeEmitterCoalescedBacklog` 只证明 Go queue 合并/锁开销，已明确不冒充 WebView
frame pacing。P9 原生 benchmark 必须吸收上述方法：connected/settled gate、真实段落和单 block stress 双场景、
无后台节流、每次 recorder generation 隔离、多次中位数，以及 dev/prod 结果分开记录。

## 结论

- 本轮没有 OpenAI Responses、Realtime、tool wire、reasoning、Browser/CDP 或 Claude 协议变化；
- 不需要改动本批 M6 微信持久轮询或 Desktop queue 生产实现；
- Codex 三项进入 P9 App-Server/virtualization 合同，Hermes 两项进入 UX/benchmark 候选；
- 只接受本文固定的完整 SHA；如果远端 HEAD 再移动，接受命令必须 fail closed 并重新审查新范围。
