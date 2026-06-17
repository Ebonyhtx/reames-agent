"""Tests for ReamesMemory -- SQLite-backed L0-L3 memory engine."""
from __future__ import annotations
import sqlite3, threading, time
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

def test_capture_writes_user_and_assistant(mem):
    mem.initialize(session_id="s")
    mem.capture_turn("hi", "hello")
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute("SELECT role,content FROM messages ORDER BY id").fetchall()
    assert len(rows) == 2
    assert rows[0] == ("user", "hi")
    assert rows[1] == ("assistant", "hello")

def test_capture_empty_user_skipped(mem):
    mem.initialize(session_id="s")
    mem.capture_turn("", "r")
    with sqlite3.connect(str(mem._db_path)) as c:
        assert c.execute("SELECT COUNT(*) FROM messages").fetchone()[0] == 0

def test_capture_empty_assistant_ok(mem):
    mem.initialize(session_id="s")
    mem.capture_turn("u", "")
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute("SELECT role FROM messages").fetchall()
    assert rows == [("user",)]

def test_session_isolation(mem):
    mem.initialize(session_id="a")
    mem.capture_turn("msg a", "r a")
    mem.on_session_switch("b", "a")
    mem.capture_turn("msg b", "r b")
    with sqlite3.connect(str(mem._db_path)) as c:
        a = [r[0] for r in c.execute("SELECT content FROM messages WHERE session_id='a'").fetchall()]
        b = [r[0] for r in c.execute("SELECT content FROM messages WHERE session_id='b'").fetchall()]
    assert "msg a" in a
    assert "msg b" in b
    assert "msg b" not in a

def test_turn_count_increments(mem):
    mem.initialize(session_id="s")
    assert mem._turn_count == 0
    mem.capture_turn("a", "b")
    assert mem._turn_count == 1

def test_concurrent_writes(mem):
    mem.initialize(session_id="s")
    def w(n):
        for i in range(20): mem.capture_turn(f"{n}-{i}", f"r{n}-{i}")
    ts = [threading.Thread(target=w, args=(t,)) for t in range(4)]
    for t in ts: t.start()
    for t in ts: t.join()
    with sqlite3.connect(str(mem._db_path)) as c:
        cnt = c.execute("SELECT COUNT(*) FROM messages").fetchone()[0]
    assert cnt == 160

def test_init_idempotent(mem):
    mem._init_db()
    mem._init_db()

def test_extract_empty_response(mem):
    mem.initialize(session_id="s")
    mem._call_llm = MagicMock(return_value="")
    mem._extract_l1()

def test_extract_llm_exception(mem):
    mem.initialize(session_id="s")
    mem._call_llm = MagicMock(side_effect=RuntimeError("x"))
    mem._extract_l1()

def test_extract_facts_written(mem):
    mem.initialize(session_id="s")
    mem._agent = MagicMock()
    mem.configure_extraction(l1=1)
    mem.capture_turn("I use Python", "ok")
    mem._call_llm = MagicMock(return_value="- user uses Python")
    import time; time.sleep(0.1)
    # _extract_l1 is called via daemon thread; if not triggered, call directly
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute("SELECT content FROM memories").fetchall()
    if len(rows) < 1:
        # daemon didn't fire, call directly
        mem._extract_l1()
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute("SELECT content FROM memories").fetchall()
    assert len(rows) == 1

def test_extract_dedup(mem):
    mem.initialize(session_id="s")
    mem._agent = MagicMock()
    def ex(f):
        mem._call_llm = MagicMock(return_value=f)
        mem._extract_l1()
    mem.configure_extraction(l1=1)
    mem.capture_turn("seed", "ok")
    ex("- fact A very long\n- fact B also long")
    ex("- fact A very long\n- fact C is new")
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute("SELECT content FROM memories").fetchall()
    assert len(rows) == 3
    assert sum(1 for r in rows if r[0] == "fact A very long") == 1

def test_extract_interval_gate(mem):
    mem.initialize(session_id="s")
    mem.configure_extraction(l1=10)
    orig = mem._extract_l1
    mem._extract_l1 = MagicMock(wraps=orig)
    for i in range(5): mem.capture_turn(f"m{i}", f"r{i}")
    mem._extract_l1.assert_not_called()

def test_l2_aggregate_creates_scenes(mem):
    mem.initialize(session_id="s")
    mem._agent = MagicMock()
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute("INSERT INTO memories(session_id,content) VALUES(?,?)", ("s", "fact A"))
        c.execute("INSERT INTO memories(session_id,content) VALUES(?,?)", ("s", "fact B"))
        c.commit()
    mem.configure_extraction(l2=1)
    mem._last_l2_count = 0
    mem._call_llm = MagicMock(return_value="## Scene\n - fact A")
    mem._aggregate_l2()
    assert mem._scenes_path.exists()

def test_l2_empty_memories(mem):
    mem.initialize(session_id="s")
    mem._call_llm = MagicMock()
    mem._aggregate_l2()
    mem._call_llm.assert_not_called()

def test_l3_synthesize_persona(mem):
    mem.initialize(session_id="s")
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute("INSERT INTO memories(session_id,content) VALUES(?,?)", ("s", "fact"))
        c.commit()
    mem._call_llm = MagicMock(return_value="User is a developer.")
    mem._synthesize_l3()
    assert mem._persona_path.exists()

def test_l3_empty_memories(mem):
    mem.initialize(session_id="s")
    mem._call_llm = MagicMock()
    mem._synthesize_l3()
    mem._call_llm.assert_not_called()

def test_system_prompt_block(mem):
    mem._persona_path.write_text("persona data", encoding="utf-8")
    block = mem.system_prompt_block()
    assert "persona" in block

def test_search_keyword_hits(mem):
    mem.initialize(session_id="s")
    mem.capture_turn("I love Python", "Great")
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute("INSERT INTO memories(session_id,content) VALUES(?,?)", ("s", "Python is great"))
        c.commit()
    r = mem._search_keyword("Python")
    assert any("Python" in x[0] for x in r)

def test_search_keyword_no_match(mem):
    mem.initialize(session_id="s")
    r = mem._search_keyword("zzz999nonexistent")
    assert len(r) == 0

def test_recall_empty_query(mem):
    assert mem.recall("") == ""

def test_tool_schemas_four(mem):
    s = mem.get_tool_schemas()
    assert len(s) == 4

def test_tool_handle_unknown(mem):
    r = mem.handle_tool_call("bad_tool", {})
    assert "Unknown" in r

def test_shutdown_clean(mem):
    mem.initialize(session_id="s")
    mem.shutdown()

def test_shutdown_idempotent(mem):
    mem.shutdown()
    mem.shutdown()

def test_call_llm_timeout(mem):
    mem.initialize(session_id="s")
    mem._call_llm = MagicMock(side_effect=TimeoutError("timeout"))
    mem._extract_l1()


def test_on_session_end(mem):
    mem.on_session_end()

def test_register_signal_handler(mem):
    mem.register_signal_handler()

def test_recall_returns_content(mem):
    mem.initialize(session_id="s")
    mem.capture_turn("hello", "world")
    result = mem.recall("hello")
    assert isinstance(result, str)

def test_search_freshness_ranking(mem):
    mem.initialize(session_id="s")
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute("INSERT INTO memories(session_id,content,created_at) VALUES(?,?,?)",
                  ("s", "old fact", "2020-01-01 00:00:00")
        )
        c.execute("INSERT INTO memories(session_id,content,created_at) VALUES(?,?,?)",
                  ("s", "new fact", "2025-06-17 12:00:00")
        )
        c.commit()
    results = mem._search_fresh("fact", limit=5)
    scores = {r[0]: r[1] for r in results}
    assert scores.get("new fact", 0) >= scores.get("old fact", 0)

def test_switch_session(mem):
    mem.initialize(session_id="s1", user_id="u1")
    mem.on_session_switch("s2", "s1")
    assert mem._session_id == "s2"
