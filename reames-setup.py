#!/usr/bin/env python3
"""Reames Agent 安装脚本 — 在新电脑上部署自定义配置。

用法:
  python reames-setup.py              # 自动安装
  python reames-setup.py --dry-run    # 预览不执行
  python reames-setup.py --hermes-dir /path/to/hermes  # 指定路径
"""
import os
import sys
import shutil
import argparse

REQUIRED_FILES = [
    "agent/entropy.py",
    "agent/status_bar.py",
    "agent/mermaid_offload.py",
    "agent/deepseek_cache.py",
    "agent/agent_init.py",
    "agent/conversation_loop.py",
    "agent/memory_manager.py",
    "agent/system_prompt.py",
    "agent/tool_executor.py",
    "agent/context_compressor.py",
    "agent/conversation_compression.py",
]

PLUGIN_DIRS = [
    "plugins/reames_git",
    "plugins/reames_build",
    "plugins/reames_code_analysis",
    "plugins/reames_crawler",
]

HOOK_FILES = [
    "hooks/pre-tool-check.py",
    "hooks/check-empty-shell.py",
    "hooks/run-tests.py",
    "hooks/turn-notifier.py",
]


def find_hermes_dir():
    """Detect Hermes Agent installation directory."""
    candidates = [
        os.environ.get("HERMES_DIR", ""),
        # Windows
        os.path.expanduser("~/AppData/Local/hermes/hermes-agent"),
        # macOS
        "/Applications/Hermes.app/Contents/Resources/hermes-agent",
        # Linux / pip
        "/opt/hermes-agent",
        "/usr/local/hermes-agent",
    ]
    for d in candidates:
        if d and os.path.isdir(d) and os.path.isfile(os.path.join(d, "agent", "conversation_loop.py")):
            return d
    return None


def backup_file(src):
    """Create .bak backup."""
    if os.path.isfile(src):
        bak = src + ".bak"
        if not os.path.exists(bak):
            shutil.copy2(src, bak)
            return True
    return False


def install(args):
    src_dir = os.path.dirname(os.path.abspath(__file__))
    
    if args.hermes_dir:
        hermes_dir = args.hermes_dir
    elif args.auto_detect:
        hermes_dir = find_hermes_dir()
        if not hermes_dir:
            print("ERROR: Could not auto-detect Hermes directory.")
            print("Use --hermes-dir to specify manually.")
            sys.exit(1)
    else:
        # Default: assume CWD is Hermes dir
        hermes_dir = os.getcwd()
    
    if not os.path.isdir(hermes_dir):
        print(f"ERROR: Hermes directory not found: {hermes_dir}")
        sys.exit(1)
    
    print(f"Installing Reames to: {hermes_dir}")
    
    copied = 0
    backed_up = 0
    
    # 1. Agent core files
    for rel_path in REQUIRED_FILES:
        src = os.path.join(src_dir, rel_path)
        dst = os.path.join(hermes_dir, rel_path)
        if not os.path.isfile(src):
            print(f"  WARNING: Source not found: {rel_path}")
            continue
        if args.dry_run:
            print(f"  [dry-run] Would copy: {rel_path}")
            copied += 1
            continue
        if backup_file(dst):
            backed_up += 1
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copy2(src, dst)
        copied += 1
        print(f"  Copy: {rel_path}")
    
    # 2. Plugins
    for rel_path in PLUGIN_DIRS:
        src_dir_path = os.path.join(src_dir, rel_path)
        dst_dir_path = os.path.join(hermes_dir, rel_path)
        if not os.path.isdir(src_dir_path):
            print(f"  WARNING: Plugin not found: {rel_path}")
            continue
        if args.dry_run:
            print(f"  [dry-run] Would copy plugin: {rel_path}")
            copied += 1
            continue
        if os.path.isdir(dst_dir_path):
            backup_file(os.path.join(dst_dir_path, "__init__.py"))
        shutil.copytree(src_dir_path, dst_dir_path, dirs_exist_ok=True)
        copied += 1
        print(f"  Copy plugin: {rel_path}")
    
    # 3. Hooks
    for rel_path in HOOK_FILES:
        src = os.path.join(src_dir, rel_path)
        dst = os.path.join(hermes_dir, rel_path)
        if not os.path.isfile(src):
            print(f"  WARNING: Hook not found: {rel_path}")
            continue
        if args.dry_run:
            print(f"  [dry-run] Would copy: {rel_path}")
            copied += 1
            continue
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copy2(src, dst)
        copied += 1
        print(f"  Copy: {rel_path}")
    
    print(f"\nComplete: {copied} files copied, {backed_up} backups created.")
    
    if not args.dry_run and backed_up > 0:
        print(f"Backups saved as .bak files in {hermes_dir}")
    
    print("\nNext steps:")
    print("  1. Restart Hermes Agent")
    print("  2. Run 'hermes' to verify the status bar shows")


def main():
    parser = argparse.ArgumentParser(description="Reames Agent Installer")
    parser.add_argument("--hermes-dir", help="Hermes Agent installation directory")
    parser.add_argument("--dry-run", action="store_true", help="Preview without copying")
    parser.add_argument("--auto-detect", action="store_true", help="Auto-detect Hermes directory")
    args = parser.parse_args()
    install(args)


if __name__ == "__main__":
    main()
