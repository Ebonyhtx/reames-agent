"""Reames Memory — Python native L0-L3 memory engine. No external Gateway needed.

Uses SQLite for storage, DeepSeek for extraction, optional embedding API for vector search.
"""

from __future__ import annotations

import json
import logging
import os
import sqlite3
import threading
import re
from pathlib import Path
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple

logger = logging.getLogger(__name__)

DEFAULT_L1_INTERVAL = 10  # user-LLM turns
DEFAULT_L2_INTERVAL = 20
DEFAULT_L3_INTERVAL = 50
DEFAULT_RECALL_COUNT = 10


class ReamesMemory:
    """Reames native memory engine. SQLite-backed, zero external dependencies.

    L0: raw conversation messages
    L1: atomic facts (LLM-extracted, optionally vectorized)
    L2: scene blocks (aggregated from L1 facts)
    L3: user persona (synthesized from L2 scenes)

    All four layers stored in SQLite. FTS5 keyword + vector semantic search.
    """

    def __init__(self, data_dir: Optional[str] = None):
        if data_dir:
            self._data_dir = Path(data_dir).expanduser()
        else:
            home = os.environ.get("HOME") or os.environ.get("USERPROFILE") or "."
            self._data_dir = Path(home) / ".reames" / "memory"
        self._data_dir.mkdir(parents=True, exist_ok=True)

        self._db_path = self._data_dir / "reames_memory.db"
        self._persona_path = self._data_dir / "persona.md"
        self._scenes_path = self._data_dir / "scenes.md"

        self._session_id = ""
        self._user_id = "default"
        self._turn_count = 0
        self._l1_pending = 0
        self._lock = threading.Lock()
        self._agent: Any = None
        self._last_l2_count = 0
        self._last_l3_count = 0

        self._embedding_api_key = ""
        self._embedding_api_base = ""
        self._embedding_model = ""

        self._l1_interval = DEFAULT_L1_INTERVAL
        self._l2_interval = DEFAULT_L2_INTERVAL
        self._l3_interval = DEFAULT_L3_INTERVAL
        self._recall_count = DEFAULT_RECALL_COUNT

        self._init_db()

    # -- Database ---------------------------------------------------

    def _init_db(self):
        with sqlite3.connect(str(self._db_path)) as conn:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.execute("""CREATE TABLE IF NOT EXISTS messages (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                session_id TEXT NOT NULL, role TEXT NOT NULL,
                content TEXT NOT NULL, embedding BLOB,
                created_at TEXT DEFAULT (datetime('now'))
            )""")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_msg_sess ON messages(session_id)")

            conn.execute("""CREATE TABLE IF NOT EXISTS memories (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                session_id TEXT NOT NULL,
                content TEXT NOT NULL, embedding BLOB,
                created_at TEXT DEFAULT (datetime('now'))
            )""")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_mem_sess ON memories(session_id)")

            # Add unique index to prevent duplicates
            conn.execute("CREATE UNIQUE INDEX IF NOT EXISTS idx_mem_content ON memories(content)")

            for table in ("memories", "messages"):
                conn.execute(f"""CREATE VIRTUAL TABLE IF NOT EXISTS {table}_fts USING fts5(
                    content, content='{table}', content_rowid='id'
                )""")
                conn.executescript(f"""
                    CREATE TRIGGER IF NOT EXISTS {table}_ai AFTER INSERT ON {table} BEGIN
                        INSERT INTO {table}_fts(rowid, content) VALUES (new.id, new.content);
                    END;
                    CREATE TRIGGER IF NOT EXISTS {table}_ad AFTER DELETE ON {table} BEGIN
                        INSERT INTO {table}_fts({table}_fts, rowid, content) VALUES('delete', old.id, old.content);
                    END;
                """)
            conn.commit()

    # -- Config -----------------------------------------------------

    def configure_embedding(self, api_key: str, api_base: str, model: str):
        self._embedding_api_key = api_key
        self._embedding_api_base = api_base.rstrip("/")
        self._embedding_model = model
        # Retroactively embed existing facts
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                rows = conn.execute("SELECT id, content FROM memories WHERE embedding IS NULL").fetchall()
                done = 0
                for mid, text in rows:
                    emb = self._get_embedding(text)
                    if emb:
                        conn.execute("UPDATE memories SET embedding=? WHERE id=?", (self._blob_from_vec(emb), mid))
                        done += 1
                conn.commit()
            if done:
                logger.info("Retroactive embedding: %d facts", done)
        except Exception:
            pass

    def configure_extraction(self, l1: int = 10, l2: int = 50, l3: int = 200):
        self._l1_interval = l1
        self._l2_interval = l2
        self._l3_interval = l3

    # -- Core API ---------------------------------------------------

    def initialize(self, *, session_id: str = "", user_id: str = "",
                   platform: str = "", hermes_home: str = "",
                   agent: Any = None, **kwargs):
        self._session_id = session_id
        self._user_id = user_id or "default"
        self._agent = agent
        self._turn_count = 0
        logger.info("ReamesMemory: initialized session=%s", session_id)
        # Startup check: trigger L3 persona if threshold met
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                cnt = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            if cnt >= self._l3_interval:
                threading.Thread(target=self._synthesize_l3, name="reames-l3-init").start()
        except Exception:
            pass

    def capture_offload(self, tool_name: str, content: str):
        """Store offloaded tool output for future retrieval (L0, tool role)."""
        if not content:
            return
        with sqlite3.connect(str(self._db_path)) as conn:
            conn.execute(
                "INSERT INTO messages (session_id, role, content) VALUES (?,?,?)",
                (self._session_id, "tool", f"[{tool_name}] {content[:3000]}")
            )
            conn.commit()

    def capture_turn(self, user_content: str, assistant_content: str):
        if not user_content:
            return
        with self._lock:
            self._turn_count += 1
            with sqlite3.connect(str(self._db_path)) as conn:
                user_emb = self._get_embedding(user_content) if self._embedding_api_key else None
                conn.execute(
                    "INSERT INTO messages (session_id, role, content, embedding) VALUES (?,?,?,?)",
                    (self._session_id, "user", user_content, self._blob_from_vec(user_emb)))
                if assistant_content:
                    asst_emb = self._get_embedding(assistant_content) if self._embedding_api_key else None
                    conn.execute(
                        "INSERT INTO messages (session_id, role, content, embedding) VALUES (?,?,?,?)",
                        (self._session_id, "assistant", assistant_content, self._blob_from_vec(asst_emb)))
                conn.commit()
            self._l1_pending += 1

        # L2 accumulation check (before L1 resets _l1_pending).
        # L3 is only triggered at session start (initialize) and session end
        # (on_session_end) — it doesn't run mid-session.
        # Throttle: only check every _l1_interval turns.
        if self._turn_count % self._l1_interval == 0:
            try:
                with sqlite3.connect(str(self._db_path)) as conn:
                    cnt = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
                if cnt >= self._l2_interval:
                    threading.Thread(target=self._aggregate_l2, name="reames-l2").start()
            except Exception:
                pass

        if self._l1_pending >= self._l1_interval and self._agent:
            self._l1_pending = 0
            t = threading.Thread(target=self._extract_l1, daemon=True, name="reames-l1")
            t.start()

    def recall(self, query: str, *, session_id: str = "") -> str:
        """Search all layers (L0+L1+L2+L3) and return ranked results."""
        if not query:
            return ""

        # L2+L3: add scene/persona text if available
        extra = ""
        if self._scenes_path.exists():
            try:
                scene_text = self._scenes_path.read_text(encoding="utf-8")
                titles = [l for l in scene_text.split(chr(10)) if l.startswith("## ")]
                # Match: space tokens + Chinese 2-gram chunks
                cjk = "".join(c for c in query if "一" <= c <= "鿿")
                chunks = query.split() + [cjk[i:i+2] for i in range(len(cjk)-1)]
                if any(c in t.lower() for c in chunks for t in titles):
                    extra += scene_text[:300] + chr(10)
            except Exception:
                pass
        if self._persona_path.exists():
            try:
                extra += self._persona_path.read_text(encoding="utf-8")[:150] + "\n"
            except Exception:
                pass

        kw = self._search_fresh(query, self._recall_count)
        results = kw
        if self._embedding_api_key:
            try:
                vec = self._search_vector(query, self._recall_count)
                results = self._rrf_fusion(kw, vec)
            except Exception as e:
                logger.debug("Vector search failed: %s", e)

        if not results and not extra:
            return ""

        parts = ["## Reames Memory\n"]
        if extra:
            parts.append(extra.strip())
        for content, _ in results[:self._recall_count]:
            parts.append(f"- {content.strip()}")

        # Limit total size
        result = "\n".join(parts)
        if len(result) > 3000:
            result = result[:3000] + "\n... (truncated)"
        return result

    def system_prompt_block(self) -> str:
        if self._persona_path.exists():
            try:
                p = self._persona_path.read_text(encoding="utf-8").strip()
                if p:
                    return f"## User Persona (Reames)\n{p}"
            except Exception:
                pass
        return ""

    def on_session_end(self, messages: list = None):
        logger.info("ReamesMemory: session end, turns=%d", self._turn_count)
        # Flush pending L1 extraction
        if self._agent and self._l1_pending > 0:
            try: self._extract_l1()
            except Exception: pass
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                cnt = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            self._bg_threads = []
            if cnt >= self._l2_interval:
                t = threading.Thread(target=self._aggregate_l2, name="reames-l2")
                t.start(); self._bg_threads.append(t)
            if cnt >= self._l3_interval:
                t = threading.Thread(target=self._synthesize_l3, name="reames-l3")
                t.start(); self._bg_threads.append(t)
            if cnt >= 500:
                t = threading.Thread(target=self.prune, name="reames-prune")
                t.start(); self._bg_threads.append(t)
        except Exception as e:
            logger.debug("L2/L3 check failed: %s", e)

    def register_signal_handler(self):
        """Register SIGINT handler to flush memory on Ctrl+C."""
        import signal
        def _flush_and_exit(signum, frame):
            try:
                self.on_session_end()
                for t in getattr(self, "_bg_threads", []):
                    t.join(timeout=3)
            except Exception:
                pass
            import sys as _sys
            _sys.exit(0)
        try:
            signal.signal(signal.SIGINT, _flush_and_exit)
        except Exception:
            pass

    def shutdown(self):
        logger.info("ReamesMemory: shutdown")
        for t in getattr(self, "_bg_threads", []):
            try:
                t.join(timeout=5)
            except Exception:
                pass

    def on_turn_start(self, turn_count: int, message: str = ""):
        """Compatibility: called at turn start."""
        pass

    def prefetch_all(self, query: str) -> str:
        """Compatibility: prefetch for all layers."""
        return self.recall(query) or ""

    def queue_prefetch_all(self, query: str):
        """Compatibility: queue async prefetch (sync in ReamesMemory)."""
        pass

    def sync_all(self, user_content: str, assistant_content: str, *, session_id: str = ""):
        """Compatibility: sync all providers."""
        self.capture_turn(user_content, assistant_content)

    def shutdown_all(self):
        """Compatibility: shutdown all providers."""
        self.shutdown()

    def on_session_switch(self, new_session_id: str, parent_session_id: str, **kwargs):
        """Compatibility: called on session switch (compression)."""
        self._session_id = new_session_id
        self.capture_turn("[Session compressed]", "")

    # -- Search (L0+L1, keyword + vector) --------------------------

    def _search_keyword(self, query: str, limit: int = 5) -> List[Tuple[str, float]]:
        results = []
        with sqlite3.connect(str(self._db_path)) as conn:
            for table, weight in [("memories", 1.0), ("messages", 0.5)]:
                try:
                    rows = conn.execute(
                        f"SELECT m.content, rank FROM {table}_fts f JOIN {table} m ON f.rowid=m.id "
                        f"WHERE {table}_fts MATCH ? ORDER BY rank LIMIT ?",
                        (query, limit)
                    ).fetchall()
                    results += [(r[0], weight/(i+1)) for i, r in enumerate(rows)]
                except Exception:
                    pass
        # LIKE fallback for CJK text — must be inside its own `with` block
        # (the conn from the FTS5 `with` above is already closed here)
        with sqlite3.connect(str(self._db_path)) as conn:
            for table, weight in [('memories', 0.3), ('messages', 0.15)]:
                try:
                    rows = conn.execute(
                        f'SELECT content FROM {table} WHERE content LIKE ? LIMIT ?',
                        ('%' + query + '%', limit)
                    ).fetchall()
                    results += [(r[0], weight/(i+1)) for i, r in enumerate(rows)]
                except Exception:
                    pass
        return results

    def _search_vector(self, query: str, limit: int = 5) -> List[Tuple[str, float]]:
        vec = self._get_embedding(query)
        if vec is None:
            return []
        results = []
        with sqlite3.connect(str(self._db_path)) as conn:
            for table, weight in [("memories", 1.0), ("messages", 0.8)]:
                rows = conn.execute(f"SELECT content, embedding FROM {table} WHERE embedding IS NOT NULL LIMIT 200").fetchall()
                for content, emb in rows:
                    v = self._blob_to_vector(emb)
                    if v and len(v) == len(vec):
                        sim = self._cosine_sim(vec, v) * weight
                        results.append((content, sim))

        # Freshness weighting
        now = datetime.now()
        ilist = []
        with __import__("sqlite3").connect(str(self._db_path)) as conn:
            for content, score in results:
                row = conn.execute("SELECT created_at FROM memories WHERE content=? LIMIT 1", (content,)).fetchone()
                if not row:
                    row = conn.execute("SELECT created_at FROM messages WHERE content=? LIMIT 1", (content,)).fetchone()
                if row and row[0]:
                    try:
                        ts = datetime.strptime(row[0][:19], "%Y-%m-%d %H:%M:%S")
                        age_weeks = (now - ts).total_seconds() / 86400 / 7.0
                        age_hours = (now - ts).total_seconds() / 3600
                        week_f = 1.0/(age_weeks+1)
                        hour_f = 1.0/(age_hours+1)
                        score = score * 0.7 + (week_f * 0.7 + hour_f * 0.3) * 0.3
                    except Exception: pass
                ilist.append((content, score))
        ilist.sort(key=lambda x: x[1], reverse=True)
        return ilist[:limit]

    def _search_fresh(self, query: str, limit: int = 5):
        """Search with freshness weighting: newer results score higher."""
        raw = self._search_keyword(query, limit * 2)
        if not raw:
            return raw
        now = datetime.now()
        weighted = []
        with __import__('sqlite3').connect(str(self._db_path)) as conn:
            for content, score in raw:
                row = conn.execute(
                    "SELECT created_at FROM memories WHERE content = ? LIMIT 1",
                    (content,)
                ).fetchone()
                if not row:
                    row = conn.execute(
                        "SELECT created_at FROM messages WHERE content = ? LIMIT 1",
                        (content,)
                    ).fetchone()
                if row and row[0]:
                    try:
                        ts = datetime.strptime(row[0][:19], "%Y-%m-%d %H:%M:%S")
                        age_weeks = (now - ts).total_seconds() / 86400 / 7.0
                        age_hours = (now - ts).total_seconds() / 3600
                        week_f = 1.0/(age_weeks+1)
                        hour_f = 1.0/(age_hours+1)
                        score = score * 0.7 + (week_f * 0.7 + hour_f * 0.3) * 0.3
                    except Exception:
                        pass
                weighted.append((content, score))
        weighted.sort(key=lambda x: x[1], reverse=True)
        return weighted[:limit]

    def _rrf_fusion(self, kw: list, vec: list, k: int = 60) -> list:
        scores = {}
        for rank, (c, _) in enumerate(kw):
            scores[c] = scores.get(c, 0) + 1.0/(k+rank+1)
        for rank, (c, _) in enumerate(vec):
            scores[c] = scores.get(c, 0) + 1.0/(k+rank+1)
        return sorted(scores.items(), key=lambda x: x[1], reverse=True)

    # -- Embedding --------------------------------------------------

    def _get_embedding(self, text: str) -> Optional[List[float]]:
        if not self._embedding_api_key:
            return None
        try:
            import urllib.request
            body = json.dumps({"model": self._embedding_model, "input": text}).encode()
            req = urllib.request.Request(
                f"{self._embedding_api_base}/embeddings", data=body,
                headers={"Authorization": f"Bearer {self._embedding_api_key}",
                         "Content-Type": "application/json"}
            )
            resp = urllib.request.urlopen(req, timeout=30)
            data = json.loads(resp.read())
            return data["data"][0]["embedding"]
        except Exception as e:
            logger.debug("Embedding failed: %s", e)
            return None

    # -- L1 Extraction (via DeepSeek, with dedup) -------------------

    def _extract_l1(self):
        if not self._agent:
            return
        try:
            recent = self._get_recent(limit=self._l1_interval*2)
            if not recent:
                return
            prompt = (
                "从以下对话中提取关键事实（用户偏好、项目信息、技术决策）。"
                "每行一条事实，不要评论。\n\n"
                + recent
            )
            facts = self._call_llm(prompt)
            if not facts:
                return
            with sqlite3.connect(str(self._db_path)) as conn:
                existing = set(r[0] for r in conn.execute("SELECT content FROM memories").fetchall())
                inserted = 0
                for line in facts.split("\n"):
                    f = line.strip().lstrip("- ").strip()
                    if not f or len(f) < 5 or f in existing:
                        continue
                    existing.add(f)
                    emb = self._get_embedding(f)
                    try:
                        conn.execute(
                            "INSERT INTO memories (session_id, content, embedding) VALUES (?,?,?)",
                            (self._session_id, f, self._blob_from_vec(emb) if emb else None)
                        )
                        inserted += 1
                    except sqlite3.IntegrityError:
                        pass  # duplicate (unique index)
                conn.commit()
            logger.info("L1: extracted %d new facts", inserted)
        except Exception as e:
            logger.warning("L1 extraction failed: %s", e)

    def _call_llm(self, prompt: str) -> str:
        try:
            agent = self._agent
            if not agent:
                return ""
            import urllib.request
            body = json.dumps({
                "model": getattr(agent, 'model', 'deepseek-v4-flash'),
                "messages": [
                    {"role": "system", "content": "你是一个记忆提取助手。只返回提取的事实，不评论。"},
                    {"role": "user", "content": prompt}
                ],
                "max_tokens": 1000, "temperature": 0.0,
            }).encode()
            base = getattr(agent, 'base_url', 'https://api.deepseek.com/v1').rstrip("/")
            # Ensure single /v1 suffix
            if not base.endswith("/v1"):
                base += "/v1"
            req = urllib.request.Request(
                f"{base}/chat/completions", data=body,
                headers={"Authorization": f"Bearer {getattr(agent, 'api_key', '')}",
                         "Content-Type": "application/json"}
            )
            resp = urllib.request.urlopen(req, timeout=60)
            return json.loads(resp.read())["choices"][0]["message"]["content"]
        except Exception as e:
            logger.warning("L1 LLM call failed: %s", e)
            return ""

    # -- L2 Scene Aggregation (via DeepSeek) ------------------------

    def _aggregate_l2(self):
        """Full re-aggregation: read ALL L1 facts, synthesize 2-4 scenes."""
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                cur = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            if cur - self._last_l2_count < self._l2_interval:
                return  # not enough new facts
            self._last_l2_count = cur
            with sqlite3.connect(str(self._db_path)) as conn:
                rows = conn.execute("SELECT content FROM memories").fetchall()
            facts = "\n".join(f"- {r[0]}" for r in rows)
            if not facts:
                return
            prompt = (
                "将以下事实按主题分为2-4个场景。每个场景格式：## 场景名\n- 事实1\n- 事实2\n\n" + facts
            )
            scenes = self._call_llm(prompt)
            if scenes:
                self._scenes_path.write_text(scenes.strip(), encoding="utf-8")
                logger.info("L2 scenes aggregated: %d facts -> %d chars", cur, len(scenes))
        except Exception as e:
            logger.warning("L2 aggregation failed: %s", e)

    def _synthesize_l3(self):
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                rows = conn.execute("SELECT content FROM memories ORDER BY id DESC LIMIT 100").fetchall()
            facts = "\n".join(f"- {r[0]}" for r in rows)
            if not facts:
                return
            prompt = "根据以下事实合成一份简洁的用户画像（偏好、习惯、技术栈、目标）。200字以内。\n\n" + facts
            persona = self._call_llm(prompt)
            if persona:
                self._persona_path.write_text(persona.strip(), encoding="utf-8")
                logger.info("L3 persona synthesized")
        except Exception as e:
            logger.warning("L3 synthesis failed: %s", e)

    # -- Helpers ----------------------------------------------------

    def _get_recent(self, limit: int = 20) -> str:
        with sqlite3.connect(str(self._db_path)) as conn:
            rows = conn.execute(
                "SELECT role, content FROM messages WHERE session_id=? AND role IN ('user','assistant') ORDER BY id DESC LIMIT ?",
                (self._session_id, limit)
            ).fetchall()
        rows = list(reversed(rows))
        text = "\n".join(f"{r[0]}: {r[1][:500]}" for r in rows)
        text = re.sub(r"<\s*memory-context\s*>[\s\S]*?<\s*/memory-context\s*>", "", text, flags=re.IGNORECASE)
        return text

    @staticmethod
    def _blob_from_vec(vec: Optional[List[float]]) -> Optional[bytes]:
        if vec is None:
            return None
        import struct as _struct
        return _struct.pack(f"<{len(vec)}d", *vec)

    @staticmethod
    def _blob_to_vector(blob: bytes) -> List[float]:
        try:
            import struct as _struct
            count = len(blob) // 8
            return list(_struct.unpack(f"<{count}d", blob))
        except Exception:
            return []

    @staticmethod
    def _cosine_sim(a: List[float], b: List[float]) -> float:
        dot = sum(x*y for x, y in zip(a, b))
        na = sum(x*x for x in a) ** 0.5
        nb = sum(x*x for x in b) ** 0.5
        return dot/(na*nb) if na and nb else 0.0

    def get_tool_schemas(self) -> List[dict]:
        return [{
            "name": "reames_memory_status",
            "description": "Show Reames memory system status (L0-L3).",
            "parameters": {"type": "object", "properties": {}, "required": []}
        }, {
            "name": "reames_memory_prune",
            "description": "Manually prune old memories.",
            "parameters": {"type": "object", "properties": {"max_count": {"type": "integer", "default": 500}}, "required": []}
        }, {
            "name": "reames_memory_list",
            "description": "List recent memories.",
            "parameters": {"type": "object", "properties": {"limit": {"type": "integer", "default": 20}}, "required": []}
        }, {
            "name": "reames_memory_search",
            "description": "Search Reames memory (L0-L3): conversations, facts, scenes, persona.",
            "parameters": {
                "type": "object",
                "properties": {"query": {"type": "string", "description": "Search query"}},
                "required": ["query"]
            }
        }]

    # -- Management -------------------------------------------------

    def get_status(self) -> str:
        """Return memory system status summary."""
        with sqlite3.connect(str(self._db_path)) as conn:
            msg_cnt = conn.execute("SELECT COUNT(*) FROM messages").fetchone()[0]
            mem_cnt = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            emb_cnt = conn.execute("SELECT COUNT(*) FROM memories WHERE embedding IS NOT NULL").fetchone()[0]
        persona = "yes" if self._persona_path.exists() else "no"
        scenes = "yes" if self._scenes_path.exists() else "no"
        db_kb = self._db_path.stat().st_size / 1024 if self._db_path.exists() else 0
        return (
            "ReamesMemory L0=%d L1=%d(e=%d) L2=%s L3=%s DB=%.0fKB turns=%d"
            % (msg_cnt, mem_cnt, emb_cnt, scenes, persona, db_kb, self._turn_count)
        )
    def restore_from_archive(self, archive_path: str) -> str:
        """Restore memory database from an archive file."""
        import shutil
        src = Path(archive_path)
        if not src.exists():
            return f"Archive not found: {archive_path}"
        self.shutdown()
        shutil.copy2(str(src), str(self._db_path))
        return f"Restored from {src.name} ({src.stat().st_size/1024:.0f}KB)"

    def list_memories(self, limit: int = 20) -> str:
        """List recent memories with clear L0/L1 separation."""
        out = []
        with sqlite3.connect(str(self._db_path)) as conn:
            mem_cnt = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            msg_cnt = conn.execute("SELECT COUNT(*) FROM messages").fetchone()[0]
            out.append("=== L1 Facts (%d total, showing %d) ===" % (mem_cnt, min(limit, mem_cnt)))
            rows = conn.execute(
                "SELECT content, created_at FROM memories ORDER BY id DESC LIMIT ?",
                (limit,)
            ).fetchall()
            if rows:
                for idx, (content, ts) in enumerate(rows, 1):
                    out.append("  %d. [%s] %s" % (idx, ts[:10], content[:120]))
            else:
                out.append("  (no L1 facts yet)")
            out.append("")
            out.append("=== L0 Messages (%d total, showing last %d) ===" % (msg_cnt, min(10, msg_cnt)))
            rows2 = conn.execute(
                "SELECT role, content, created_at FROM messages ORDER BY id DESC LIMIT 10"
            ).fetchall()
            if rows2:
                for idx, (role, content, ts) in enumerate(rows2, 1):
                    out.append("  %d. [%s] %s: %s" % (idx, ts[:10], role, content[:100]))
            else:
                out.append("  (no messages yet)")
        return chr(10).join(out)

    def manual_prune(self, max_count: int = 500) -> str:
        """Manually trigger prune and return result."""
        with sqlite3.connect(str(self._db_path)) as conn:
            before = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
        self.archive()
        self.prune(max_memories=max_count)
        with sqlite3.connect(str(self._db_path)) as conn:
            after = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
        return f"Pruned: {before} -> {after} memories"

    # -- Entropy management ------------------------------------------

    def prune(self, max_memories: int = 500, max_age_days: int = 30):
        """Prune old memories. Keeps most recent N, deletes oldest beyond that.
        
        Args:
            max_memories: Keep at most this many (default 500)
            max_age_days: Also delete older than this (default 30 days)
        """
        with sqlite3.connect(str(self._db_path)) as conn:
            try:
                # Delete by age
                conn.execute(
                    "DELETE FROM memories WHERE created_at < datetime('now', ?)",
                    (f'-{max_age_days} days',)
                )
                aged = conn.total_changes

                # Delete by count: keep only the most recent max_memories
                conn.execute("""
                    DELETE FROM memories WHERE id NOT IN (
                        SELECT id FROM memories ORDER BY id DESC LIMIT ?
                    )
                """, (max_memories,))
                counted = conn.total_changes - aged

                # Also prune old messages
                conn.execute(
                    "DELETE FROM messages WHERE created_at < datetime('now', ?)",
                    (f'-{max_age_days} days',)
                )
                conn.execute("""
                    DELETE FROM messages WHERE id NOT IN (
                        SELECT id FROM messages ORDER BY id DESC LIMIT ?
                    )
                """, (max_memories * 10,))

                # Rebuild FTS index after deletes
                conn.execute("INSERT INTO memories_fts(memories_fts) VALUES('rebuild')")
                conn.execute("INSERT INTO messages_fts(messages_fts) VALUES('rebuild')")
                conn.commit()

                total = aged + counted
                if total > 0:
                    logger.info("Pruned %d old records (age=%d, count=%d)", total, aged, counted)
            except Exception as e:
                logger.warning("Prune failed: %s", e)

    def archive(self, archive_dir: str = ""):
        """Archive old memories to a backup file before pruning."""
        target = Path(archive_dir or str(self._data_dir)) / f"archive_{datetime.now().strftime('%Y%m%d')}.db"
        if target.exists():
            return
        try:
            import shutil
            shutil.copy2(str(self._db_path), str(target))
            logger.info("Archived to %s", target)
        except Exception as e:
            logger.warning("Archive failed: %s", e)

    # -- API compatibility ------------------------------------------

    def prefetch(self, query: str, *, session_id: str = "") -> str:
        return self.recall(query, session_id=session_id or self._session_id)

    def sync_turn(self, user_content: str, assistant_content: str, *, session_id: str = ""):
        self.capture_turn(user_content, assistant_content)

    def queue_prefetch(self, query: str, *, session_id: str = ""):
        pass

    def handle_tool_call(self, tool_name: str, args: dict) -> str:
        if tool_name == "reames_memory_search":
            return self.recall(args.get("query", ""))
        if tool_name == "reames_memory_status":
            return self.get_status()
        if tool_name == "reames_memory_prune":
            return self.manual_prune(int(args.get("max_count", 500)))
        if tool_name == "reames_memory_list":
            return self.list_memories(int(args.get("limit", 20)))
        return f"Unknown tool: {tool_name}"

    def has_tool(self, tool_name: str) -> bool:
        return tool_name in ("reames_memory_search", "reames_memory_status", "reames_memory_prune", "reames_memory_list")


def get_tool_schemas() -> List[dict]:
    return ReamesMemory().get_tool_schemas()
