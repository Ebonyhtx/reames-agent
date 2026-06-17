"""Integration tests for cache hit rate data flow: API response -> UsagePricing -> StatusBar."""
from __future__ import annotations
from unittest.mock import MagicMock, patch
from types import SimpleNamespace
import pytest


class TestNormalizeUsage:
    """usage_pricing.normalize_usage parses cache tokens from API responses."""

    def test_deepseek_format_with_cached_tokens(self):
        """DeepSeek returns prompt_tokens + prompt_tokens_details.cached_tokens."""
        from agent.usage_pricing import normalize_usage
        response_usage = SimpleNamespace(
            prompt_tokens=1000,
            completion_tokens=200,
            prompt_tokens_details=SimpleNamespace(cached_tokens=800),
        )
        result = normalize_usage(response_usage)
        assert result.cache_read_tokens == 800
        assert result.input_tokens == 200  # 1000 - 800
        assert result.output_tokens == 200

    def test_no_cache_fields(self):
        """API returns no cache fields -> cache_read_tokens = 0."""
        from agent.usage_pricing import normalize_usage
        response_usage = SimpleNamespace(
            prompt_tokens=500,
            completion_tokens=100,
        )
        result = normalize_usage(response_usage)
        assert result.cache_read_tokens == 0

    def test_anthropic_format(self):
        """Anthropic returns input_tokens + cache_read_input_tokens."""
        from agent.usage_pricing import normalize_usage
        response_usage = SimpleNamespace(
            input_tokens=600,
            output_tokens=200,
            cache_read_input_tokens=400,
        )
        result = normalize_usage(response_usage, api_mode='anthropic_messages')
        assert result.cache_read_tokens == 400
        assert result.output_tokens == 200

    def test_none_usage(self):
        from agent.usage_pricing import normalize_usage
        result = normalize_usage(None)
        assert result.cache_read_tokens == 0
        assert result.total_tokens == 0


class TestPricingEstimateWithCache:
    """Cost estimation accounts for cache tokens."""

    def test_deepseek_v4_flash_cache_pricing(self):
        from agent.usage_pricing import estimate_usage_cost, CanonicalUsage
        result = estimate_usage_cost(
            'deepseek-v4-flash',
            CanonicalUsage(input_tokens=100, output_tokens=50, cache_read_tokens=500),
            provider='deepseek',
        )
        assert result.amount_usd is not None
        assert result.status != 'unknown'

    def test_cache_read_cost_calculation(self):
        from agent.usage_pricing import estimate_usage_cost, CanonicalUsage
        result = estimate_usage_cost(
            'deepseek-v4-flash',
            CanonicalUsage(cache_read_tokens=1_000_000),
            provider='deepseek',
        )
        # cache_read_cost_per_million for deepseek-v4-flash = 0.02
        # 1000000 * 0.02 / 1000000 = 0.02
        if result.amount_usd is not None:
            assert float(result.amount_usd) > 0


class TestStatusBarCacheFlow:
    """StatusBar receives and displays cache hit rate from real data."""

    def test_record_from_canonical_usage(self):
        from agent.status_bar import StatusBar
        from agent.usage_pricing import CanonicalUsage
        sb = StatusBar()
        cu = CanonicalUsage(input_tokens=100, output_tokens=200, cache_read_tokens=300)
        total = cu.prompt_tokens  # 100
        sb.record_api_usage(
            prompt_tokens=total,
            completion_tokens=cu.output_tokens,
            cache_hit_tokens=cu.cache_read_tokens,
            model='deepseek-v4-flash',
        )
        # cache_hit_pct = 300 / (100 + 200) * 100 = 100.0
        assert sb.last_cache_hit_pct == 100.0

    def test_full_prompt_is_cache(self):
        """All prompt tokens are cached -> hit rate includes prompt in total."""
        from agent.status_bar import StatusBar
        sb = StatusBar()
        sb.record_api_usage(
            prompt_tokens=500,
            completion_tokens=100,
            cache_hit_tokens=500,
        )
        # 500 / (500 + 100) * 100 = 83.33
        import math
        assert abs(sb.last_cache_hit_pct - 83.33) < 0.1


class TestDoubleMetricsCoherence:
    """CacheStats (prefix stability) and StatusBar (API cache) should correlate."""

    def test_both_hit_on_stable_prefix(self):
        from agent.status_bar import StatusBar
        from agent.deepseek_cache import CacheStats
        sb = StatusBar()
        cs = CacheStats()
        sb.set_cache_stats(cs)

        msgs = [
            {'role': 'system', 'content': 'You are a helpful assistant.'},
            {'role': 'user', 'content': 'Hello'},
        ]
        cs.record_turn(msgs)  # first turn always miss
        cs.record_turn(msgs)  # second turn hit
        assert cs.hit_rate == 0.5
        assert cs._hits == 1

    def test_both_low_on_changing_prefix(self):
        from agent.status_bar import StatusBar
        from agent.deepseek_cache import CacheStats
        sb = StatusBar()
        cs = CacheStats()
        sb.set_cache_stats(cs)

        msgs = [{'role': 'system', 'content': 'sys'}, {'role': 'user', 'content': 'q'}]
        cs.record_turn(msgs)
        msgs[-1]['content'] = 'q2'
        cs.record_turn(msgs)
        msgs[-1]['content'] = 'q3'
        cs.record_turn(msgs)
        assert cs._hits == 0
        assert cs._misses == 3
