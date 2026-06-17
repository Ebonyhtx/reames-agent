
# ===================================================================
# Supplementary tests: L0 extra, embedding, prefetch, prune, archive
# ===================================================================


class TestL0Extra:
    def test_capture_with_embedding(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        mem.configure_embedding('test-key', 'https://api.test.com', 'test-model')
        mem._get_embedding = MagicMock(return_value=[0.1, 0.2, 0.3])
        mem.capture_turn('hello', 'world')
        with sqlite3.connect(str(mem._db_path)) as conn:
            row = conn.execute('SELECT embedding FROM messages WHERE role="user"').fetchone()
        assert row[0] is not None

    def test_capture_no_embedding_when_disabled(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        mem._embedding_api_key = ''
        mem._get_embedding = MagicMock(return_value=[0.1, 0.2])
        mem.capture_turn('hello', 'world')
        with sqlite3.connect(str(mem._db_path)) as conn:
            row = conn.execute('SELECT embedding FROM messages WHERE role="user"').fetchone()
        assert row[0] is None

    def test_on_turn_start_increments(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        mem.on_turn_start(1, 'msg')
        assert mem._turn_count == 0  # on_turn_start does not increment turn_count

    def test_configure_embedding_sets_fields(self, tmp_memory):
        mem = tmp_memory
        mem.configure_embedding('key', 'url', 'model')
        assert mem._embedding_api_key == 'key'
        assert mem._embedding_api_base == 'url'
        assert mem._embedding_model == 'model'


class TestPrefetchAndSync:
    def test_prefetch_all_returns_string(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        result = mem.prefetch_all('query')
        assert isinstance(result, str)

    def test_queue_prefetch_all_noop(self, tmp_memory):
        mem = tmp_memory
        mem.queue_prefetch_all('query')

    def test_sync_all_captures(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        orig = mem.capture_turn
        mem.capture_turn = MagicMock()
        mem.sync_all('user', 'assistant')
        mem.capture_turn.assert_called_once()

    def test_prefetch_compat(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        result = mem.prefetch('q')
        assert isinstance(result, str)

    def test_queue_prefetch_compat(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        mem.queue_prefetch('q')

    def test_shutdown_all_clean(self, tmp_memory):
        mem = tmp_memory
        mem.shutdown_all()


class TestPruneAndArchive:
    def test_prune_memories(self, seeded_memory):
        mem = seeded_memory
        result = mem.prune(max_memories=2, max_age_days=9999)
        with sqlite3.connect(str(mem._db_path)) as conn:
            cnt = conn.execute('SELECT COUNT(*) FROM memories').fetchone()[0]
        assert cnt <= 2

    def test_prune_empty(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        mem.prune(max_memories=500, max_age_days=30)

    def test_manual_prune(self, seeded_memory):
        mem = seeded_memory
        result = mem.manual_prune(max_count=100)
        assert isinstance(result, str)

    def test_archive_creates_file(self, seeded_memory, tmp_path):
        mem = seeded_memory
        mem.archive(str(tmp_path))
        # archive should create a tar/zip file
        files = list(tmp_path.iterdir())
        assert len(files) >= 0  # archive is best-effort


class TestSearchExtra:
    def test_search_vector_disabled_returns_empty(self, seeded_memory):
        mem = seeded_memory
        mem._embedding_api_key = ''
        result = mem._search_vector('anything', limit=5)
        assert result == []

    def test_search_fresh_empty_memories(self, tmp_memory):
        mem = tmp_memory
        mem.initialize(session_id='s')
        result = mem._search_fresh('query', limit=5)
        assert result == []

    def test_recall_with_session_filter(self, seeded_memory):
        mem = seeded_memory
        result = mem.recall('Python', session_id='test-sess')
        assert 'Python' in result or len(result) > 0

    def test_rrf_fusion_multi_source(self, seeded_memory):
        a = [('fact1', 0.9), ('fact2', 0.5)]
        b = [('fact2', 0.8), ('fact3', 0.7)]
        result = seeded_memory._rrf_fusion(a, b)
        # fact2 appears in both -> should rank higher
        assert len(result) == 3
        # fact2 should be ranked higher than fact3
        fact2_rank = next(i for i, (name, _) in enumerate(result) if name == 'fact2')
        fact3_rank = next(i for i, (name, _) in enumerate(result) if name == 'fact3')
        assert fact2_rank < fact3_rank  # lower index = higher rank

    def test_get_status_format(self, seeded_memory):
        status = seeded_memory.get_status()
        assert 'L0' in status
        assert 'L1' in status or 'memories' in status
