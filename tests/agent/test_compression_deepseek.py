"""Tests for compression mechanism with DeepSeek-specific behavior."""
from __future__ import annotations
from unittest.mock import MagicMock, patch, PropertyMock
import pytest
from agent.context_compressor import ContextCompressor, _strip_historical_media


class TestShouldCompress:
    """should_compress threshold and anti-thrashing."""

    def test_below_threshold_false(self):
        comp = ContextCompressor(model="test", threshold_percent=0.5)
        # threshold_tokens = 256000 * 0.5 = 128000
        comp.last_prompt_tokens = 30000
        assert comp.should_compress(30000) is False

    def test_above_threshold_true(self):
        comp = ContextCompressor(model="test", threshold_percent=0.1)
        # threshold_tokens = 256000 * 0.1 = 25600; 70000 > 25600
        comp.last_prompt_tokens = 70000
        assert comp.should_compress(70000) is True

    def test_anti_thrashing_backoff(self):
        comp = ContextCompressor(model="test", threshold_percent=0.1)
        comp.last_prompt_tokens = 70000
        comp._ineffective_compression_count = 2
        assert comp.should_compress(70000) is False

    def test_anti_thrashing_resets_after_effective(self):
        comp = ContextCompressor(model="test", threshold_percent=0.1)
        comp.last_prompt_tokens = 70000
        comp._ineffective_compression_count = 0
        assert comp.should_compress(70000) is True


class TestSanitizeToolPairs:
    """Orphaned tool_call/tool_result cleanup."""

    def test_remove_orphaned_result(self):
        comp = ContextCompressor(model="test")
        messages = [
            {"role": "assistant", "content": "ok"},
            {"role": "tool", "content": "result", "tool_call_id": "orphan_1"},
        ]
        result = comp._sanitize_tool_pairs(messages)
        assert len(result) == 1
        assert result[0]["role"] == "assistant"

    def test_add_stub_for_missing_result(self):
        comp = ContextCompressor(model="test")
        messages = [
            {"role": "assistant", "content": None, "tool_calls": [{"id": "call_1", "function": {"name": "t", "arguments": "{}"}}]},
        ]
        result = comp._sanitize_tool_pairs(messages)
        assert len(result) == 2
        assert result[1]["role"] == "tool"
        assert result[1]["tool_call_id"] == "call_1"

    def test_well_formed_unchanged(self):
        comp = ContextCompressor(model="test")
        messages = [
            {"role": "assistant", "content": None, "tool_calls": [{"id": "c1", "function": {"name": "t", "arguments": "{}"}}]},
            {"role": "tool", "content": "ok", "tool_call_id": "c1"},
        ]
        result = comp._sanitize_tool_pairs(messages)
        assert len(result) == 2


class TestPruneOldToolResults:
    """Tool result pruning pre-pass."""
    def test_prune_old_results(self):
        comp = ContextCompressor(model="test")
        messages = [
            {"role": "system", "content": "sys"},
            {"role": "user", "content": "q1"},
            {"role": "assistant", "content": None, "tool_calls": [{"id": "c1", "function": {"name": "t", "arguments": "{}"}}]},
            {"role": "tool", "content": "long result x" * 50, "tool_call_id": "c1"},
            {"role": "user", "content": "q2"},
            {"role": "assistant", "content": "r2"},
        ]
        result, count = comp._prune_old_tool_results(messages, protect_tail_count=2)
        assert count >= 0

    def test_empty_messages(self):
        comp = ContextCompressor(model="test")
        result, count = comp._prune_old_tool_results([], protect_tail_count=5)
        assert count == 0
        assert result == []


class TestBoundarySignals:
    """Compression boundary detection smoke tests."""
    def test_protect_head_size_default(self):
        comp = ContextCompressor(model="test", protect_first_n=3)
        messages = [{"role": "system", "content": "s"}, {"role": "user", "content": "u1"}, {"role": "assistant", "content": "a1"}]
        size = comp._protect_head_size(messages)
        assert size >= 1

    def test_find_tail_cut_reasonable(self):
        comp = ContextCompressor(model="test")
        messages = [{"role": "system", "content": "s"}] + [{"role": "user", "content": f"q{i}"} for i in range(10)]
        with patch("agent.context_compressor.estimate_messages_tokens_rough", return_value=100):
            tail = comp._find_tail_cut_by_tokens(messages, 1)
        assert 1 <= tail <= len(messages)


class TestStripHistoricalMedia:
    """_strip_historical_media."""

    def test_empty_messages(self):
        assert _strip_historical_media([]) == []

    def test_no_images_unchanged(self):
        msgs = [{"role": "user", "content": "text"}]
        result = _strip_historical_media(msgs)
        assert result[0]["content"] == "text"

    def test_none_input(self):
        assert _strip_historical_media(None) is None
