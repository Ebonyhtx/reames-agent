#!/usr/bin/env python3
"""Credential-gated provider verification script.

Verifies that a configured LLM provider can establish a real API
connection WITHOUT writing secrets to files, logs, or command-line
arguments. The script reads credentials from the process environment
only (no .env files, no config.toml, no CLI flags).

Output is a machine-readable JSON evidence record with content
redacted so it can be stored in CI artifacts or audit trails.

Usage:
  set DEEPSEEK_API_KEY=sk-...
  python scripts/verify_real_provider.py --provider deepseek
  python scripts/verify_real_provider.py --provider deepseek --out evidence.json

The script NEVER:
- Reads .env files, config.toml, or credential files
- Writes secrets to stdout, stderr, or output files
- Prints API key values or full response bodies
- Accepts API keys via command-line flags
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.request
import urllib.error
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

# Maximum length of any content snippet included in evidence output.
MAX_SNIPPET_LEN = 80

# Max total response tokens allowed in evidence.
MAX_RESPONSE_TOKENS = 50


def redact(text: str) -> str:
    """Truncate text to a short, non-sensitive snippet for evidence."""
    if len(text) > MAX_SNIPPET_LEN:
        return text[:MAX_SNIPPET_LEN] + "..."
    return text


def build_evidence(
    provider: str,
    model: str,
    outcome: str,
    status_code: int | None,
    error: str | None,
    latency_ms: int,
    prompt_tokens: int | None,
    completion_tokens: int | None,
    snippets: list[str],
) -> dict:
    """Construct a redacted, machine-readable evidence record."""
    return {
        "check": "verify_real_provider",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "provider": provider,
        "model": model,
        "outcome": outcome,  # "passed" | "blocked" | "failed"
        "status_code": status_code,
        "error": redact(error) if error else None,
        "latency_ms": latency_ms,
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
        },
        "snippets": [redact(s) for s in snippets],
        "redacted": True,
        "note": "All content fields are truncated to <=%d chars. Secrets are never included." % MAX_SNIPPET_LEN,
    }


def verify_deepseek(model: str) -> dict:
    """Verify DeepSeek API connectivity using only process env vars."""
    api_key = os.environ.get("DEEPSEEK_API_KEY", "")
    if not api_key:
        return build_evidence(
            "deepseek", model, "blocked", None,
            "DEEPSEEK_API_KEY not set in process environment", 0, None, None, []
        )

    base_url = os.environ.get("DEEPSEEK_BASE_URL", "https://api.deepseek.com")
    url = f"{base_url.rstrip('/')}/chat/completions"

    payload = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": "Say 'OK' and nothing else."}],
        "max_tokens": 10,
        "temperature": 0,
        "stream": False,
    }).encode("utf-8")

    req = urllib.request.Request(
        url,
        data=payload,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
    )

    start = time.monotonic()
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            status = resp.status
            body = resp.read().decode("utf-8")
            elapsed_ms = int((time.monotonic() - start) * 1000)

            data = json.loads(body)
            choices = data.get("choices", [])
            usage = data.get("usage", {})

            snippets = []
            for choice in choices[:1]:
                content = choice.get("message", {}).get("content", "")
                if content:
                    snippets.append(content.strip())

            return build_evidence(
                "deepseek", model, "passed", status, None, elapsed_ms,
                usage.get("prompt_tokens"), usage.get("completion_tokens"),
                snippets,
            )
    except urllib.error.HTTPError as exc:
        elapsed_ms = int((time.monotonic() - start) * 1000)
        body = ""
        try:
            body = exc.read().decode("utf-8")
        except Exception:
            pass
        return build_evidence(
            "deepseek", model, "failed", exc.code,
            f"HTTP {exc.code}: {body}", elapsed_ms, None, None, []
        )
    except Exception as exc:
        elapsed_ms = int((time.monotonic() - start) * 1000)
        return build_evidence(
            "deepseek", model, "failed", None,
            str(exc), elapsed_ms, None, None, []
        )


PROVIDERS = {
    "deepseek": verify_deepseek,
}


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Credential-gated provider verification (reads keys from env only)"
    )
    parser.add_argument(
        "--provider", required=True,
        choices=list(PROVIDERS.keys()),
        help="Provider to verify"
    )
    parser.add_argument(
        "--model", default="deepseek-chat",
        help="Model name (default: deepseek-chat)"
    )
    parser.add_argument(
        "--out",
        help="Write JSON evidence to this file"
    )
    args = parser.parse_args()

    verifier = PROVIDERS[args.provider]
    evidence = verifier(args.model)

    if args.out:
        out_path = Path(args.out)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
        print(f"Evidence written to {args.out}")

    print(json.dumps(evidence, indent=2))

    if evidence["outcome"] == "blocked":
        print("\nBLOCKED: set DEEPSEEK_API_KEY in your process environment and retry.")
        print("Example:  set DEEPSEEK_API_KEY=sk-...")
        print("          python scripts/verify_real_provider.py --provider deepseek")
        return 0  # "blocked" is not an error — it means external resource needed

    return 0 if evidence["outcome"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
