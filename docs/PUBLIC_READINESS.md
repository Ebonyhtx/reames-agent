# 公开仓库前检查清单

> 状态：公开前门禁  
> 更新：2026-07-17

本清单的目标不是证明项目已经稳定发布，而是证明“把仓库设为 public”不会暴露明显错误入口、上游身份残留、误触发生产发布或缺失基本治理文件。

自动化入口：

```powershell
python scripts/check_public_readiness.py
```

CI 已把该脚本作为 public-readiness job 执行。仓库公开后仍保留这个门禁，防止 README、发布流程和安全边界回退。

## 公开前必须满足

- 根目录存在 `README.md`、`LICENSE`、`NOTICE.md`、`SECURITY.md`、`CONTRIBUTING.md` 和 `AGENTS.md`。
- `README.md` 只描述当前入口：CLI、`serve`、`gateway run`、`gateway install --dry-run`，并可说明 `bot start` 是兼容入口；后台 gateway service 文档必须说明 `--home`/`REAMES_AGENT_HOME` 绑定；不得再出现旧 `gateway start`。
- 示例密钥使用占位符，不使用看起来像真实 token 的 `sk-xxx`。
- `LICENSE` 同时保留 Reames Agent 和 Reasonix 的 MIT 版权归属；`NOTICE.md` 说明 DeepSeek Reasonix 来源并保留 go-tuf 归属，`third_party/go-tuf/` 保存 Apache-2.0 LICENSE/NOTICE。
- `.github/CODEOWNERS` 指向当前仓库维护者，不再请求上游维护者审查。
- 生产发布仍保持关闭：tag 不自动发布 GitHub Release、npm、Homebrew、对象存储或 updater。
- 旧 worker/site 目录和 deployment workflows 已删除，public-readiness 会阻止它们或旧 Hermes/Python runtime 根目录重新进入主分支。
- Docker、compose、systemd 和部署文档继续通过 `scripts/check_deploy_contracts.py`。
- Upstream Watch 只产出报告和 Issue，不能自动合并或自动升级。
- CodeQL advanced setup workflow 已恢复，覆盖 Go、JavaScript/TypeScript 和 GitHub Actions。
- 官方 JavaScript actions 使用内置 Node.js 24 的 major；公开门禁扫描 `.yml/.yaml`，拒绝 checkout/setup/upload 等回退到 Node.js 20 major 或使用未经审计的 commit pin。

## 仓库公开后的第一轮设置

公开仓库后，请在 GitHub 页面完成这些人工设置：

1. 确认默认分支是 `main`。
2. 打开 Actions，并确认 `CI`、`Upstream Watch`、`Release candidate` 可见。
3. 打开 Dependabot alerts 与 secret scanning；CodeQL workflow 已在仓库内，公开后观察首次 CodeQL run。
4. 确认 `Settings → Actions → General` 中 fork PR 的权限保持最小化，不给 fork PR 写权限或 secrets。
5. 暂时不要配置 npm、Homebrew、Cloudflare、R2、crash report、telemetry 或 updater secrets。
6. 首次公开后立刻观察一次远端 CI；公开仓库前的本地通过不等于远端通过。

## 已关闭的遗留面

初始迁移曾短期隔离一整套 Hermes/Python runtime、Electron Desktop、旧 TUI、plugins、tests、Python/Node package 元数据以及 `site/`、`workers/`。在 Go/Wails 主产品、Gateway、插件、恢复和发布契约形成替代实现后，这些目录已从当前树删除；Git 历史和 `F:\code-reference\Hermes` 继续承担参考与追溯职责。

持续规则：

- 不在主仓库重新 vendor 参考项目的 runtime、UI、依赖体系或测试树；
- `scripts/check_public_readiness.py` 拒绝已删除 legacy 根目录、根包元数据、worker/site workflows 和活跃产品面中的 Hermes/Nous 运行品牌；
- `REASONIX.md` 只作为旧文件名兼容，新的项目记忆和所有用户提示统一使用 `AGENTS.md`；
- 若未来恢复网站、托管 worker 或新语言 runtime，必须作为新的有所有权产品面重新设计、测试和发布，而不是复活旧目录。

## 当前结论

达到本页和脚本门槛后，仓库可以公开为“开发中项目”，但不应宣传为稳定发布。真实运营 registry、云节点、IM 应用和签名发布链仍按路线图保持外部证据边界。
