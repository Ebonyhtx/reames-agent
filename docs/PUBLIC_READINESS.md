# 公开仓库前检查清单

> 状态：公开前门禁  
> 更新：2026-07-09

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
- `LICENSE` 同时保留 Reames Agent 和 Reasonix 的 MIT 版权归属；`NOTICE.md` 说明 DeepSeek Reasonix 来源。
- `.github/CODEOWNERS` 指向当前仓库维护者，不再请求上游维护者审查。
- 生产发布仍保持关闭：tag 不自动发布 GitHub Release、npm、Homebrew、对象存储或 updater。
- 遗留 worker/site workflow 不提供 `workflow_dispatch` 手动部署入口；如需恢复，必须先完成 `docs/RELEASING.md` 的生产门槛。
- Docker、compose、systemd 和部署文档继续通过 `scripts/check_deploy_contracts.py`。
- Upstream Watch 只产出报告和 Issue，不能自动合并或自动升级。
- CodeQL advanced setup workflow 已恢复，覆盖 Go、JavaScript/TypeScript 和 GitHub Actions。

## 仓库公开后的第一轮设置

公开仓库后，请在 GitHub 页面完成这些人工设置：

1. 确认默认分支是 `main`。
2. 打开 Actions，并确认 `CI`、`Upstream Watch`、`Release candidate` 可见。
3. 打开 Dependabot alerts 与 secret scanning；CodeQL workflow 已在仓库内，公开后观察首次 CodeQL run。
4. 确认 `Settings → Actions → General` 中 fork PR 的权限保持最小化，不给 fork PR 写权限或 secrets。
5. 暂时不要配置 npm、Homebrew、Cloudflare、R2、crash report、telemetry 或 updater secrets。
6. 首次公开后立刻观察一次远端 CI；公开仓库前的本地通过不等于远端通过。

## 已知不等于阻塞公开的遗留面

仓库里仍保留部分从旧项目继承的辅助脚本和 workers，例如旧 release 辅助脚本、实验桥接器和 worker 目录。正式 installer 已替换为 Reames 自有 `scripts/install.sh`、`scripts/install.ps1`、`scripts/install.cmd`；旧 Hermes gateway 原型不再作为仓库脚本入口保留。

处理规则：

- 若没有当前 Reames 等价实现，先隔离并标注，不急于大删；
- 若被公开文档、workflow 或安装入口引用，必须先迁移或移除；
- 后续每批清理都要证明“无运行引用、无发布依赖、有替代实现或明确无价值”。

## 当前结论

达到本页和脚本门槛后，仓库可以先公开为“开发中项目”，但不应宣传为稳定发布。公开后我会继续做远端 CI 观察、CodeQL/secret scanning 补齐、M1 真实任务闭环和云端 Agent 后续实现。
