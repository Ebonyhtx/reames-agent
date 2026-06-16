"""Reames Memory — Python native L0-L3 memory engine. No external Gateway needed.

Uses SQLite for storage, DeepSeek for extraction, optional embedding API for vector search.
"""

from __future__ import annotations

import json
import logging
import os
import sqlite3
import threading
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

logger = logging.getLogger(__name__)

DEFAULT_L1_INTERVAL = 10
DEFAULT_L2_INTERVAL = 50
DEFAULT_L3_INTERVAL = 200
DEFAULT_RECALL_COUNT = 5


class ReamesMemory:
    """Reames native memory engine. SQLite-backed, zero external dependencies.

    Usage:
        mem = ReamesMemory(data_dir="~/.reames/memory")
        mem.initialize(session_id="xxx", user_id="user1", agent=agent)
        mem.capture_turn("hello", "hi there")
        ctx = mem.recall("Python project", session_id="xxx")
        mem.on_session_end()
    """

    def __init__(self, data_dir: Optional[str] = None):
        if data_dir:
            self._data_dir = Path(data_dir)
        else:
            home = os.environ.get("HOME") or os.environ.get("USERPROFILE") or "."
            self._data_dir = Path(home) / ".reames" / "memory"
        self._data_dir.mkdir(parents=True, exist_ok=True)

        self._db_path = self._data_dir / "reames_memory.db"
        self._persona_path = self._data_dir / "persona.md"

        self._session_id = ""
        self._user_id = "default"
        self._turn_count = 0
        self._l1_pending = 0
        self._lock = threading.Lock()
        self._agent: Any = None

        # Embedding config
        self._embedding_api_key = ""
        self._embedding_api_base = ""
        self._embedding_model = ""

        # Intervals
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
                content TEXT NOT NULL,
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

            # FTS5 indexes
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

    def capture_turn(self, user_content: str, assistant_content: str):
        if not user_content:
            return
        with self._lock:
            self._turn_count += 1
            with sqlite3.connect(str(self._db_path)) as conn:
                conn.execute("INSERT INTO messages (session_id, role, content) VALUES (?,?,?)",
                            (self._session_id, "user", user_content))
                if assistant_content:
                    conn.execute("INSERT INTO messages (session_id, role, content) VALUES (?,?,?)",
                                (self._session_id, "assistant", assistant_content))
                conn.commit()
            self._l1_pending += 1

        if self._l1_pending >= self._l1_interval and self._agent:
            self._l1_pending = 0
            t = threading.Thread(target=self._extract_l1, daemon=True, name="reames-l1")
            t.start()

    def recall(self, query: str, *, session_id: str = "") -> str:
        if not query:
            return ""
        kw = self._search_keyword(query, self._recall_count)
        results = kw
        if self._embedding_api_key:
            try:
                vec = self._search_vector(query, self._recall_count)
                results = self._rrf_fusion(kw, vec)
            except Exception as e:
                logger.debug("Vector search failed: %s", e)
        if not results:
            return ""
        parts = ["## Reames Memory\n"]
        for content, _ in results[:self._recall_count]:
            parts.append(f"- {content.strip()}")
        return "\n".join(parts)

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
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                cnt = conn.execute("SELECT COUNT(*) FROM memories").fetchone()[0]
            if cnt >= self._l2_interval:
                t = threading.Thread(target=self._synthesize_l3, daemon=True, name="reames-l3")
                t.start()
        except Exception as e:
            logger.debug("L3 check failed: %s", e)

    def shutdown(self):
        logger.info("ReamesMemory: shutdown")

    # -- Search -----------------------------------------------------

    def _search_keyword(self, query: str, limit: int = 5) -> List[Tuple[str, float]]:
        with sqlite3.connect(str(self._db_path)) as conn:
            try:
                rows = conn.execute(
                    "SELECT m.content, rank FROM memories_fts f JOIN memories m ON f.rowid=m.id "
                    "WHERE memories_fts MATCH ? ORDER BY rank LIMIT ?",
                    (query, limit)
                ).fetchall()
                return [(r[0], 1.0/(i+1)) for i, r in enumerate(rows)]
            except Exception:
                return []

    def _search_vector(self, query: str, limit: int = 5) -> List[Tuple[str, float]]:
        vec = self._get_embedding(query)
        if vec is None:
            return []
        with sqlite3.connect(str(self._db_path)) as conn:
            rows = conn.execute("SELECT content, embedding FROM memories WHERE embedding IS NOT NULL").fetchall()
        results = []
        for content, emb in rows:
            v = self._blob_to_vector(emb)
            if v and len(v) == len(vec):
                sim = self._cosine_sim(vec, v)
                results.append((content, sim))
        results.sort(key=lambda x: x[1], reverse=True)
        return results[:limit]

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

    # -- L1 Extraction (via DeepSeek) -------------------------------

    def _extract_l1(self):
        if not self._agent:
            return
        try:
            recent = self._get_recent(limit=self._l1_interval*2)
            if not recent:
                return
            prompt = (
                "Extract key facts from this conversation (user preferences, project info, "
                "technical decisions). One fact per line. No commentary.\n\n"
                + recent
            )
            facts = self._call_llm(prompt)
            if not facts:
                return
            with sqlite3.connect(str(self._db_path)) as conn:
                for line in facts.split("\n"):
                    f = line.strip().lstrip("- ").strip()
                    if not f or len(f) < 5:
                        continue
                    emb = self._get_embedding(f)
                    conn.execute(
                        "INSERT INTO memories (session_id, content, embedding) VALUES (?,?,?)",
                        (self._session_id, f, self._blob_from_vec(emb) if emb else None)
                    )
                conn.commit()
            logger.info("L1: extracted %d facts", len([l for l in facts.split("\n") if l.strip()]))
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
                    {"role": "system", "content": "You are a memory extraction assistant. Return facts only."},
                    {"role": "user", "content": prompt}
                ],
                "max_tokens": 1000, "temperature": 0.0,
            }).encode()
            base = getattr(agent, 'base_url', 'https://api.deepseek.com')
            req = urllib.request.Request(
                f"{base}/v1/chat/completions", data=body,
                headers={"Authorization": f"Bearer {getattr(agent, 'api_key', '')}",
                         "Content-Type": "application/json"}
            )
            resp = urllib.request.urlopen(req, timeout=60)
            return json.loads(resp.read())["choices"][0]["message"]["content"]
        except Exception as e:
            logger.warning("L1 LLM call failed: %s", e)
            return ""

    # -- L3 Synthesis -----------------------------------------------

    def _synthesize_l3(self):
        try:
            with sqlite3.connect(str(self._db_path)) as conn:
                rows = conn.execute("SELECT content FROM memories ORDER BY id DESC LIMIT 100").fetchall()
            facts = "\n".join(f"- {r[0]}" for r in rows)
            if not facts:
                return
            prompt = "Synthesize a concise user persona (preferences, habits, goals) from these facts. Under 200 chars.\n\n" + facts
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
                "SELECT role, content FROM messages WHERE session_id=? ORDER BY id DESC LIMIT ?",
                (self._session_id, limit)
            ).fetchall()
        rows = list(reversed(rows))
        return "\n".join(f"{r[0]}: {r[1][:500]}" for r in rows)

    @staticmethod
    def _blob_from_vec(vec: Optional[List[float]]) -> Optional[bytes]:
        if vec is None:
            return None
        return b"".join(str(v).encode() + b"," for v in vec)

    @staticmethod
    def _blob_to_vector(blob: bytes) -> List[float]:
        try:
            return [float(p) for p in blob.decode().rstrip(",").split(",") if p]
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
            "name": "reames_memory_search",
            "description": "Search Reames memory (L0-L3): conversations, facts, scenes.",
            "parameters": {
                "type": "object",
                "properties": {"query": {"type": "string", "description": "Search query"}},
                "required": ["query"]
            }
        }]

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
        return f"Unknown tool: {tool_name}"

    def has_tool(self, tool_name: str) -> bool:
        return tool_name == "reames_memory_search"


def get_tool_schemas() -> List[dict]:
    return ReamesMemory().get_tool_schemas()
