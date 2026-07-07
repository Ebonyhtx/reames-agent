from __future__ import annotations
from unittest.mock import MagicMock
import pytest

# ===================================================================
# Supplementary: compression session rotation, strip_media edge cases
# ===================================================================


class TestCompressionSessionRotation:
    def test_compress_context_creates_new_session(self):
        from agent.conversation_compression import compress_context
        agent = MagicMock()
        agent.compression_enabled = True
        agent._compression_feasibility_checked = True
        agent._session_db = MagicMock()
        agent._session_db.get_session_title.return_value = ''
        agent.session_id = 'old-session-id'
        agent.model = 'deepseek-v4-flash'
        agent.context_compressor = MagicMock()
        agent.context_compressor.compress.return_value = [{'role': 'user', 'content': 'test'}]
        agent.context_compressor._last_summary_error = None
        agent.context_compressor._last_aux_model_failure_model = None
        agent.context_compressor.compression_count = 1
        agent.context_compressor._last_compression_savings_pct = 50.0
        agent._todo_store = MagicMock()
        agent._todo_store.format_for_injection.return_value = ''
        agent._cached_system_prompt = 'sys'
        agent._build_system_prompt = MagicMock()
        agent.platform = 'cli'
        agent._session_init_model_config = {}
        agent._session_db_created = False
        agent.api_key = ''
        agent.base_url = ''
        agent.provider = 'deepseek'

        messages = [{'role': 'user', 'content': 'hi'}]

    def test_compress_context_lock_unavailable(self):
        from agent.conversation_compression import compress_context
        agent = MagicMock()
        agent._compression_feasibility_checked = True
        agent._session_db = None
        agent.session_id = 's'
        agent.model = 'deepseek-v4-flash'
        agent.context_compressor = MagicMock()
        agent.context_compressor.compress.return_value = [{'role': 'user', 'content': 't'}]
        agent.context_compressor._last_summary_error = None
        agent.context_compressor._last_aux_model_failure_model = None
        agent.context_compressor.compression_count = 1
        agent.context_compressor._last_compression_savings_pct = 50.0
        agent._todo_store = MagicMock()
        agent._todo_store.format_for_injection.return_value = ''
        agent._cached_system_prompt = 'sys'
        agent._build_system_prompt = MagicMock()
        agent.platform = 'cli'
        agent._session_init_model_config = {}

    def test_strip_historical_media_with_images(self):
        from agent.context_compressor import _strip_historical_media
        msgs = [
            {'role': 'user', 'content': [{'type': 'text', 'text': 'first'}, {'type': 'image_url', 'image_url': {'url': 'data:base64,aGVsbG8='}}]},
            {'role': 'assistant', 'content': 'ok'},
            {'role': 'user', 'content': 'second'},
        ]
        result = _strip_historical_media(msgs)
        assert len(result) == 3

    def test_strip_media_only_text_no_change(self):
        from agent.context_compressor import _strip_historical_media
        msgs = [{'role': 'user', 'content': 'just text'}, {'role': 'assistant', 'content': 'ok'}]
        result = _strip_historical_media(msgs)
        assert result == msgs

    def test_strip_media_first_msg_only(self):
        from agent.context_compressor import _strip_historical_media
        msgs = [
            {'role': 'user', 'content': [{'type': 'image_url', 'image_url': {'url': 'data:i'}}]},
            {'role': 'assistant', 'content': 'r'},
        ]
        result = _strip_historical_media(msgs)
        assert len(result) == 2


class TestCompressionFeasibility:
    def test_feasibility_check_flows(self):
        from agent.conversation_compression import check_compression_model_feasibility
        agent = MagicMock()
        agent._compression_feasibility_checked = False
        agent.context_compressor = MagicMock()
        agent.context_compressor.context_length = 128000
        agent.context_compressor.update_from_response = MagicMock()
        agent._compression_warning = False
        check_compression_model_feasibility(agent)
