"""Reames-style status bar for CLI/desktop display.

Tracks and formats per-turn statistics:
  - Cache hit rates (current + average)
  - Token usage (session + per-turn)
  - Costs (per-turn + session)
  - Context usage %
  - Session turns
  - Compression threshold
  - Account balance
"""

import logging

logger = logging.getLogger(__name__)

# ANSI colors for terminal output
_CYAN = "\033[36m"
_GREEN = "\033[32m"
_YELLOW = "\033[33m"
_RESET = "\033[0m"
_BOLD = "\033[1m"
_DIM = "\033[2m"


class StatusBar:
    """Tracks and formats per-turn statistics for display."""

    def __init__(self):
        self._cache_stats = None  # injected externally via set_cache_stats()
        
        # Cumulative tracking
        self.session_prompt_tokens = 0
        self.session_completion_tokens = 0
        self.session_cache_hit_tokens = 0
        self.session_cost = 0.0
        self.turn_count = 0
        self.last_turn_prompt_tokens = 0
        self.last_turn_completion_tokens = 0
        self.last_turn_cache_hit_tokens = 0
        self.last_turn_cost = 0.0
        self.last_cache_hit_pct = 0.0
        self.last_model = ""
        self.context_used_pct = 0
        self.compression_threshold = 80
        self.balance = ""
        self._hit_pct_total = 0.0
    
    def record_api_usage(self, prompt_tokens: int, completion_tokens: int, 
                          cache_hit_tokens: int = 0, cost: float = 0.0,
                          model: str = "", context_window: int = 0,
                          context_used: int = 0):
        """Record API usage after each turn."""
        self.turn_count += 1
        self.last_model = model or self.last_model
        
        # Per-turn
        self.last_turn_prompt_tokens = prompt_tokens
        self.last_turn_completion_tokens = completion_tokens
        self.last_turn_cache_hit_tokens = cache_hit_tokens
        self.last_turn_cost = cost
        
        # Cumulative
        self.session_prompt_tokens += prompt_tokens
        self.session_completion_tokens += completion_tokens
        self.session_cache_hit_tokens += cache_hit_tokens
        self.session_cost += cost
        
        # Cache hit %
        total = prompt_tokens + completion_tokens
        if total > 0 and cache_hit_tokens > 0:
            self.last_cache_hit_pct = (cache_hit_tokens / total) * 100
        self._hit_pct_total += self.last_cache_hit_pct
        
        # Context usage %
        if context_window > 0 and context_used > 0:
            self.context_used_pct = int((context_used / context_window) * 100)
    
    def record_turn_cache(self, messages: list):
        """Record cache stats from message stability."""
        if self._cache_stats is not None:
            try:
                self._cache_stats.record_turn(messages)
            except Exception:
                logger.debug("CacheStats record_turn failed")
    
    def set_cache_stats(self, cache_stats):
        self._cache_stats = cache_stats
    
    def fetch_balance(self, api_key: str):
        """Fetch DeepSeek account balance from API."""
        if not api_key:
            return
        try:
            import urllib.request, json
            req = urllib.request.Request(
                "https://api.deepseek.com/user/balance",
                headers={"Authorization": f"Bearer {api_key}", "Accept": "application/json"}
            )
            resp = urllib.request.urlopen(req, timeout=5)
            data = json.loads(resp.read())
            bal = data.get("balance", data.get("total_balance", ""))
            if bal is not None:
                self.balance = f"¥{float(bal):.2f}"
        except Exception:
            pass
    
    def set_balance(self, balance_str: str):
        self.balance = balance_str
    
    def set_compression_threshold(self, threshold: int):
        self.compression_threshold = threshold
    
    def _fmt_tokens(self, n: int) -> str:
        """Format token count in human-readable form."""
        if n >= 1_000_000:
            return f"{n/1_000_000:,.1f}M"
        if n >= 1_000:
            return f"{n/1_000:,.1f}K"
        return str(n)
    
    def _fmt_cost(self, cost: float) -> str:
        """Format cost in human-readable form."""
        if cost < 0.01:
            return f"¥{cost:.6f}"
        return f"¥{cost:.4f}"
    
    def _ctx_bar(self, pct: int) -> str:
        """Visual progress bar for context usage."""
        filled = pct // 10
        bar = "█" * filled + "░" * (10 - filled)
        return f"[{bar}] {pct}%"
    
    def _cache_hit_rate(self) -> float:
        """Get cache hit rate from CacheStats or fallback."""
        if self._cache_stats is not None:
            return self._cache_stats.hit_rate * 100
        return self.last_cache_hit_pct
    
    def format_status_line(self) -> str:
        """Format the status bar line (Reames-style)."""
        model = self.last_model.split("/")[-1] if self.last_model else "unknown"
        hit_rate = self.last_cache_hit_pct
        avg_hit = self._hit_pct_total / self.turn_count if self.turn_count > 0 else hit_rate
        
        parts = [
            f"{_CYAN}{model}{_RESET}",
            f"{_GREEN}{hit_rate:.2f}%{_RESET}",
            f"{_DIM}avg {avg_hit:.2f}%{_RESET}",
            f"{self._fmt_cost(self.last_turn_cost)}",
            f"{self.turn_count} turns",
            f"ctx {self._ctx_bar(self.context_used_pct)}",
            f"{self.compression_threshold}%",
            f"{_BOLD}{self._fmt_cost(self.session_cost)}{_RESET}",
        ]
        
        if self.balance:
            parts.append(self.balance)
        
        return " | ".join(parts)
    
    def print_status_line(self):
        """Print the status bar line."""
        line = self.format_status_line()
        # Use \r to overwrite the line, then \n for newline
        sys_stderr = __import__('sys').stderr
        sys_stderr.write(f"\r{_DIM}{'─'*60}{_RESET}\n")
        sys_stderr.write(f"\r{line}\n")
        sys_stderr.flush()
