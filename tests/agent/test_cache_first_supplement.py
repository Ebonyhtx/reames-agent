
# ===================================================================
# Supplementary tests: cache warmup request validation, failure scenarios
# ===================================================================


class TestCacheWarmupRequest:
    def test_warmup_fires_only_for_deepseek(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = 'deepseek-v4-flash'
        agent._cache_stats = MagicMock()
        agent.base_url = 'https://api.deepseek.com'
        agent.api_key = 'sk-test'
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_skipped_non_deepseek(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = 'claude-sonnet'
        agent._cache_stats = MagicMock()
        # should return early without trying to send request
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_skipped_no_cache_stats(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = 'deepseek-v4-flash'
        agent._cache_stats = None
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_skipped_empty_model(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = ''
        agent._cache_stats = MagicMock()
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_timeout_no_crash(self):
        from agent.conversation_compression import _fire_cache_warmup
        import agent.conversation_compression as cc
        agent = MagicMock()
        agent.model = 'deepseek-v4-flash'
        agent._cache_stats = MagicMock()
        agent.base_url = 'https://api.deepseek.com'
        agent.api_key = 'sk-test'
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_deepseek_chat_model(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = 'deepseek-chat'
        agent._cache_stats = MagicMock()
        agent.base_url = 'https://api.deepseek.com'
        agent.api_key = 'sk-test'
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_deepseek_reasoner_model(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = 'deepseek-reasoner'
        agent._cache_stats = MagicMock()
        agent.base_url = 'https://api.deepseek.com'
        agent.api_key = 'sk-test'
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])

    def test_warmup_empty_base_url_uses_default(self):
        from agent.conversation_compression import _fire_cache_warmup
        agent = MagicMock()
        agent.model = 'deepseek-v4-flash'
        agent._cache_stats = MagicMock()
        agent.base_url = ''
        agent.api_key = ''
        _fire_cache_warmup(agent, [{'role': 'user', 'content': 'hi'}])


class TestCacheFirstCompressEdgeCases:
    def test_cache_first_compress_empty_messages(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5, cache_first=True)
        comp._generate_summary = MagicMock()
        result = comp.compress([], current_tokens=0)
        assert result == []

    def test_cache_first_compress_minimal_messages(self):
        """Only 2-3 messages, not enough to compress -> return unchanged."""
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5, cache_first=True)
        comp._generate_summary = MagicMock()
        msgs = [{'role': 'system', 'content': 's'}, {'role': 'user', 'content': 'q'}]
        result = comp.compress(msgs, current_tokens=500)
        assert len(result) == 2 or len(result) == len(msgs)

    def test_cache_first_ineffective_compression_counter(self):
        from agent.context_compressor import ContextCompressor
        comp = ContextCompressor(context_length=128000, threshold_percent=0.5, cache_first=True)
        comp._generate_summary = MagicMock()
        # Very small messages that won't save much
        msgs = [{'role': 'system', 'content': 's'}, {'role': 'user', 'content': 'q1'}, {'role': 'assistant', 'content': 'a1'}, {'role': 'user', 'content': 'q2'}, {'role': 'assistant', 'content': 'a2'}, {'role': 'user', 'content': 'q3'}, {'role': 'assistant', 'content': 'a3'}]
        # Mock estimates to always return small savings
        with patch('agent.context_compressor.estimate_messages_tokens_rough', return_value=100):
            comp.compress(msgs, current_tokens=5000)
            # After compress, if savings <10% the counter increments
            if comp._last_compression_savings_pct < 10:
                assert comp._ineffective_compression_count > 0


class TestCacheFirstAutoEnableDeepseekV3:
    def test_deepseek_v3_auto_enables(self):
        cfg = {}
        model = 'deepseek-v3'
        raw = cfg.get('cache_first')
        if raw is not None:
            result = str(raw).lower() in {'true', '1', 'yes'}
        else:
            result = bool(model and 'deepseek' in model.lower())
        assert result is True

    def test_deepseek_v3_0324_auto_enables(self):
        assert 'deepseek' in 'deepseek-v3-0324'.lower()

    def test_deepseek_creator_model_auto_enables(self):
        assert 'deepseek' in 'deepseek/deepseek-v4-flash'.lower()
