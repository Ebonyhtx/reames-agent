"""Tests for compression mechanism with DeepSeek-specific behavior."""
from __future__ import annotations
from unittest.mock import MagicMock, patch, PropertyMock
import pytest


class TestShouldCompress:
    """should_compress threshold and anti-thrashing."""

    def test_below_threshold_false(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5)
        comp.last_prompt_tokens = 30000  # below 64000 threshold
        assert comp.should_compress(30000) is False

    def test_above_threshold_true(self):
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5)
        comp.last_prompt_tokens = 70000
        assert comp.should_compress(70000) is True

    def test_anti_thrashing_backoff(self):
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5)
        comp.last_prompt_tokens = 70000
        comp._ineffective_compression_count = 2
        assert comp.should_compress(70000) is False

    def test_anti_thrashing_resets_after_effective(self):
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5)
        comp.last_prompt_tokens = 70000
        comp._ineffective_compression_count = 0
        assert comp.should_compress(70000) is True


class TestSanitizeToolPairs:
    """Orphaned tool_call/tool_result cleanup."""

    def test_remove_orphaned_result(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000)
        messages = [
            {'role': 'assistant', 'content': 'ok'},
            {'role': 'tool', 'content': 'result', 'tool_call_id': 'orphan_1'},
        ]
        result = comp._sanitize_tool_pairs(messages)
        # orphaned result removed
        assert len(result) == 1
        assert result[0]['role'] == 'assistant'

    def test_add_stub_for_missing_result(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000)
        messages = [
            {'role': 'assistant', 'content': None, 'tool_calls': [{'id': 'call_1', 'function': {'name': 't', 'arguments': '{}'}}]},
        ]
        result = comp._sanitize_tool_pairs(messages)
        assert len(result) == 2
        assert result[1]['role'] == 'tool'
        assert result[1]['tool_call_id'] == 'call_1'

    def test_well_formed_unchanged(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000)
        messages = [
            {'role': 'assistant', 'content': None, 'tool_calls': [{'id': 'c1', 'function': {'name': 't', 'arguments': '{}'}}]},
            {'role': 'tool', 'content': 'ok', 'tool_call_id': 'c1'},
        ]
        result = comp._sanitize_tool_pairs(messages)
        assert len(result) == 2


class TestPruneOldToolResults:
    """Tool result pruning pre-pass."""

    def test_prune_old_results(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5)
        messages = [
            {'role': 'system', 'content': 'sys'},
            {'role': 'user', 'content': 'q1'},
            {'role': 'assistant', 'content': None, 'tool_calls': [{'id': 'c1', 'function': {'name': 't', 'arguments': '{}'}}]},
            {'role': 'tool', 'content': 'long result ' * 100, 'tool_call_id': 'c1'},
            {'role': 'user', 'content': 'q2'},
            {'role': 'assistant', 'content': 'r2'},
        ]
        result, count = comp._prune_old_tool_results(messages, protect_tail_count=2)
        assert count > 0

    def test_empty_messages(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000)
        result, count = comp._prune_old_tool_results([], protect_tail_count=5)
        assert count == 0
        assert result == []


class TestBoundaryDetection:
    """Compression boundary calculation."""

    def test_protect_head_size(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000, protect_first_n=3)
        messages = [{'role': 'system', 'content': 's'}, {'role': 'user', 'content': 'u1'}, {'role': 'assistant', 'content': 'a1'}, {'role': 'user', 'content': 'u2'}, {'role': 'assistant', 'content': 'a2'}]
        size = comp._protect_head_size(messages)
        assert size >= 3

    def test_align_boundary_forward_from_system(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000)
        messages = [{'role': 'system', 'content': 's'}, {'role': 'user', 'content': 'u'}, {'role': 'assistant', 'content': 'a'}]
        idx = comp._align_boundary_forward(messages, 0)
        assert idx >= 1  # skip past system

    def test_find_tail_cut_not_exceed_total(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000)
        messages = [{'role': 'system', 'content': 's'}] + [{'role': 'user', 'content': f'q{i}'} for i in range(20)]
        tail = comp._find_tail_cut_by_tokens(messages, 1)
        assert tail <= len(messages)


class TestStripHistoricalMedia:
    """_strip_historical_media."""

    def test_empty_messages(self):
        from agent.context_compressor import _strip_historical_media
        assert _strip_historical_media([]) == []

    def test_no_images_unchanged(self):
        from agent.context_compressor import _strip_historical_media
        msgs = [{'role': 'user', 'content': 'text'}]
        result = _strip_historical_media(msgs)
        assert result[0]['content'] == 'text'

    def test_none_messages(self):
        from agent.context_compressor import _strip_historical_media
        assert _strip_historical_media(None) is None
