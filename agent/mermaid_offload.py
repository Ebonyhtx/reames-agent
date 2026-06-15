__all__ = ["maybe_offload", "should_offload", "offload_notification", "MERMAID_OFFLOAD_THRESHOLD"]
"""Mermaid short-term memory offload (Reames Agent)."""
import os
import time
import tempfile

MERMAID_OFFLOAD_THRESHOLD = 5000
_REF_DIR = None
_REF_RETENTION_SECONDS = 86400  # 24 hours
_HAS_OFFLOADED = False  # Track whether offload happened this session


def _cleanup_old_refs(ref_dir):
    """Clean up ref files older than retention period."""
    now = time.time()
    cutoff = now - _REF_RETENTION_SECONDS
    try:
        for f in os.listdir(ref_dir):
            fp = os.path.join(ref_dir, f)
            if os.path.isfile(fp) and os.path.getmtime(fp) < cutoff:
                os.remove(fp)
    except Exception:
        logger.debug("Failed to cleanup old refs")


def _get_ref_dir():
    global _REF_DIR
    if _REF_DIR is None:
        try:
            from hermes_constants import get_hermes_home
            _REF_DIR = str(get_hermes_home()) + '/memory/refs'
        except Exception:
            _REF_DIR = tempfile.gettempdir() + '/hermes-refs'
        os.makedirs(_REF_DIR, exist_ok=True)
        _cleanup_old_refs(_REF_DIR)
    return _REF_DIR


def maybe_offload(content, tool_name, tool_use_id):
    """If content is large, save to ref file and return Mermaid summary."""
    global _HAS_OFFLOADED
    _HAS_OFFLOADED = True
    if len(content) < MERMAID_OFFLOAD_THRESHOLD:
        return content
    
    ref_dir = _get_ref_dir()
    safe_name = os.path.basename(tool_use_id)  # prevent path traversal
    ref_file = os.path.join(ref_dir, safe_name + '.md')
    
    # O_EXCL prevents race conditions from concurrent tool calls
    try:
        fd = os.open(ref_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL)
        with os.fdopen(fd, 'w', encoding='utf-8') as f:
            f.write('# Tool Output: ' + tool_name + '\n\n')
            f.write(content)
    except FileExistsError:
        # Ref file already exists from another call - content is identical
        pass
    except Exception:
        return content
    
    node_id = tool_use_id[:12]
    size_kb = len(content) // 1024
    
    mermaid = 'graph LR\n'
    mermaid += '    O[\u2699 ' + tool_name + ']\n'
    mermaid += '    O -->|output| F["[ref: ' + node_id + ']"]\n'
    
    tag = '<mermaid-offload>\n'
    tag += 'Tool output was ' + str(size_kb) + ' KB. Symbolic summary:\n'
    tag += '```mermaid\n'
    tag += mermaid
    tag += '```\n'
    tag += 'Full output: ' + ref_file + '\n'
    tag += '(use read_file to access)\n'
    tag += '</mermaid-offload>'
    return tag


def offload_notification() -> str:
    """Return system prompt snippet if offload has occurred."""
    if _HAS_OFFLOADED:
        return "Note: Some large tool outputs have been offloaded. Use read_file to access them."
    return ""

def should_offload(content):
    return len(content) > MERMAID_OFFLOAD_THRESHOLD
