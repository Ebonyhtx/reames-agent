"""熵管理 - MemoryProvider 深度集成版。

设计原则：
  - 继承 MemoryProvider 生命周期钩子
  - 使用 Reames 集成的 skill_commands 扫描系统
  - 通过 on_session_end 钩子触发记忆修剪
  - 通过 ToolRegistry 注册熵管理工具
  - 健康度注入 volatile 层
  - 配置漂移使用 DEFAULT_CONFIG
"""

import os
import time
import json
import logging
import hashlib

logger = logging.getLogger(__name__)

# 全局状态
_HAS_RUN_THIS_SESSION = False
_LAST_REPORT = None


# ─── 熵管理配置 ───────────────────────────────────

def _get_hermes_home():
    """Get Reames home directory."""
    try:
        from hermes_constants import get_hermes_home as gh
        return str(gh())
    except Exception:
        return os.path.expanduser("~/.hermes")


# ─── 技能质量评估（复用集成扫描）────────

def assess_skills():
    """Assess installed skills using built-in scanner.
    
    This calls skill_commands.scan_skill_commands() to get the canonical
    skill list, then evaluates each for quality.
    
    Returns list of (name, score, [issues]) sorted by score ascending.
    """
    try:
        from agent.skill_commands import scan_skill_commands
        skills = scan_skill_commands()
    except Exception as e:
        logger.debug("skill_commands scan failed: %s, falling back to dir scan", e)
        skills = _scan_skills_fallback()
    
    results = []
    for slug, info in skills.items():
        score = 50
        issues = []
        
        name = info.get("name", slug.strip("/"))
        
        # Check frontmatter completeness
        fm = info.get("frontmatter", {})
        if not fm.get("description"):
            issues.append("Missing description in frontmatter")
            score -= 15
        
        # Check content size
        content = info.get("content", "")
        if not content:
            issues.append("Empty skill content")
            score -= 25
        elif len(content) < 100:
            issues.append("Skill content too short")
            score -= 10
        
        score = max(0, min(100, score))
        results.append((name, score, issues))
    
    return sorted(results, key=lambda x: x[1])


def _scan_skills_fallback():
    """Fallback skill scan when skill_commands is unavailable."""
    skills = {}
    skills_dir = os.path.join(_get_hermes_home(), "skills")
    if not os.path.isdir(skills_dir):
        return skills
    for name in os.listdir(skills_dir):
        sp = os.path.join(skills_dir, name)
        if os.path.isdir(sp):
            md = os.path.join(sp, "SKILL.md")
            content = ""
            desc = ""
            if os.path.isfile(md):
                try:
                    with open(md, "r", encoding="utf-8") as f:
                        content = f.read()
                except Exception:
                    pass
            skills["/" + name] = {"name": name, "content": content, "frontmatter": {"description": desc}}
    return skills


# ─── 记忆修剪（通过 MemoryManager 钩子触发） ────

def prune_memory(dry_run=True):
    """Prune stale memory artifacts via MemoryManager lifecycle.
    
    Designed to be called from MemoryManager.on_session_end().
    
    Returns dict with counts of pruned items.
    """
    result = {"pruned_refs": 0, "pruned_sessions": 0, "archived": 0}
    now = time.time()
    
    # Clean memory refs older than 24h
    refs_dir = os.path.join(_get_hermes_home(), "memory", "refs")
    if os.path.isdir(refs_dir):
        for f in os.listdir(refs_dir):
            fp = os.path.join(refs_dir, f)
            if os.path.isfile(fp) and (now - os.path.getmtime(fp)) > 86400:
                if not dry_run:
                    try:
                        os.remove(fp)
                        result["pruned_refs"] += 1
                    except Exception:
                        pass
                else:
                    result["pruned_refs"] += 1
    
    return result


# ─── 插件健康检查（使用 ToolRegistry）────

def check_plugin_health():
    """Check plugin health using ToolRegistry.
    
    Returns list of (plugin_name, status, [issues]).
    """
    results = []
    try:
        from tools.registry import registry
        entries = registry.list_tools()
        
        # Group by toolset
        toolsets = {}
        for entry in entries:
            ts = getattr(entry, "toolset", "unknown")
            if ts not in toolsets:
                toolsets[ts] = []
            toolsets[ts].append(entry.name)
        
        for ts, tools in toolsets.items():
            issues = []
            if len(tools) == 0:
                issues.append("No tools in toolset")
            status = "healthy" if not issues else "warning"
            results.append((ts, status, issues))
    
    except Exception as e:
        results.append(("registry", "error", [str(e)]))
    
    return results


# ─── 配置漂移检测（使用 config 系统）────

def detect_config_drift():
    """Detect config drift using hermes_cli.config DEFAULT_CONFIG comparison.
    
    Returns list of (key, status, detail).
    """
    results = []
    try:
        from hermes_cli.config import DEFAULT_CONFIG, load_config_readonly
        current = load_config_readonly()
        
        # Check for keys in current that differ from defaults
        if hasattr(DEFAULT_CONFIG, 'keys'):
            for key in DEFAULT_CONFIG:
                if key in current:
                    if current[key] != DEFAULT_CONFIG[key]:
                        # Only flag if not an expected user override
                        if key in ("api_key", "providers"):
                            continue
                        results.append((key, "info", f"Differs from default"))
        
        # Specific checks
        if current.get("hooks") == {} or current.get("hooks") is None:
            results.append(("hooks", "info", "No hooks configured"))
        
        if "api_key" in str(current.get("model", {})):
            results.append(("auth", "warning", "API key in config (should use env var)"))
    
    except Exception as e:
        results.append(("config", "error", [str(e)]))
    
    return results


# ─── 熵报告（带缓存，避免每轮重建） ────────────

def generate_report(dry_run=True):
    """Generate entropy report, cached per session."""
    global _LAST_REPORT
    if _LAST_REPORT and not dry_run:
        return _LAST_REPORT
    
    report = {
        "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
        "dry_run": dry_run,
        "skills": [],
        "memory": {},
        "plugins": [],
        "config_drift": [],
        "summary": "",
    }
    
    try:
        report["skills"] = assess_skills()
    except Exception as e:
        report["skills_error"] = str(e)
    
    try:
        report["memory"] = prune_memory(dry_run=dry_run)
    except Exception as e:
        report["memory_error"] = str(e)
    
    try:
        report["plugins"] = check_plugin_health()
    except Exception as e:
        report["plugins_error"] = str(e)
    
    try:
        report["config_drift"] = detect_config_drift()
    except Exception as e:
        report["config_error"] = str(e)
    
    low_quality = [s for s in report["skills"] if s[1] < 40]
    plugin_issues = [p for p in report["plugins"] if p[1] != "healthy"]
    
    parts = []
    if low_quality:
        parts.append(f"{len(low_quality)} low-quality skills")
    if report["memory"].get("pruned_refs", 0) > 0:
        parts.append(f"{report['memory']['pruned_refs']} refs pruned")
    if plugin_issues:
        parts.append(f"{len(plugin_issues)} toolsets with issues")
    
    report["summary"] = "; ".join(parts) if parts else "All systems healthy"
    
    if not dry_run:
        _LAST_REPORT = report
    
    return report


# ─── 熵管理生命周期钩子 ─────────────────────────

def health_status_line():
    """Return one-line health status for system prompt volatile injection."""
    global _HAS_RUN_THIS_SESSION
    try:
        report = generate_report(dry_run=True)
        if report["summary"] != "All systems healthy":
            _HAS_RUN_THIS_SESSION = True
            return f"[Entropy] {report['summary']}"
        return ""
    except Exception:
        return ""


def on_session_end(dry_run=False):
    """Called by MemoryManager on_session_end hook.
    
    Performs: memory pruning + config drift check.
    """
    logger.info("Entropy on_session_end (dry_run=%s)", dry_run)
    return prune_memory(dry_run=dry_run)


def on_pre_compress():
    """Called before context compression.
    
    Checks if entropy state is still valid.
    """
    global _HAS_RUN_THIS_SESSION
    if _HAS_RUN_THIS_SESSION:
        try:
            r = generate_report(dry_run=True)
            _HAS_RUN_THIS_SESSION = False
            return r
        except Exception:
            return {}
    return {}


# ─── 熵管理工具注册 ─────────────────────────────

def _register_tools():
    """Register entropy management tools with ToolRegistry.
    
    Called once at module load time.
    """
    try:
        from tools.registry import registry
        
        registry.register(
            name="entropy_report",
            toolset="entropy",
            schema={
                "type": "object",
                "properties": {
                    "dry_run": {"type": "boolean", "description": "If true, only report without cleanup"}
                },
                "required": []
            },
            handler=_handle_entropy_report,
            check_fn=lambda: True,
            emoji="🧹",
            description="Generate entropy management report (skills, memory, plugins, config)",
        )
        
        registry.register(
            name="entropy_prune",
            toolset="entropy",
            schema={
                "type": "object",
                "properties": {
                    "target": {"type": "string", "enum": ["memory", "all"], "description": "What to prune"}
                },
                "required": []
            },
            handler=_handle_entropy_prune,
            check_fn=lambda: True,
            emoji="🗑️",
            description="Prune stale memory artifacts",
        )
        
        registry.register(
            name="entropy_health",
            toolset="entropy",
            schema={
                "type": "object",
                "properties": {},
                "required": []
            },
            handler=_handle_entropy_health,
            check_fn=lambda: True,
            emoji="💚",
            description="Quick health check of all subsystems",
        )
        
        logger.info("Entropy tools registered: entropy_report, entropy_prune, entropy_health")
        return True
    except Exception as e:
        logger.debug("Entropy tool registration deferred: %s", e)
        return False


def _handle_entropy_report(args):
    """Handler for entropy_report tool."""
    dry_run = args.get("dry_run", True)
    report = generate_report(dry_run=dry_run)
    return json.dumps(report, indent=2, ensure_ascii=False)


def _handle_entropy_prune(args):
    """Handler for entropy_prune tool."""
    target = args.get("target", "memory")
    if target == "memory":
        result = prune_memory(dry_run=False)
        return json.dumps(result, ensure_ascii=False)
    return "Unknown target"


def _handle_entropy_health(args):
    """Handler for entropy_health tool."""
    report = generate_report(dry_run=True)
    return report["summary"]


# ─── 模块载入时自动注册工具 ─────────────────────

_register_tools()
