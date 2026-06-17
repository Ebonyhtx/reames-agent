"""Tests for CacheStats prefix-cache stability tracker and deterministic JSON."""

from __future__ import annotations
from agent.deepseek_cache import CacheStats, deterministic_json, ensure_cache_stable


def test_deterministic_json_sorts_keys():
    obj = {'z': 1, 'a': 2, 'm': 3}
    result = deterministic_json(obj)
    assert result == '{"a": 2, "m": 3, "z": 1}'

def test_ensure_cache_stable_empty():
    assert ensure_cache_stable([]) == []
    assert ensure_cache_stable(None) is None

def test_ensure_cache_stable_sorts_keys():
    msgs = [{'z': 1, 'a': 2}]
    result = ensure_cache_stable(msgs)
    assert list(result[0].keys()) == ['a', 'z']

def test_ensure_cache_stable_strips_content():
    msgs = [{'role': 'user', 'content': '  hello  '}]
    result = ensure_cache_stable(msgs)
    assert result[0]['content'] == 'hello'

def test_cache_stats_init():
    cs = CacheStats()
    assert cs._hits == 0 and cs._misses == 0 and cs._total == 0
    assert cs.hit_rate == 0.0

def test_cache_stats_first_turn_miss():
    cs = CacheStats()
    r = cs.record_turn([{'role': 'user', 'content': 'hi'}])
    assert r is False
    assert cs._hits == 0 and cs._misses == 1

def test_cache_stats_second_turn_hit():
    cs = CacheStats()
    msgs = [{'role': 'system', 'content': 'sys'}, {'role': 'user', 'content': 'hello'}]
    cs.record_turn(msgs)
    r = cs.record_turn(msgs)
    assert r is True
    assert cs._hits == 1 and cs._misses == 1
    assert cs.hit_rate == 0.5

def test_cache_stats_changed_prefix_miss():
    cs = CacheStats()
    msgs = [{'role': 'system', 'content': 'sys'}, {'role': 'user', 'content': 'hello'}]
    cs.record_turn(msgs)
    msgs[-1]['content'] = 'world'
    r = cs.record_turn(msgs)
    assert r is False
    assert cs._hits == 0 and cs._misses == 2

def test_cache_stats_sequence():
    cs = CacheStats()
    sys_msg = {'role': 'system', 'content': 'sys'}
    msgs = [sys_msg, {'role': 'user', 'content': 'q'}]
    cs.record_turn(msgs)  # miss
    msgs[-1]['content'] = 'q2'
    cs.record_turn(msgs)  # miss (changed)
    cs.record_turn(msgs)  # hit (same)
    cs.record_turn(msgs)  # hit
    assert cs._hits == 2 and cs._total == 4
    assert cs.hit_rate == 0.5

def test_cache_stats_prefix_excludes_last():
    cs = CacheStats()
    msgs = [
        {'role': 'system', 'content': 'sys'},
        {'role': 'assistant', 'content': 'resp'},
        {'role': 'user', 'content': 'q1'},
    ]
    cs.record_turn(msgs)  # miss - prefix = system+assistant
    msgs[-1]['content'] = 'q2'
    r = cs.record_turn(msgs)  # prefix unchanged, should be hit
    assert r is True

def test_cache_stats_report():
    cs = CacheStats()
    cs._hits = 3; cs._misses = 1; cs._total = 4
    r = cs.report()
    assert '75.0%' in r
    assert '3/4' in r

def test_cache_stats_zero_report():
    cs = CacheStats()
    r = cs.report()
    assert '0/0' in r