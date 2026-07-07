"""Stop/on_session_end: Notify user that turn completed."""
import sys, json

payload = json.loads(sys.stdin.read())
event = payload.get("event", "on_session_end")

print("=" * 50)
print("  Turn completed - please review the result.")
print("  Event:", event)
print("  CWD:", payload.get("cwd", "N/A"))
print("=" * 50)
sys.exit(0)
