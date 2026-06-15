"""PostToolUse: Auto-run tests after file edits."""
import sys, os, json, subprocess

BYPASS_FILE = os.path.expanduser("~/.hermes/.bypass-hooks")
if os.path.exists(BYPASS_FILE):
    sys.exit(0)

payload = json.loads(sys.stdin.read())
tool_name = payload.get("toolName", "")
cwd = payload.get("cwd", os.getcwd())

file_tools = ["edit_file", "write_file", "multi_edit", "delete_range"]
if tool_name not in file_tools:
    sys.exit(0)

def detect_test_command(dir_path):
    for _ in range(5):
        files = os.listdir(dir_path)
        if "go.mod" in files:
            has_tests = any(f.endswith("_test.go") for f in files)
            if has_tests:
                return ["go", "test", "./..."], 60
        if "package.json" in files:
            scripts = ""
            try:
                with open(os.path.join(dir_path, "package.json"), "r") as f:
                    scripts = f.read()
            except:
                pass
            if '"test"' in scripts or os.path.exists(os.path.join(dir_path, "node_modules")):
                return ["npm", "test"], 60
        if "pytest.ini" in files or "pyproject.toml" in files:
            return ["python", "-m", "pytest"], 60
        parent = os.path.dirname(dir_path)
        if parent == dir_path:
            break
        dir_path = parent
    return None, 0

cmd_info = detect_test_command(cwd)
if cmd_info[0] is None:
    sys.exit(0)

cmd, timeout = cmd_info
try:
    r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)
    if r.returncode != 0:
        print("Tests failed:", file=sys.stderr)
        print(r.stdout[-500:], file=sys.stderr)
        print(r.stderr[-500:], file=sys.stderr)
        sys.exit(2)
except FileNotFoundError:
    pass
except Exception as e:
    print("Test error:", str(e), file=sys.stderr)
    sys.exit(2)
sys.exit(0)
