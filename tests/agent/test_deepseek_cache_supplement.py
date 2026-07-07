
# ===================================================================
# Supplementary: CacheStats edge cases
# ===================================================================


class TestCacheStatsSerialization:
    def test_non_dict_in_messages(self):
        from agent.deepseek_cache import ensure_cache_stable
        msgs = ['raw string', {'role': 'user', 'content': 'hi'}]
        result = ensure_cache_stable(msgs)
        assert result[0] == 'raw string'
        assert result[1]['role'] == 'user'

    def test_deterministic_json_nested_dict(self):
        from agent.deepseek_cache import deterministic_json
        obj = {'outer': {'z': 1, 'a': 2}, 'b': 3}
        result = deterministic_json(obj)
        assert '"a": 2' in result
        assert '"z": 1' in result

    def test_deterministic_json_list(self):
        from agent.deepseek_cache import deterministic_json
        obj = {'items': [3, 1, 2], 'name': 'test'}
        result = deterministic_json(obj)
        assert 'items' in result

    def test_cache_stats_with_tool_calls(self):
        from agent.deepseek_cache import CacheStats
        cs = CacheStats()
        msgs = [
            {'role': 'user', 'content': 'search for python'},
            {'role': 'assistant', 'content': None, 'tool_calls': [{'id': 'c1', 'function': {'name': 'web_search', 'arguments': '{"q":"python"}'}}]},
        ]
        cs.record_turn(msgs)
        cs.record_turn(msgs)
        assert cs.hit_rate == 0.5

    def test_cache_stats_empty_list(self):
        from agent.deepseek_cache import CacheStats
        cs = CacheStats()
        cs.record_turn([])
        cs.record_turn([])
        assert cs._hits == 1

    def test_ensure_cache_stable_with_extra_fields(self):
        from agent.deepseek_cache import ensure_cache_stable
        msgs = [{'role': 'user', 'custom_field': 'value', 'content': 'hi'}]
        result = ensure_cache_stable(msgs)
        keys = list(result[0].keys())
        # keys should be sorted alphabetically
        assert keys == sorted(keys)
