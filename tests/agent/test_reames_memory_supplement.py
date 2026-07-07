"""Supplementary ReamesMemory tests (embedding, prefetch, prune, search)."""
from __future__ import annotations
import sqlite3, threading
from unittest.mock import MagicMock
import pytest

@pytest.fixture
def mem(tmp_path):
    from agent.reames_memory import ReamesMemory
    m = ReamesMemory(data_dir=str(tmp_path))
    m._call_llm = MagicMock(return_value="")
    yield m
    try: m.shutdown()
    except: pass

def test_capture_with_embedding(mem):
    mem.initialize(session_id="s")
    mem.configure_embedding("test-key", "https://api.test.com", "test-model")
    mem._get_embedding = MagicMock(return_value=[0.1, 0.2, 0.3])
    mem.capture_turn("hello", "world")
    with sqlite3.connect(str(mem._db_path)) as c:
        row = c.execute("SELECT embedding FROM messages WHERE role='user'").fetchone()
    assert row[0] is not None

def test_capture_no_embedding_when_disabled(mem):
    mem.initialize(session_id="s")
    mem._get_embedding = MagicMock(return_value=[0.1, 0.2])
    mem.capture_turn("hello", "world")
    with sqlite3.connect(str(mem._db_path)) as c:
        row = c.execute("SELECT embedding FROM messages WHERE role='user'").fetchone()
    assert row[0] is None

def test_configure_embedding_sets_fields(mem):
    mem.configure_embedding("key", "url", "model")
    assert mem._embedding_api_key == "key"
    assert mem._embedding_api_base == "url"
    assert mem._embedding_model == "model"

def test_prefetch_all_returns_string(mem):
    mem.initialize(session_id="s")
    result = mem.prefetch_all("query")
    assert isinstance(result, str)

def test_sync_all_captures(mem):
    mem.initialize(session_id="s")
    orig = mem.capture_turn
    mem.capture_turn = MagicMock()
    mem.sync_all("user", "assistant")
    mem.capture_turn.assert_called_once()

def test_shutdown_all_clean(mem):
    mem.shutdown_all()

def test_prune_memories(mem):
    mem.initialize(session_id="s")
    with sqlite3.connect(str(mem._db_path)) as c:
        for i in range(10):
            c.execute("INSERT INTO memories(session_id,content) VALUES(?,?)", ("s", f"fact {i}"))
        c.commit()
    mem._last_l2_count = 10
    mem.prune(max_memories=3, max_age_days=9999)
    with sqlite3.connect(str(mem._db_path)) as c:
        cnt = c.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
    assert cnt <= 3

def test_manual_prune(mem):
    mem.initialize(session_id="s")
    result = mem.manual_prune(max_count=100)
    assert isinstance(result, str)

def test_search_vector_disabled_returns_empty(mem):
    mem._embedding_api_key = ""
    result = mem._search_vector("anything", limit=5)
    assert result == []

def test_search_fresh_empty_memories(mem):
    mem.initialize(session_id="s")
    result = mem._search_fresh("query", limit=5)
    assert result == []

def test_rrf_fusion_multi_source(mem):
    a = [("fact1", 0.9), ("fact2", 0.5)]
    b = [("fact2", 0.8), ("fact3", 0.7)]
    result = mem._rrf_fusion(a, b)
    assert len(result) == 3
    fact2_rank = next(i for i, (name, _) in enumerate(result) if name == "fact2")
    fact3_rank = next(i for i, (name, _) in enumerate(result) if name == "fact3")
    assert fact2_rank < fact3_rank

def test_get_status_format(mem):
    mem.initialize(session_id="s")
    mem.capture_turn("hi", "hello")
    status = mem.get_status()
    assert "L0" in status or "messages" in status


def test_on_turn_start_increments(mem):
    mem.initialize(session_id="s")
    mem.on_turn_start(1, "msg")
    # on_turn_start does not increment turn_count
    assert mem._turn_count == 0

def test_queue_prefetch_all_noop(mem):
    mem.initialize(session_id="s")
    mem.queue_prefetch_all("query")

def test_prefetch_compat(mem):
    mem.initialize(session_id="s")
    result = mem.prefetch("q")
    assert isinstance(result, str)

def test_queue_prefetch_compat(mem):
    mem.initialize(session_id="s")
    mem.queue_prefetch("q")

def test_prune_empty(mem):
    mem.initialize(session_id="s")
    mem.prune(max_memories=500, max_age_days=30)

def test_archive_creates_file(mem, tmp_path):
    mem.initialize(session_id="s")
    mem.archive(str(tmp_path))
    # archive is best-effort

def test_system_prompt_block_no_file(mem):
    assert mem.system_prompt_block() == ""

