"""Message normalization for stable prefix caching (provider-agnostic)."""

import json
import logging

logger = logging.getLogger(__name__)


def deterministic_json(obj) -> str:
    """Serialize JSON with deterministic key ordering."""
    return json.dumps(obj, sort_keys=True, ensure_ascii=False, default=str)


def ensure_cache_stable(api_messages: list) -> list:
    """Normalize messages for byte-stable prefix caching.

    Preserves ALL fields - only sorts keys and strips content whitespace.
    Compatible with Anthropic cache_control, DeepSeek reasoning_content, etc.
    """
    if not api_messages:
        return api_messages

    result = []
    for msg in api_messages:
        if not isinstance(msg, dict):
            result.append(msg)
            continue
        
        stable = {}
        for key in sorted(msg.keys()):
            val = msg[key]
            if key == "content" and isinstance(val, str):
                stable[key] = val.strip()
            else:
                stable[key] = val
        result.append(stable)

    return result


class CacheStats:
    """Track prefix-cache stability across a session."""
    
    def __init__(self):
        self._last_serialized = None
        self._hits = 0
        self._misses = 0
        self._total = 0
    
    def record_turn(self, messages: list) -> bool:
        """Record a turn and return whether prefix is cache-stable.

        The prefix is messages[0:-1] (all except the latest user message).
        A hit means the prefix hasn't changed since last turn.
        """
        prefix = messages[:-1] if len(messages) > 1 else messages
        serialized = deterministic_json(prefix)
        
        is_hit = (serialized == self._last_serialized) if self._last_serialized else False
        
        self._total += 1
        if is_hit:
            self._hits += 1
        else:
            self._misses += 1
        
        self._last_serialized = serialized
        return is_hit
    
    @property
    def hit_rate(self) -> float:
        return self._hits / self._total if self._total > 0 else 0.0
    
    def report(self) -> str:
        pct = self.hit_rate * 100
        return f"Prefix cache: {self._hits}/{self._total} hits ({pct:.1f}%)"
