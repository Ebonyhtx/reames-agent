"""Tests for StatusBar cache hit rate tracking and display."""
from __future__ import annotations
from agent.status_bar import StatusBar


def test_init_defaults():
    sb = StatusBar()
    assert sb.last_cache_hit_pct == 0.0
    assert sb._hit_pct_total == 0.0
    assert sb.turn_count == 0
    assert sb.user_turn_count == 0
    assert sb.last_model == ''


def test_record_api_usage_first_turn():
    sb = StatusBar()
    sb.record_api_usage(prompt_tokens=1000, completion_tokens=500, cache_hit_tokens=300, cost=0.01, model='deepseek-v4-flash')
    assert sb.last_cache_hit_pct == 20.0  # 300/(1000+500) * 100
    assert sb._hit_pct_total == 20.0
    assert sb.turn_count == 1
    assert sb.session_prompt_tokens == 1000
    assert sb.session_cache_hit_tokens == 300


def test_record_api_usage_zero_cache_hit():
    sb = StatusBar()
    sb.record_api_usage(prompt_tokens=100, completion_tokens=50, cache_hit_tokens=0)
    assert sb.last_cache_hit_pct == 0.0


def test_record_api_usage_multiple_turns():
    sb = StatusBar()
    sb.record_api_usage(prompt_tokens=1000, completion_tokens=500, cache_hit_tokens=100)
    sb.record_api_usage(prompt_tokens=2000, completion_tokens=1000, cache_hit_tokens=900)
    assert sb.turn_count == 2
    # avg = (100/1500 + 900/3000) / 2 * 100 = (6.67 + 30.0) / 2 = 18.33
    assert sb._hit_pct_total == pytest.approx(6.67 + 30.0, abs=0.1)
    assert sb.last_cache_hit_pct == pytest.approx(30.0, abs=0.1)


def test_model_updates():
    sb = StatusBar()
    sb.record_api_usage(model='deepseek-v4-flash', prompt_tokens=10, completion_tokens=5)
    assert sb.last_model == 'deepseek-v4-flash'
    sb.record_api_usage(model='', prompt_tokens=10, completion_tokens=5)
    assert sb.last_model == 'deepseek-v4-flash'  # keeps previous


def test_session_cost_accumulates():
    sb = StatusBar()
    sb.record_api_usage(cost=0.0012, prompt_tokens=100, completion_tokens=50)
    sb.record_api_usage(cost=0.0034, prompt_tokens=100, completion_tokens=50)
    assert sb.session_cost == pytest.approx(0.0046, abs=0.0001)


def test_context_usage_pct():
    sb = StatusBar()
    sb.record_api_usage(prompt_tokens=100, completion_tokens=50, context_window=10000, context_used=5000)
    assert sb.context_used_pct == 50


def test_context_usage_zero_window():
    sb = StatusBar()
    sb.record_api_usage(prompt_tokens=100, completion_tokens=50, context_window=0, context_used=5000)
    assert sb.context_used_pct == 0


def test_format_status_line():
    sb = StatusBar()
    sb.record_api_usage(prompt_tokens=1000, completion_tokens=500, cache_hit_tokens=400, cost=0.0012, model='deepseek/deepseek-v4-flash', context_window=65536, context_used=10000)
    sb.set_compression_threshold(50)
    sb.user_turn_count = 3
    line = sb.format_status_line()
    assert 'deepseek-v4-flash' in line
    assert '26.67%' in line  # 400/1500 * 100
    assert '3轮' in line or '3轮' in line
    assert '50%' in line


def test_format_status_line_unknown_model():
    sb = StatusBar()
    line = sb.format_status_line()
    assert 'unknown' in line


def test_format_status_line_zero_turns():
    sb = StatusBar()
    line = sb.format_status_line()
    assert line is not None


def test_ctx_bar_empty():
    sb = StatusBar()
    assert sb._ctx_bar(0) == '[▊░░░░░░░░░] 0%' or '0%' in sb._ctx_bar(0)


def test_ctx_bar_full():
    bar = StatusBar()._ctx_bar(100)
    assert '100%' in bar


def test_fmt_tokens():
    sb = StatusBar()
    assert sb._fmt_tokens(500) == '500'
    assert sb._fmt_tokens(1500) == '1.5K'
    assert sb._fmt_tokens(1500000) == '1.5M'


def test_fmt_cost():
    sb = StatusBar()
    assert '0.000006' in sb._fmt_cost(0.000006)
    assert '0.0100' in sb._fmt_cost(0.01)


def test_set_cache_stats():
    sb = StatusBar()
    from agent.deepseek_cache import CacheStats
    cs = CacheStats()
    sb.set_cache_stats(cs)
    assert sb._cache_stats is cs


def test_record_turn_cache():
    sb = StatusBar()
    from agent.deepseek_cache import CacheStats
    cs = CacheStats()
    sb.set_cache_stats(cs)
    sb.record_turn_cache([{'role': 'user', 'content': 'hi'}])
    assert cs._total == 1


def test_fetch_balance_no_key():
    sb = StatusBar()
    sb.fetch_balance('')
    assert sb.balance == ''


def test_set_balance():
    sb = StatusBar()
    sb.set_balance('¥20.42')
    assert sb.balance == '¥20.42' or '20.42' in sb.balance


def test_set_compression_threshold():
    sb = StatusBar()
    sb.set_compression_threshold(80)
    assert sb.compression_threshold == 80
