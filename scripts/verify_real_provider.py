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
import re
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
MAX_RESPONSE_TOKENS = 10

# Bound provider responses before JSON parsing so this verifier cannot be used
# to exhaust memory with a hostile or misconfigured endpoint.
MAX_RESPONSE_BYTES = 1 << 20


def redact(text: str, secrets: tuple[str, ...] = ()) -> str:
    """Remove credential-shaped values, then truncate evidence text."""
    for secret in secrets:
        if secret:
            text = text.replace(secret, "[REDACTED]")
    text = re.sub(r"(?i)bearer\s+[a-z0-9._~+/=-]+", "Bearer [REDACTED]", text)
    text = re.sub(r"(?i)\bsk-[a-z0-9_-]{8,}\b", "[REDACTED]", text)
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
    secrets: tuple[str, ...] = (),
) -> dict:
    """Construct a redacted, machine-readable evidence record."""
    return {
        "check": "verify_real_provider",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "provider": provider,
        "model": model,
        "outcome": outcome,  # "passed" | "blocked" | "failed"
        "status_code": status_code,
        "error": redact(error, secrets) if error else None,
        "latency_ms": latency_ms,
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
        },
        "snippets": [redact(s, secrets) for s in snippets],
        "redacted": True,
        "note": "Content fields are credential-sanitized and truncated to <=%d chars." % MAX_SNIPPET_LEN,
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
        "max_tokens": MAX_RESPONSE_TOKENS,
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
            raw_body = resp.read(MAX_RESPONSE_BYTES + 1)
            elapsed_ms = int((time.monotonic() - start) * 1000)

            if len(raw_body) > MAX_RESPONSE_BYTES:
                return build_evidence(
                    "deepseek", model, "failed", status,
                    f"HTTP {status}: response exceeded {MAX_RESPONSE_BYTES}-byte limit",
                    elapsed_ms, None, None, [], (api_key,),
                )

            data = json.loads(raw_body.decode("utf-8"))
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
                snippets, (api_key,),
            )
    except urllib.error.HTTPError as exc:
        elapsed_ms = int((time.monotonic() - start) * 1000)
        # Provider error bodies are intentionally omitted: some gateways echo
        # Authorization values or request headers in diagnostic responses.
        return build_evidence(
            "deepseek", model, "failed", exc.code,
            f"HTTP {exc.code}: provider rejected request", elapsed_ms, None, None, [],
            (api_key,),
        )
    except Exception as exc:
        elapsed_ms = int((time.monotonic() - start) * 1000)
        return build_evidence(
            "deepseek", model, "failed", None,
            f"{type(exc).__name__}: provider request failed", elapsed_ms, None, None, [],
            (api_key,),
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
        return 2  # Distinct from both verified success (0) and failed request (1).

    return 0 if evidence["outcome"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
