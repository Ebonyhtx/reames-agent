"""PreToolUse: Check if edit would delete real code."""
import sys, os, json

BYPASS_FILE = os.path.expanduser("~/.reames/.bypass-hooks")
if os.path.exists(BYPASS_FILE):
    sys.exit(0)

payload = json.loads(sys.stdin.read())
tool_name = payload.get("toolName", "")
tool_args = payload.get("toolArgs", {})

file_tools = ["edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol"]
if tool_name not in file_tools:
    sys.exit(0)

file_path = tool_args.get("path", "")
if not file_path or not os.path.exists(file_path):
    sys.exit(0)

try:
    with open(file_path, "r", encoding="utf-8") as f:
        old_content = f.read()
    old_lines = len([l for l in old_content.split("\n") if l.strip()])

    if tool_name == "edit_file":
        old_str = tool_args.get("old_string", "")
        new_str = tool_args.get("new_string", "")
        old_chunk = len([l for l in old_str.split("\n") if l.strip()])
        new_chunk = len([l for l in new_str.split("\n") if l.strip()])
        if old_chunk >= 2 and new_chunk == 0:
            print("ERROR: Deleting code (empty replacement)", file=sys.stderr)
            sys.exit(2)
        if old_chunk >= 3 and new_chunk > 0 and (new_chunk / old_chunk) < 0.4:
            print("ERROR: Shrinking code block significantly", file=sys.stderr)
            sys.exit(2)

    if tool_name == "write_file":
        new_content = tool_args.get("content", "")
        new_lines = len([l for l in new_content.split("\n") if l.strip()])
        if old_lines >= 3 and new_lines > 0 and (new_lines / old_lines) < 0.25:
            print("ERROR: Reducing file significantly", file=sys.stderr)
            sys.exit(2)
except Exception:
    pass
sys.exit(0)
