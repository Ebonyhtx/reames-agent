# Script surface cleanup audit

Date: 2026-07-09

## 背景

公开仓库后，`scripts/` 会被用户和后续维护者自然理解为当前 Reames Agent 的官方维护入口。审计发现该目录仍保留多份继承自 Hermes/Python 时代的脚本，包含旧家目录、旧包名和旧发布仓库引用，例如 `HERMES_HOME`、`~/.hermes`、`hermes_cli`、`reames_cli`、`NousResearch/hermes-agent`。

这些脚本有历史参考价值，但当前 Go 版 Reames Agent 的安装、部署、CI、上游追踪和发布候选流程已经有新的官方入口。继续把旧脚本放在 `scripts/` 顶层会制造两个问题：

1. 用户可能把旧 Python/Hermes 流程误认为当前官方部署方式。
2. 自动化或维护者可能误触旧 release、Open WebUI、WhatsApp bridge、live test 等路径。

## 清理原则

- 保留当前 Go 项目实际使用的脚本：安装器、部署契约、公开门禁、发布候选契约、上游追踪、桌面构建辅助。
- 删除未被当前 CI 调用、且明确绑定旧 Hermes/Python 运行时的脚本。
- 不在本轮大规模删除旧 Python 源码树；`reames_cli/`、`gateway/`、`tests/` 等历史目录需要后续单独做 legacy quarantine 审计。
- 参考项目仍以 `F:\code-reference` 和 `docs/REFERENCE_GOVERNANCE.md` 为准，不把旧脚本当成当前产品入口。

## 本轮删除的旧脚本面

- 旧 live test / tool-search harness：`scripts/LIVETEST_README.md`、`scripts/tool_search_livetest.py`、`scripts/analyze_livetest.py`
- 旧 Hermes/Python 目录生成器和模型目录：`scripts/build_model_catalog.py`、`scripts/build_skills_index.py`
- 旧 Python runtime/diagnostic 脚本：`scripts/discord-voice-doctor.py`、`scripts/profile-tui.py`、`scripts/keystroke_diagnostic.py`、`scripts/benchmark_browser_eval.py`
- 旧 Python Docker/config/android helper：`scripts/docker_config_migrate.py`、`scripts/install_psutil_android.py`
- 旧 release/contributor 工具：`scripts/release.py`、`scripts/contributor_audit.py`
- 旧 Python pytest runner：`scripts/run_tests.sh`、`scripts/run_tests_parallel.py`
- 旧 data compression sample：`scripts/sample_and_compress.py`
- 旧 Open WebUI bootstrap：`scripts/setup_open_webui.sh`
- 旧 Node bootstrap helper：`scripts/lib/node-bootstrap.sh`
- 旧 WhatsApp bridge package：`scripts/whatsapp-bridge/`
- 旧 Python Windows footgun checker：`scripts/check-windows-footguns.py`

## 当前保留的脚本职责

- `scripts/install.sh`、`scripts/install.ps1`、`scripts/install.cmd`：Reames source-build 安装入口，支持 gateway service dry-run。
- `scripts/check_public_readiness.py`：公开仓库门禁。
- `scripts/check_deploy_contracts.py`：服务器部署契约门禁。
- `scripts/check_release_contracts.py`：发布候选安全门禁。
- `scripts/check_upstreams.py`、`scripts/test_check_upstreams.py`、`scripts/test_upstream_watch_issue.mjs`：官方上游/参考项目 advisory watch。
- `scripts/backfill-issue-labels.mjs`：GitHub issue label 运维。
- `scripts/cache-guard.sh`、`scripts/check-cache-impact.sh`：缓存影响检查。
- `scripts/desktop-build.sh`、`scripts/desktop-test-times.py`、`scripts/resolve-desktop-release.sh`：桌面构建/发布辅助。
- `scripts/kill_modal.sh`、`scripts/lint_diff.py`、`scripts/verify-baseline.ps1`：本地维护辅助。

## 新增防回归

`scripts/check_public_readiness.py` 新增 script surface gate：

- 禁止旧官方感强的脚本文件名回流，例如 `scripts/release.py`、`scripts/setup_open_webui.sh`、`scripts/whatsapp-bridge/`。
- 禁止非 allowlist 脚本重新包含 `HERMES_HOME`、`~/.hermes`、`hermes_cli`、`reames_cli`、`NousResearch/hermes-agent`、`Hermes Agent` 等旧运行时/品牌入口。

## 后续建议

下一步应对仓库根目录的旧 Python/Hermes 目录做单独审计，给出三类决策：

1. 迁移为 Go 版 Reames 功能；
2. 移入明确的 `legacy/` 或外部参考位置；
3. 删除并以 `F:\code-reference\Hermes` 作为历史来源。

这一步应独立执行，因为旧 Python 树内部互相引用较多，直接删除风险高于 `scripts/` 顶层清理。
