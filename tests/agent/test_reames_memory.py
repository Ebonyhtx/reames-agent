"""Tests for ReamesMemory -- SQLite-backed L0-L3 memory engine."""
from __future__ import annotations
import sqlite3, threading, time
from unittest.mock import MagicMock
import pytest

@pytest.fixture
def mem(tmp_path):
    from agent.reames_memory import ReamesMemory
    m = ReamesMemory(data_dir=str(tmp_path))
    m._call_llm = MagicMock(return_value='')
    yield m
    try: m.shutdown()
    except: pass

def test_capture_writes_user_and_assistant(mem):
    mem.initialize(session_id='s')
    mem.capture_turn('hi', 'hello')
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute('SELECT role,content FROM messages ORDER BY id').fetchall()
    assert len(rows) == 2
    assert rows[0] == ('user', 'hi')
    assert rows[1] == ('assistant', 'hello')

def test_capture_empty_user_skipped(mem):
    mem.initialize(session_id='s')
    mem.capture_turn('', 'r')
    with sqlite3.connect(str(mem._db_path)) as c:
        assert c.execute('SELECT COUNT(*) FROM messages').fetchone()[0] == 0

def test_capture_empty_assistant_ok(mem):
    mem.initialize(session_id='s')
    mem.capture_turn('u', '')
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute('SELECT role FROM messages').fetchall()
    assert rows == [('user',)]

def test_session_isolation(mem):
    mem.initialize(session_id='a')
    mem.capture_turn('msg a', 'r a')
    mem.on_session_switch('b', 'a')
    mem.capture_turn('msg b', 'r b')
    with sqlite3.connect(str(mem._db_path)) as c:
        a = [r[0] for r in c.execute('SELECT content FROM messages WHERE session_id="a"').fetchall()]
        b = [r[0] for r in c.execute('SELECT content FROM messages WHERE session_id="b"').fetchall()]
    assert 'msg a' in a
    assert 'msg b' in b
    assert 'msg b' not in a

def test_turn_count_increments(mem):
    mem.initialize(session_id='s')
    assert mem._turn_count == 0
    mem.capture_turn('a', 'b')
    assert mem._turn_count == 1
    mem.capture_turn('c', 'd')
    assert mem._turn_count == 2

def test_concurrent_writes(mem):
    mem.initialize(session_id='s')
    def w(n):
        for i in range(20): mem.capture_turn(f'{n}-{i}', f'r{n}-{i}')
    ts = [threading.Thread(target=w, args=(t,)) for t in range(4)]
    for t in ts: t.start()
    for t in ts: t.join()
    with sqlite3.connect(str(mem._db_path)) as c:
        cnt = c.execute('SELECT COUNT(*) FROM messages').fetchone()[0]
    assert cnt == 160

def test_init_idempotent(mem):
    mem._init_db()
    mem._init_db()

def test_extract_empty_response(mem):
    mem.initialize(session_id='s')
    mem._call_llm = MagicMock(return_value='')
    mem._extract_l1()
    with sqlite3.connect(str(mem._db_path)) as c:
        assert c.execute('SELECT COUNT(*) FROM memories').fetchone()[0] == 0

def test_extract_llm_exception(mem):
    mem.initialize(session_id='s')
    mem._call_llm = MagicMock(side_effect=RuntimeError('x'))
    mem._extract_l1()

def test_extract_facts_written(mem):
    mem.initialize(session_id='s')
    mem.capture_turn('I use Python', 'ok')
    mem._call_llm = MagicMock(return_value='- user uses Python
- user likes coding')
    mem._extract_l1()
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute('SELECT content FROM memories').fetchall()
    assert len(rows) == 2

def test_extract_dedup(mem):
    mem.initialize(session_id='s')
    def ex(f):
        mem._call_llm = MagicMock(return_value=f)
        mem._extract_l1()
    ex('- A
- B')
    ex('- A
- C')
    with sqlite3.connect(str(mem._db_path)) as c:
        rows = c.execute('SELECT content FROM memories').fetchall()
    assert len(rows) == 3
    assert sum(1 for r in rows if r[0] == 'A') == 1

def test_extract_interval_gate(mem):
    mem.initialize(session_id='s')
    mem.configure_extraction(l1=10)
    orig = mem._extract_l1
    mem._extract_l1 = MagicMock(wraps=orig)
    for i in range(5): mem.capture_turn(f'm{i}', f'r{i}')
    mem._extract_l1.assert_not_called()

def test_l2_aggregate_creates_scenes(mem):
    mem.initialize(session_id='s')
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute('INSERT INTO memories(session_id,content) VALUES(?,?)', ('s', 'fact A'))
        c.execute('INSERT INTO memories(session_id,content) VALUES(?,?)', ('s', 'fact B'))
        c.commit()
    mem._last_l2_count = 0
    mem._call_llm = MagicMock(return_value='## Scene
- fact A')
    mem._aggregate_l2()
    assert mem._scenes_path.exists()

def test_l2_empty_memories(mem):
    mem.initialize(session_id='s')
    mem._call_llm = MagicMock()
    mem._aggregate_l2()
    mem._call_llm.assert_not_called()

def test_l3_synthesize_persona(mem):
    mem.initialize(session_id='s')
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute('INSERT INTO memories(session_id,content) VALUES(?,?)', ('s', 'fact'))
        c.commit()
    mem._call_llm = MagicMock(return_value='User is a developer.')
    mem._synthesize_l3()
    assert mem._persona_path.exists()
    assert 'developer' in mem._persona_path.read_text()

def test_l3_empty_memories(mem):
    mem.initialize(session_id='s')
    mem._call_llm = MagicMock()
    mem._synthesize_l3()
    mem._call_llm.assert_not_called()

def test_system_prompt_block(mem):
    mem._persona_path.write_text('persona data', encoding='utf-8')
    block = mem.system_prompt_block()
    assert 'persona data' in block

def test_system_prompt_block_no_file(mem):
    assert mem.system_prompt_block() == ''

def test_search_keyword_hits(mem):
    mem.initialize(session_id='s')
    mem.capture_turn('I love Python', 'Great')
    with sqlite3.connect(str(mem._db_path)) as c:
        c.execute('INSERT INTO memories(session_id,content) VALUES(?,?)', ('s', 'Python is great'))
        c.commit()
    r = mem._search_keyword('Python')
    assert any('Python' in x[0] for x in r)

def test_search_keyword_no_match(mem):
    mem.initialize(session_id='s')
    r = mem._search_keyword('zzz999nonexistent')
    assert len(r) == 0

def test_recall_empty_query(mem):
    assert mem.recall('') == ''

def test_tool_schemas_four(mem):
    s = mem.get_tool_schemas()
    assert len(s) == 4

def test_tool_schemas_names(mem):
    names = [x['name'] for x in mem.get_tool_schemas()]
    assert 'reames_memory_search' in names
    assert 'reames_memory_list' in names
    assert 'reames_memory_status' in names
    assert 'reames_memory_prune' in names

def test_handle_unknown_tool(mem):
    r = mem.handle_tool_call('bad_tool', {})
    assert 'Unknown' in r

def test_shutdown_clean(mem):
    mem.initialize(session_id='s')
    mem.shutdown()

def test_shutdown_idempotent(mem):
    mem.shutdown()
    mem.shutdown()

def test_on_session_end(mem):
    mem.on_session_end()

def test_db_deleted_external(mem):
    mem.initialize(session_id='s')
    if mem._db_path.exists(): mem._db_path.unlink()
    mem.capture_turn('hi', 'ok')

def test_call_llm_timeout(mem):
    mem.initialize(session_id='s')
    mem._call_llm = MagicMock(side_effect=TimeoutError('timeout'))
    mem._extract_l1()

def test_switch_session(mem):
    mem.initialize(session_id='s1', user_id='u1')
    mem.on_session_switch('s2', 's1')
    assert mem._session_id == 's2'
    assert mem._user_id == 'u1'
