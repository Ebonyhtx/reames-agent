"""PostToolUse: Check for empty stubs, TODOs, not implemented."""
import sys, os, json, re

BYPASS_FILE = os.path.expanduser("~/.reames/.bypass-hooks")
if os.path.exists(BYPASS_FILE):
    sys.exit(0)

payload = json.loads(sys.stdin.read())
tool_name = payload.get("toolName", "")
tool_args = payload.get("toolArgs", {})

file_tools = ["edit_file", "write_file", "multi_edit"]
if tool_name not in file_tools:
    sys.exit(0)

file_path = tool_args.get("path", "")
if not file_path or not os.path.exists(file_path):
    sys.exit(0)

try:
    with open(file_path, "r", encoding="utf-8") as f:
        content = f.read()
    lines = content.split("\n")
    non_empty = len([l for l in lines if l.strip()])

    issues = []
    if non_empty < 3:
        issues.append("File has only " + str(non_empty) + " non-empty lines")

    empty_funcs = re.findall(r'\b(function|def|func|fn)\s+\w+\s*\([^)]*\)\s*\{[\s]*\}', content)
    if empty_funcs:
        issues.append(str(len(empty_funcs)) + " empty function body(ies)")

    todo_markers = re.findall(r'\b(TODO|FIXME|HACK|XXX|not\s+implemented)\b', content, re.IGNORECASE)
    if todo_markers:
        issues.append("Contains TODO/FIXME/not implemented markers")

    if re.search(r'throw\s+new\s+Error\s*\(\s*["\']not\s+implemented', content, re.IGNORECASE):
        issues.append("Contains 'not implemented' throw")

    if re.search(r'\b(def|class)\s+\w+\s*[:\(].*:\s*$[\s]*pass\s*$', content, re.MULTILINE):
        issues.append("Contains Python pass-only stubs")

    empty_catches = re.findall(r'catch\s*\([^)]*\)\s*\{[\s]*\}', content)
    if empty_catches:
        issues.append(str(len(empty_catches)) + " empty catch block(s)")

    if issues:
        msg = "Empty shell check: " + "; ".join(issues)
        print(msg, file=sys.stderr)
        sys.exit(2)
except Exception:
    pass
sys.exit(0)
