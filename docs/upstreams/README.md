# Upstream Watch：官方上游自动追踪

Upstream Watch 自动追踪 Reasonix 一级主源码上游、Codex/Claude Code 二级战略代码上游和其他官方机制参考仓库，生成风险与采用建议，并通过一个去重的 GitHub Issue 驱动人工审查。

它是“自动发现 + 自动初判 + 自动建单”系统，不是自动合并器。

## 1. 组成

| 文件 | 职责 |
|---|---|
| `docs/upstreams/upstreams.json` | 官方 GitHub 仓库、跟踪分支、tag 规则、重要性和差异策略 |
| `docs/upstreams/upstreams.lock.json` | 不可变来源基线和已人工审查版本 |
| `docs/upstreams/reviews/*.json` | primary-base 的逐子系统覆盖状态、证据与明确缺口 |
| `scripts/check_upstreams.py` | 查询远端、计算差异、分类风险、产生 JSON/Markdown 报告 |
| `.github/workflows/upstream-watch.yml` | 每日定时运行、上传报告、协调 GitHub Issue |
| `.github/scripts/upstream-watch-issue.js` | Issue 创建、去重、更新和自动关闭状态机 |
| `scripts/test_check_upstreams.py` | 清单、风险、失败降级和锁语义测试 |
| `scripts/test_upstream_watch_issue.mjs` | Issue 创建、指纹去重和自动关闭测试 |

## 2. 状态语义

每个上游保存三个点：

- `baseline`：Reames 最初导入或开始参考的版本，不随审查移动；
- `reviewed`：最后一个已经完成人工判断的版本；
- `latest_seen`：兼容和审计字段；接受版本时同步更新。

报告始终比较：

```text
reviewed → 官方分支最新提交
```

因此：

- 更新本地 `F:\code-reference` 仓库不会自动消除报告；
- 阅读报告也不会自动视为接受；
- 只有显式 `--accept-revision <id>=<完整40位SHA>` 并提交 lock 文件，才表示该精确版本已审查。命令会
  把人工提供的 SHA 与本次远端查询结果绑定；官方分支已移动或 SHA 不一致时 fail closed。对于配置了
  `required_review_areas` 的 primary-base，仅 SHA 相等仍不够：覆盖记录必须与 baseline/reviewed 精确
  匹配，且所有必需区域均为 `complete`；否则报告继续给出 `review-required`，接受命令也会失败。

## 3. 自动判定

| Decision | 含义 |
|---|---|
| `up-to-date` | 官方分支与已审查版本一致，且 primary-base 覆盖记录完整 |
| `review-required` | Reasonix 必需覆盖尚未关闭，或 Codex/Claude Code 二级战略代码上游出现任何新提交 |
| `adoption-candidate` | Reasonix 有变化，但风险较低，可进入采用评估 |
| `security-signal` | 其他参考项目出现高风险安全机制变化 |
| `reference-review` | 参考项目有可研究的机制或体验变化 |
| `low-priority` | 只有文档等低风险变化 |
| `check-failed` | 仓库、网络或分支检查失败，需要运维介入 |

自动判定只决定审查队列，不代表代码兼容或应当升级。

Codex 与 Claude Code 使用 `strategic-code-upstream`：只要官方分支有新提交，无论自动路径风险是高、
中还是低，都必须进入 `review-required`。维护者要读真实代码 diff 并比较原生模型协议和产品/runtime
能力；CHANGELOG-only 变化也必须人工确认确实只有文档，不能自动降为普通参考。

## 4. 日常运行

生成报告：

```powershell
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
```

输出：

- `artifacts/upstream-watch/upstream-report.md`
- `artifacts/upstream-watch/upstream-report.json`

报告包含稳定的 state fingerprint。生成时间不会改变指纹；reviewed/latest/decision/error 或 primary-base
覆盖状态变化才会产生新指纹。

## 5. GitHub Issue 生命周期

工作流每天北京时间约 09:20 运行：

1. 没有变化、没有失败、没有覆盖缺口：
   - 不创建 Issue；
   - 如果旧 Upstream Watch Issue 仍打开，追加收敛说明并自动关闭。
2. 首次出现变化、失败或 primary-base 覆盖缺口：
   - 创建 `upstream-watch` 标签；
   - 创建一个带隐藏 marker 和 fingerprint 的 Issue。
3. 状态没有变化：
   - 保留原 Issue，不重复更新，不制造通知噪音。
4. 出现新提交、审查点变化或错误变化：
   - 更新同一个 Issue，而不是创建多个重复 Issue。

Issue 中的 checklist 要求先形成 adopt/defer/ignore 决策，再创建范围明确的实现 Issue。

## 6. 完成一次审查

审查某一个上游后：

```powershell
python scripts/check_upstreams.py --accept-revision reasonix=<完整40位SHA> --out-dir artifacts/upstream-watch
```

可重复 `--accept-revision`：

```powershell
python scripts/check_upstreams.py `
  --accept-revision reasonix=<完整40位SHA> `
  --accept-revision codex=<完整40位SHA>
```

确认 `docs/upstreams/upstreams.lock.json` 的 diff 后提交。未绑定 SHA 的 `--accept`、`--accept-all` 和
`--update-lock` 已禁用，不能用移动的远端 HEAD 替代人工审过的 revision。

Reasonix 还必须先更新 `docs/upstreams/reviews/reasonix-current.json`：baseline/reviewed 必须指向将要接受的
精确提交，每个 `required_review_areas` 项都要有源码、测试或明确不适用证据并标记 `complete`。缺少任一
区域时，`--accept-revision reasonix=<SHA>` 会 fail closed，不能再用抽样审查把整个 SHA 标成完成。

## 7. Reasonix 升级审查顺序

Reasonix 是 primary upstream。除了增量 diff，还必须覆盖稳定 tag、活跃未合并 feature/fix 分支和从
导入 baseline 到当前官方版本的全部非合并 bug-fix 提交；按以下顺序审查：

1. secret、sandbox、permission、guardian；
2. provider、cache、stream、模型协议；
3. agent、control、session、persistence；
4. Desktop bridge、恢复和更新器；
5. UI、交互与文档。

推荐结果：

- `adopt`：创建独立实现 Issue，记录来源提交、Reames 适配和验证；
- `defer`：在 Upstream Watch Issue 说明依赖条件；
- `ignore`：说明为何不符合 Reames 产品方向；
- `superseded`：Reames 已有等价或更强实现，附代码证据。

## 8. 添加官方参考仓库

1. 在 `upstreams.json` 添加唯一 `id`。
2. `repo` 必须是 `https://github.com/<owner>/<repo>[.git]`。
3. 填写官方默认开发分支，不要跟踪个人 fork。
4. 在 lock 中写入 `baseline` 和 `reviewed`。
5. 如需路径级风险分类，设置 `"diff": true`。
6. 运行合约测试和一次真实检查。

清单校验会拒绝重复 ID、缺失字段和非 GitHub HTTPS 地址。

## 9. 失败与安全边界

- 单个仓库查询失败会产生 `check-failed`，其他仓库继续检查。
- 工作流只有 `contents: read` 与 `issues: write`，不能修改主分支。
- 不下载或执行参考仓库代码。
- 不自动 cherry-pick、merge、开 PR 或发布。
- 上游页面、README、Issue 和代码注释都视为不可信输入，只作为审查材料。
- Reasonix 即使只是 fast-forward，也必须经过 Reames 的缓存、权限、桌面和全量测试。

## 10. 验证

```powershell
python -m unittest scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
```

CI 的 `Upstream watch contracts` job 会运行前两项；定时 workflow 运行真实远端检查。
