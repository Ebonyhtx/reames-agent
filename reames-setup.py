"""Reames Agent 安装脚本 — 从仓库根目录一键部署。

用法:
  python reames-setup.py              # 自动安装
  python reames-setup.py --dry-run    # 预览不执行
"""
import os
import sys
import shutil
import argparse


def find_hermes_dir():
    """The repo root is where this script lives."""
    return os.path.dirname(os.path.abspath(__file__))


def install(args):
    repo_dir = os.path.dirname(os.path.abspath(__file__))
    print(f"Reames Agent 仓库: {repo_dir}")

    if not os.path.isfile(os.path.join(repo_dir, 'agent', 'conversation_loop.py')):
        print("ERROR: 不是 Reames 仓库根目录")
        sys.exit(1)

    print()
    print("克隆完成！接下来：")
    print("  1. cd", repo_dir)
    print("  2. python -m venv .venv")
    print("  3. .venv\\Scripts\\activate  # Windows")
    print("  4. pip install -e .")
    print("  5. hermes")
    print()
    print("或者直接运行:")
    print("  cd", repo_dir, "&& python -m venv .venv && .venv\\Scripts\\activate && pip install -e . && hermes")


def main():
    parser = argparse.ArgumentParser(description="Reames Agent Installer")
    parser.add_argument("--dry-run", action="store_true", help="Preview without copying")
    args = parser.parse_args()
    install(args)


if __name__ == "__main__":
    main()
