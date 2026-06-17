"""
Memory provider stubs.

Hermes originally shipped 9 memory plugins (honcho, mem0, etc.) under this
package. Reames replaced them with the native ReamesMemory engine
(agent/reames_memory.py). These stubs exist so Hermes skeleton code that
imports from plugins.memory doesn't crash at startup.
"""

from __future__ import annotations
from typing import Any


def find_provider_dir(name: str) -> None:
    """Stub: all external providers have been removed."""
    return None


def discover_memory_providers() -> list:
    """Stub: no external providers available."""
    return []


def load_memory_provider(name: str) -> None:
    """Stub: all external providers have been removed."""
    return None


def discover_plugin_cli_commands() -> list:
    """Stub: no plugin CLI commands."""
    return []
