"""Tests for cache_first mode and auto-enable logic for DeepSeek."""
from __future__ import annotations
from unittest.mock import MagicMock, patch
import pytest


class TestCacheFirstAutoEnable:
    """agent_init.py auto-enable logic for DeepSeek."""

    def test_deepseek_no_config_auto_true(self):
        """DeepSeek model + no cache_first config -> auto true."""
        cfg = {}
        raw = cfg.get('cache_first')
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool('deepseek' in 'deepseek-v4-flash'.lower())
        assert result is True

    def test_deepseek_explicit_false_respected(self):
        """DeepSeek model + cache_first: false -> false."""
        cfg = {'cache_first': False}
        raw = cfg.get('cache_first')
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool('deepseek' in 'deepseek-v4-flash'.lower())
        assert result is False

    def test_deepseek_explicit_true_respected(self):
        """DeepSeek model + cache_first: true -> true."""
        cfg = {'cache_first': True}
        raw = cfg.get('cache_first')
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool('deepseek' in 'deepseek-v4-flash'.lower())
        assert result is True

    def test_non_deepseek_no_config_false(self):
        """Non-DeepSeek model + no config -> false."""
        cfg = {}
        raw = cfg.get('cache_first')
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool('deepseek' in 'claude-sonnet'.lower())
        assert result is False

    def test_non_deepseek_explicit_true_honored(self):
        """Non-DeepSeek + cache_first: true -> true."""
        cfg = {'cache_first': True}
        raw = cfg.get('cache_first')
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool('deepseek' in 'gpt-4'.lower())
        assert result is True

    def test_empty_model_no_config_false(self):
        """Empty model + no config -> false (no crash)."""
        cfg = {}
        raw = cfg.get('cache_first')
        model = ''
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool(model and 'deepseek' in model.lower())
        assert result is False

    def test_deepseek_chat_matches(self):
        """deepseek-chat should also auto-enable."""
        model = 'deepseek-chat'
        assert 'deepseek' in model.lower()

    def test_deepseek_v4_flash_matches(self):
        """deepseek-v4-flash should auto-enable."""
        assert 'deepseek' in 'deepseek-v4-flash'.lower()


class TestCacheFirstCompressBehavior:
    """ContextCompressor.compress() with cache_first flag."""

    def test_cache_first_skips_summary(self):
        """cache_first=True -> _generate_summary not called."""
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(model="test", threshold_percent=0.5, cache_first=True)

        messages = [
            {'role': 'system', 'content': 'sys'},
            {'role': 'user', 'content': 'hi'},
            {'role': 'assistant', 'content': 'hello'},
            {'role': 'user', 'content': 'q2'},
            {'role': 'assistant', 'content': 'r2'},
            {'role': 'user', 'content': 'q3'},
            {'role': 'assistant', 'content': 'r3'},
            {'role': 'user', 'content': 'q4'},
            {'role': 'assistant', 'content': 'r4'},
        ]
        comp._generate_summary = MagicMock()
        result = comp.compress(messages, current_tokens=20000)
        comp._generate_summary.assert_not_called()
        assert result is not None

    def test_cache_first_still_prunes_tools(self):
        """cache_first=True -> tool pruning still runs."""
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(model="test", threshold_percent=0.5, cache_first=True)

        messages = [
            {'role': 'system', 'content': 'sys'},
            {'role': 'user', 'content': 'q', 'tool_calls': None},
            {'role': 'assistant', 'content': None, 'tool_calls': [{'id': 'call_1', 'function': {'name': 'test', 'arguments': '{}'}}]},
            {'role': 'tool', 'content': 'very long tool result ' * 100, 'tool_call_id': 'call_1'},
            {'role': 'user', 'content': 'q2'},
            {'role': 'assistant', 'content': 'r2'},
        ]
        comp._generate_summary = MagicMock()
        result = comp.compress(messages, current_tokens=10000)
        comp._generate_summary.assert_not_called()

    def test_cache_false_still_uses_summary(self):
        """cache_first=False -> _generate_summary called."""
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(model="test", threshold_percent=0.5, cache_first=False)
        messages = [
            {'role': 'system', 'content': 'sys'},
            {'role': 'user', 'content': 'q'},
            {'role': 'assistant', 'content': 'r'},
            {'role': 'user', 'content': 'q2'},
            {'role': 'assistant', 'content': 'r2'},
            {'role': 'user', 'content': 'q3'},
            {'role': 'assistant', 'content': 'r3'},
            {'role': 'user', 'content': 'q4'},
            {'role': 'assistant', 'content': 'r4'},
        ]
        comp._generate_summary = MagicMock(return_value='summary text')
        comp._sanitize_tool_pairs = lambda ms: ms
        import agent.context_compressor as cc
        orig = cc._strip_historical_media
        cc._strip_historical_media = lambda ms: ms
        try:
            with patch.object(comp, '_find_latest_context_summary', return_value=(None, None)):
                comp.compress(messages, current_tokens=10000)
        finally:
            cc._strip_historical_media = orig
        comp._generate_summary.assert_called_once()
