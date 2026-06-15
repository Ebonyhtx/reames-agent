"""Code analysis tools."""
import os
import re
import subprocess

def _count_lines(files):
    total = 0
    for f in files:
        try:
            with open(f, 'r') as fh:
                total += len(fh.readlines())
        except:
            pass
    return total

def handle_complexity(args):
    """Analyze code complexity using lizard or basic metrics."""
    path = args.get("path", os.getcwd())
    try:
        r = subprocess.run(["lizard", path, "-l", "python,go,javascript,typescript,rust", "-w"], 
                         capture_output=True, text=True, timeout=30)
        return r.stdout[:2000] if r.stdout else "lizard not installed. Try: pip install lizard"
    except:
        return "lizard not available for complexity analysis."

def handle_dependencies(args):
    """Analyze project dependencies."""
    path = args.get("path", os.getcwd())
    results = []
    for root, dirs, files in os.walk(path):
        dirs[:] = [d for d in dirs if not d.startswith('.') and d != 'node_modules' and d != 'venv' and d != '.git']
        for f in files:
            fp = os.path.join(root, f)
            try:
                with open(fp, 'r') as fh:
                    for line in fh:
                        if re.match(r'^(import |from |require\()', line.strip()):
                            results.append(f"{fp}: {line.strip()[:80]}")
            except:
                pass
    return "Dependencies:\n" + "\n".join(results[:50]) if results else "No dependencies found."

CODE_TOOLS = {
    "analyze_complexity": {"handler": handle_complexity, "description": "Analyze code complexity"},
    "analyze_dependencies": {"handler": handle_dependencies, "description": "Analyze project dependencies"},
}
