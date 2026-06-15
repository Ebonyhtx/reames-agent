"""Git development workflow tools."""
import subprocess
import os as _os

GIT_TOOLS = {}

def _run_git(args, cwd=None):
    try:
        r = subprocess.run(['git'] + args, cwd=cwd, capture_output=True, text=True, timeout=60)
        return r.stdout.strip(), r.stderr.strip(), r.returncode
    except FileNotFoundError:
        return "", "git not found.", -1
    except subprocess.TimeoutExpired:
        return "", "git timed out.", -1

def handle_git_clone(args):
    url = args.get("url", "")
    directory = args.get("directory", "")
    branch = args.get("branch", "")
    if not url:
        return "Error: url is required"
    if directory:
        directory = _os.path.basename(directory)  # prevent path traversal
    cmd = ["clone"]
    if branch:
        cmd += ["--branch", branch]
    cmd.append(url)
    if directory:
        cmd.append(directory)
    out, err, code = _run_git(cmd)
    if code == 0:
        return f"Cloned to {directory or _os.path.basename(url)}"
    return f"Clone failed: {err}"

def handle_git_commit(args):
    message = args.get("message", "")
    paths = args.get("paths", [])
    cwd = args.get("cwd", _os.getcwd())
    if isinstance(paths, list) and paths:
        out, err, code = _run_git(["add"] + paths, cwd)
    else:
        out, err, code = _run_git(["add", "-A"], cwd)
    if code != 0:
        return f"Stage failed: {err}"
    out, err, code = _run_git(["commit", "-m", message], cwd)
    if code == 0:
        return f"Committed: {out}"
    return f"Commit failed: {err}"

def handle_git_branch(args):
    action = args.get("action", "list")
    name = args.get("name", "")
    cwd = args.get("cwd", _os.getcwd())
    if action == "list":
        out, err, code = _run_git(["branch"], cwd)
        return f"Branches:\n{out}" if out else err
    elif action == "create":
        out, err, code = _run_git(["branch", name], cwd)
        return f"Created: {name}" if code == 0 else err
    elif action == "switch":
        out, err, code = _run_git(["checkout", name], cwd)
        return f"Switched to: {name}" if code == 0 else err
    elif action == "delete":
        out, err, code = _run_git(["branch", "-d", name], cwd)
        return f"Deleted: {name}" if code == 0 else err
    return f"Unknown action: {action}"

def handle_git_log(args):
    count = args.get("count", 10)
    cwd = args.get("cwd", _os.getcwd())
    out, err, code = _run_git(["log", f"--max-count={count}", "--oneline", "--graph"], cwd)
    if code == 0:
        return f"Log:\n{out}" if out else "No commits"
    return f"Log failed: {err}"

def handle_git_status(args):
    cwd = args.get("cwd", _os.getcwd())
    out, err, code = _run_git(["status", "--short"], cwd)
    if code == 0:
        return f"Status:\n{out}" if out else "Clean"
    return f"Status failed: {err}"

def handle_git_review(args):
    cwd = args.get("cwd", _os.getcwd())
    out, err, code = _run_git(["diff", "--stat"], cwd)
    if code != 0:
        return f"Diff failed: {err}"
    if not out:
        return "No unstaged changes."
    return f"Review:\n{out}"

def _run_gh(args, cwd=None):
    try:
        r = subprocess.run(['gh'] + args, cwd=cwd, capture_output=True, text=True, timeout=60)
        return r.stdout.strip(), r.stderr.strip(), r.returncode
    except FileNotFoundError:
        return "", "gh not found.", -1
    except subprocess.TimeoutExpired:
        return "", "gh timed out.", -1

def handle_git_pr(args):
    action = args.get("action", "list")
    title = args.get("title", "")
    body = args.get("body", "")
    cwd = args.get("cwd", _os.getcwd())
    if action == "list":
        out, err, code = _run_gh(["pr", "list"], cwd)
        return f"PRs:\n{out}" if out else err
    elif action == "create":
        cmd = ["pr", "create", "--title", title]
        if body:
            cmd += ["--body", body]
        out, err, code = _run_gh(cmd, cwd)
        return f"PR created: {out}" if code == 0 else err
    return f"Unknown action: {action}"

# Tool schemas
GIT_CLONE_SCHEMA = {
    "type": "object",
    "properties": {
        "url": {"type": "string", "description": "Git repository URL"},
        "directory": {"type": "string", "description": "Target directory (optional)"},
        "branch": {"type": "string", "description": "Branch to clone (optional)"}
    },
    "required": ["url"]
}

GIT_COMMIT_SCHEMA = {
    "type": "object",
    "properties": {
        "message": {"type": "string", "description": "Commit message"},
        "paths": {"type": "array", "items": {"type": "string"}, "description": "Files to stage"},
        "cwd": {"type": "string", "description": "Working directory"}
    },
    "required": ["message"]
}

GIT_BRANCH_SCHEMA = {
    "type": "object",
    "properties": {
        "action": {"type": "string", "enum": ["list", "create", "switch", "delete"], "description": "Branch action"},
        "name": {"type": "string", "description": "Branch name"},
        "cwd": {"type": "string", "description": "Working directory"}
    },
    "required": ["action"]
}

GIT_LOG_SCHEMA = {
    "type": "object",
    "properties": {
        "count": {"type": "integer", "description": "Number of commits"},
        "cwd": {"type": "string", "description": "Working directory"}
    }
}

GIT_STATUS_SCHEMA = {
    "type": "object",
    "properties": {
        "cwd": {"type": "string", "description": "Working directory"}
    }
}

GIT_REVIEW_SCHEMA = {
    "type": "object",
    "properties": {
        "cwd": {"type": "string", "description": "Working directory"}
    }
}

GIT_PR_SCHEMA = {
    "type": "object",
    "properties": {
        "action": {"type": "string", "enum": ["list", "create"], "description": "PR action"},
        "title": {"type": "string", "description": "PR title"},
        "body": {"type": "string", "description": "PR description"},
        "cwd": {"type": "string", "description": "Working directory"}
    },
    "required": ["action"]
}

# Registry
GIT_TOOLS = {
    "git_clone": {"schema": GIT_CLONE_SCHEMA, "handler": handle_git_clone, "description": "Clone a git repository to local"},
    "git_commit": {"schema": GIT_COMMIT_SCHEMA, "handler": handle_git_commit, "description": "Stage and commit changes"},
    "git_branch": {"schema": GIT_BRANCH_SCHEMA, "handler": handle_git_branch, "description": "List, create, switch, or delete branches"},
    "git_log": {"schema": GIT_LOG_SCHEMA, "handler": handle_git_log, "description": "View commit history"},
    "git_status": {"schema": GIT_STATUS_SCHEMA, "handler": handle_git_status, "description": "Check workspace status"},
    "git_review": {"schema": GIT_REVIEW_SCHEMA, "handler": handle_git_review, "description": "Review unstaged changes"},
    "git_pr": {"schema": GIT_PR_SCHEMA, "handler": handle_git_pr, "description": "List or create pull requests via GitHub CLI"},
}
