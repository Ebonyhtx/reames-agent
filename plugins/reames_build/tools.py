"""Build system tools — auto-detect project type, build, test, lint, clean."""
import subprocess
import os

def _detect_project_type(cwd):
    """Auto-detect project type by checking for config files."""
    markers = {
        "go": ["go.mod"],
        "node": ["package.json"],
        "rust": ["Cargo.toml"],
        "python": ["pyproject.toml", "setup.py", "requirements.txt"],
        "make": ["Makefile", "makefile"],
        "cmake": ["CMakeLists.txt"],
    }
    for ptype, files in markers.items():
        for f in files:
            if os.path.exists(os.path.join(cwd, f)):
                return ptype
    return "unknown"

def _get_build_command(ptype):
    commands = {
        "go": {"build": "go build ./...", "test": "go test ./...", "lint": "golint ./...", "clean": "go clean"},
        "node": {"build": "npm run build", "test": "npm test", "lint": "npx eslint .", "clean": "rm -rf node_modules dist"},
        "rust": {"build": "cargo build", "test": "cargo test", "lint": "cargo clippy", "clean": "cargo clean"},
        "python": {"build": "python -m build", "test": "python -m pytest", "lint": "ruff check .", "clean": "rm -rf dist *.egg-info"},
        "make": {"build": "make", "test": "make test", "lint": "", "clean": "make clean"},
        "cmake": {"build": "cmake --build .", "test": "ctest", "lint": "", "clean": "cmake --build . --target clean"},
    }
    return commands.get(ptype, {})

def _run(cmd_str, cwd, timeout=120):
    """Run a command string safely without shell=True."""
    import shlex
    try:
        cmd_list = shlex.split(cmd_str)
        r = subprocess.run(cmd_list, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        return r.stdout + r.stderr, r.returncode
    except Exception as e:
        return str(e), -1

def handle_build(args):
    cwd = args.get("cwd", os.getcwd())
    ptype = _detect_project_type(cwd)
    cmds = _get_build_command(ptype)
    if not cmds.get("build"):
        return f"No build command configured for project type: {ptype}"
    output, code = _run(cmds["build"], cwd)
    status = "succeeded" if code == 0 else f"failed (code {code})"
    return f"[{ptype}] Build {status}:\n{output[:2000]}"

def handle_test(args):
    cwd = args.get("cwd", os.getcwd())
    ptype = _detect_project_type(cwd)
    cmds = _get_build_command(ptype)
    if not cmds.get("test"):
        return f"No test command configured for project type: {ptype}"
    output, code = _run(cmds["test"], cwd)
    status = "passed" if code == 0 else f"failed (code {code})"
    return f"[{ptype}] Tests {status}:\n{output[:2000]}"

def handle_lint(args):
    cwd = args.get("cwd", os.getcwd())
    ptype = _detect_project_type(cwd)
    cmds = _get_build_command(ptype)
    if not cmds.get("lint"):
        return f"No lint command for {ptype}. Skipping."
    output, code = _run(cmds["lint"], cwd)
    return f"[{ptype}] Lint results:\n{output[:2000]}"

def handle_clean(args):
    cwd = args.get("cwd", os.getcwd())
    ptype = _detect_project_type(cwd)
    cmds = _get_build_command(ptype)
    if not cmds.get("clean"):
        return f"No clean command for {ptype}. Skipping."
    output, code = _run(cmds["clean"], cwd)
    return f"[{ptype}] Clean completed.\n{output[:1000]}"

BUILD_TOOLS = {
    "build_project": {"handler": handle_build, "description": "Build the current project"},
    "test_project": {"handler": handle_test, "description": "Run tests for the current project"},
    "lint_project": {"handler": handle_lint, "description": "Lint the current project"},
    "clean_project": {"handler": handle_clean, "description": "Clean build artifacts"},
}
